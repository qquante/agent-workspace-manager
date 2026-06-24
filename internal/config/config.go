package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DefaultConfigPath = "/etc/stola/stola.yaml"
	DefaultStatePath  = "/var/lib/stola/stola.db"
	DefaultAuditPath  = "/var/log/stola/audit.jsonl"
	DefaultRuntimeDir = "/run/stola"
)

type Config struct {
	Node     NodeConfig
	Backend  BackendConfig
	Paths    PathsConfig
	API      APIConfig
	LLMProxy LLMProxyConfig
}

type NodeConfig struct {
	ID string
}

type BackendConfig struct {
	Type    string
	Proxmox ProxmoxConfig
	Docker  DockerConfig
}

type ProxmoxConfig struct {
	Endpoint              string
	Node                  string
	TokenFile             string
	InsecureSkipTLSVerify bool
}

type DockerConfig struct {
	CLIPath       string
	Image         string
	NetworkPrefix string
	VolumePrefix  string
}

type PathsConfig struct {
	State      string
	AuditLog   string
	RuntimeDir string
}

type APIConfig struct {
	Listen         string
	UsersFile      string
	SigningKeyFile string
	MCPManifest    string
	PolicyFile     string
}

type LLMProxyConfig struct {
	Listen            string
	ProvidersManifest string
	SecretsFile       string
}

func Default() Config {
	return Config{
		Node: NodeConfig{ID: "stola-local"},
		Backend: BackendConfig{
			Type: "mock",
		},
		Paths: PathsConfig{
			State:      DefaultStatePath,
			AuditLog:   DefaultAuditPath,
			RuntimeDir: DefaultRuntimeDir,
		},
		API: APIConfig{
			Listen:         "127.0.0.1:8750",
			UsersFile:      "/etc/stola/users.yaml",
			SigningKeyFile: "/etc/stola/secrets/control-plane.key",
		},
		LLMProxy: LLMProxyConfig{
			Listen: "127.0.0.1:8760",
		},
	}
}

func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	return Parse(f)
}

func LoadOrDefault(path string) (Config, bool, error) {
	cfg, err := Load(path)
	if err == nil {
		return cfg, true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return Default(), false, nil
	}
	return Config{}, false, err
}

func WriteFile(path string, cfg Config, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config already exists: %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(Render(cfg)), 0o600)
}

func ParseString(s string) (Config, error) {
	return Parse(strings.NewReader(s))
}

func Parse(r interface{ Read([]byte) (int, error) }) (Config, error) {
	cfg := Config{}
	scanner := bufio.NewScanner(r)
	sections := map[int]string{}
	lineNo := 0

	for scanner.Scan() {
		lineNo++
		textLine := scanner.Text()
		if lineNo == 1 {
			textLine = strings.TrimPrefix(textLine, "\ufeff")
		}
		raw := stripComment(textLine)
		if strings.TrimSpace(raw) == "" {
			continue
		}
		indent := leadingSpaces(raw)
		if indent%2 != 0 {
			return Config{}, fmt.Errorf("line %d: indentation must use multiples of two spaces", lineNo)
		}
		depth := indent / 2
		text := strings.TrimSpace(raw)
		key, value, hasValue, err := parseKeyValue(text)
		if err != nil {
			return Config{}, fmt.Errorf("line %d: %w", lineNo, err)
		}
		if !hasValue {
			sections[depth] = key
			for k := range sections {
				if k > depth {
					delete(sections, k)
				}
			}
			continue
		}

		path := make([]string, 0, depth+1)
		for i := 0; i < depth; i++ {
			section, ok := sections[i]
			if !ok {
				return Config{}, fmt.Errorf("line %d: missing parent section for %q", lineNo, key)
			}
			path = append(path, section)
		}
		path = append(path, key)
		if err := setValue(&cfg, path, unquote(value)); err != nil {
			return Config{}, fmt.Errorf("line %d: %w", lineNo, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Render(cfg Config) string {
	lines := []string{
		"node:",
		fmt.Sprintf("  id: %s", cfg.Node.ID),
		"backend:",
		fmt.Sprintf("  type: %s", cfg.Backend.Type),
		"  proxmox:",
		fmt.Sprintf("    endpoint: %s", cfg.Backend.Proxmox.Endpoint),
		fmt.Sprintf("    node: %s", cfg.Backend.Proxmox.Node),
		fmt.Sprintf("    token_file: %s", cfg.Backend.Proxmox.TokenFile),
		fmt.Sprintf("    insecure_skip_tls_verify: %t", cfg.Backend.Proxmox.InsecureSkipTLSVerify),
		"  docker:",
		fmt.Sprintf("    cli_path: %s", cfg.Backend.Docker.CLIPath),
		fmt.Sprintf("    image: %s", cfg.Backend.Docker.Image),
		fmt.Sprintf("    network_prefix: %s", cfg.Backend.Docker.NetworkPrefix),
		fmt.Sprintf("    volume_prefix: %s", cfg.Backend.Docker.VolumePrefix),
		"paths:",
		fmt.Sprintf("  state: %s", cfg.Paths.State),
		fmt.Sprintf("  audit_log: %s", cfg.Paths.AuditLog),
		fmt.Sprintf("  runtime_dir: %s", cfg.Paths.RuntimeDir),
		"api:",
		fmt.Sprintf("  listen: %s", cfg.API.Listen),
		fmt.Sprintf("  users_file: %s", cfg.API.UsersFile),
		fmt.Sprintf("  signing_key_file: %s", cfg.API.SigningKeyFile),
		fmt.Sprintf("  mcp_manifest: %s", cfg.API.MCPManifest),
		fmt.Sprintf("  policy_file: %s", cfg.API.PolicyFile),
		"llm_proxy:",
		fmt.Sprintf("  listen: %s", cfg.LLMProxy.Listen),
		fmt.Sprintf("  providers_manifest: %s", cfg.LLMProxy.ProvidersManifest),
		fmt.Sprintf("  secrets_file: %s", cfg.LLMProxy.SecretsFile),
		"",
	}
	return strings.Join(lines, "\n")
}

func (c Config) Validate() error {
	var problems []string
	if strings.TrimSpace(c.Node.ID) == "" {
		problems = append(problems, "node.id is required")
	}
	switch c.Backend.Type {
	case "mock":
	case "proxmox":
		if c.Backend.Proxmox.Endpoint == "" {
			problems = append(problems, "backend.proxmox.endpoint is required")
		}
		if c.Backend.Proxmox.Node == "" {
			problems = append(problems, "backend.proxmox.node is required")
		}
		if c.Backend.Proxmox.TokenFile == "" {
			problems = append(problems, "backend.proxmox.token_file is required")
		}
	case "docker":
	case "":
		problems = append(problems, "backend.type is required")
	default:
		problems = append(problems, "backend.type must be mock, docker, or proxmox")
	}
	if c.Paths.State == "" {
		problems = append(problems, "paths.state is required")
	}
	if c.Paths.AuditLog == "" {
		problems = append(problems, "paths.audit_log is required")
	}
	if c.Paths.RuntimeDir == "" {
		problems = append(problems, "paths.runtime_dir is required")
	}
	if c.API.Listen == "" {
		problems = append(problems, "api.listen is required")
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func setValue(cfg *Config, path []string, value string) error {
	joined := strings.Join(path, ".")
	switch joined {
	case "node.id":
		cfg.Node.ID = value
	case "backend.type":
		cfg.Backend.Type = value
	case "backend.proxmox.endpoint":
		cfg.Backend.Proxmox.Endpoint = value
	case "backend.proxmox.node":
		cfg.Backend.Proxmox.Node = value
	case "backend.proxmox.token_file":
		cfg.Backend.Proxmox.TokenFile = value
	case "backend.proxmox.insecure_skip_tls_verify":
		parsed, err := parseBool(value)
		if err != nil {
			return err
		}
		cfg.Backend.Proxmox.InsecureSkipTLSVerify = parsed
	case "backend.docker.cli_path":
		cfg.Backend.Docker.CLIPath = value
	case "backend.docker.image":
		cfg.Backend.Docker.Image = value
	case "backend.docker.network_prefix":
		cfg.Backend.Docker.NetworkPrefix = value
	case "backend.docker.volume_prefix":
		cfg.Backend.Docker.VolumePrefix = value
	case "paths.state":
		cfg.Paths.State = value
	case "paths.audit_log":
		cfg.Paths.AuditLog = value
	case "paths.runtime_dir":
		cfg.Paths.RuntimeDir = value
	case "api.listen":
		cfg.API.Listen = value
	case "api.users_file":
		cfg.API.UsersFile = value
	case "api.signing_key_file":
		cfg.API.SigningKeyFile = value
	case "api.mcp_manifest":
		cfg.API.MCPManifest = value
	case "api.policy_file":
		cfg.API.PolicyFile = value
	case "llm_proxy.listen":
		cfg.LLMProxy.Listen = value
	case "llm_proxy.providers_manifest":
		cfg.LLMProxy.ProvidersManifest = value
	case "llm_proxy.secrets_file":
		cfg.LLMProxy.SecretsFile = value
	default:
		return fmt.Errorf("unsupported config key %q", joined)
	}
	return nil
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "1":
		return true, nil
	case "false", "no", "0", "":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value %q", value)
	}
}

func stripComment(s string) string {
	inSingle := false
	inDouble := false
	for i, r := range s {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble {
				return s[:i]
			}
		}
	}
	return s
}

func leadingSpaces(s string) int {
	count := 0
	for _, r := range s {
		if r != ' ' {
			break
		}
		count++
	}
	return count
}

func parseKeyValue(s string) (key string, value string, hasValue bool, err error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return "", "", false, errors.New("empty key")
	}
	key = strings.TrimSpace(parts[0])
	if len(parts) == 1 {
		return key, "", false, nil
	}
	value = strings.TrimSpace(parts[1])
	return key, value, value != "", nil
}

func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
