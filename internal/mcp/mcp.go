package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DefaultRepoManifestPath  = "manifests/mcp/default.yaml"
	DefaultImageManifestPath = "/opt/stola/manifests/mcp/default.yaml"

	WorkspaceManifestPath       = ".stola/mcp/default.yaml"
	AllowlistPath               = ".stola/mcp/allowlist.json"
	OpenCodeConfigPath          = "opencode.json"
	HermesConfigPath            = ".stola/mcp/hermes.config.yaml"
	OpenClawConfigPath          = ".stola/mcp/openclaw.json"
	ContainerOpenCodeConfigPath = "/home/stola/.config/opencode/opencode.json"
	ContainerHermesConfigPath   = "/home/stola/.hermes/config.yaml"
	ContainerOpenClawConfigPath = "/home/stola/.openclaw/openclaw.json"
)

type Manifest struct {
	Version         int             `yaml:"version" json:"version"`
	Name            string          `yaml:"name" json:"name"`
	Description     string          `yaml:"description" json:"description,omitempty"`
	Workspace       Workspace       `yaml:"workspace" json:"workspace"`
	Policy          Policy          `yaml:"policy" json:"policy"`
	ExecutionPolicy ExecutionPolicy `yaml:"execution_policy" json:"execution_policy,omitempty"`
	Servers         []Server        `yaml:"servers" json:"servers"`
	OptionalServers []Server        `yaml:"optional_servers" json:"optional_servers,omitempty"`
	BlockedClasses  []string        `yaml:"blocked_classes" json:"blocked_classes,omitempty"`
}

type Workspace struct {
	Root         string `yaml:"root" json:"root"`
	ManifestPath string `yaml:"manifest_path" json:"manifest_path,omitempty"`
}

type Policy struct {
	DefaultState          string `yaml:"default_state" json:"default_state,omitempty"`
	RequireExplicitEnable bool   `yaml:"require_explicit_enable" json:"require_explicit_enable"`
	SecretsFromEnvOnly    bool   `yaml:"secrets_from_env_only" json:"secrets_from_env_only"`
	AuditAllToolCalls     bool   `yaml:"audit_all_tool_calls" json:"audit_all_tool_calls"`
	NetworkDefault        string `yaml:"network_default" json:"network_default,omitempty"`
	FilesystemDefault     string `yaml:"filesystem_default" json:"filesystem_default,omitempty"`
}

type Server struct {
	ID             string         `yaml:"id" json:"id"`
	Title          string         `yaml:"title" json:"title,omitempty"`
	DefaultEnabled bool           `yaml:"default_enabled" json:"default_enabled"`
	Risk           string         `yaml:"risk" json:"risk,omitempty"`
	Transport      string         `yaml:"transport" json:"transport"`
	Command        []string       `yaml:"command" json:"command,omitempty"`
	URL            string         `yaml:"url" json:"url,omitempty"`
	Permissions    map[string]any `yaml:"permissions" json:"permissions,omitempty"`
	ToolPolicy     ToolPolicy     `yaml:"tool_policy" json:"tool_policy,omitempty"`
	ArgumentPolicy ArgumentPolicy `yaml:"argument_policy" json:"argument_policy,omitempty"`
}

type ToolPolicy struct {
	DefaultAction string     `yaml:"default_action" json:"default_action,omitempty"`
	Rules         []ToolRule `yaml:"rules" json:"rules,omitempty"`
}

type ToolRule struct {
	Tool   string `yaml:"tool" json:"tool"`
	Action string `yaml:"action" json:"action"`
	Reason string `yaml:"reason" json:"reason,omitempty"`
}

type ToolDecision struct {
	Action string
	Reason string
	Rule   string
}

const (
	ToolActionAllow           = "allow"
	ToolActionRequireApproval = "require_approval"
	ToolActionDeny            = "deny"
)

type ServerView struct {
	Server
	Optional bool `json:"optional"`
	Enabled  bool `json:"enabled"`
}

type Bundle struct {
	Manifest Manifest
	Files    map[string][]byte
}

var serverIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)

func ResolveManifestPath(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return explicit
	}
	if envPath := strings.TrimSpace(os.Getenv("STOLA_MCP_MANIFEST")); envPath != "" {
		return envPath
	}
	if _, err := os.Stat(DefaultRepoManifestPath); err == nil {
		return DefaultRepoManifestPath
	}
	return DefaultImageManifestPath
}

func LoadFile(filePath string) (Manifest, []byte, error) {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return Manifest{}, nil, err
	}
	manifest, err := Parse(raw)
	if err != nil {
		return Manifest{}, nil, err
	}
	return manifest, raw, nil
}

func Parse(raw []byte) (Manifest, error) {
	var manifest Manifest
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (m Manifest) Validate() error {
	var problems []string
	if m.Version <= 0 {
		problems = append(problems, "version must be positive")
	}
	if strings.TrimSpace(m.Name) == "" {
		problems = append(problems, "name is required")
	}
	if strings.TrimSpace(m.Workspace.Root) == "" {
		problems = append(problems, "workspace.root is required")
	}
	if len(m.Servers) == 0 {
		problems = append(problems, "servers must not be empty")
	}
	problems = append(problems, validateExecutionPolicy("execution_policy", m.ExecutionPolicy)...)

	seen := map[string]bool{}
	for _, item := range []struct {
		kind    string
		servers []Server
	}{
		{kind: "servers", servers: m.Servers},
		{kind: "optional_servers", servers: m.OptionalServers},
	} {
		for index, server := range item.servers {
			location := fmt.Sprintf("%s[%d]", item.kind, index)
			id := strings.TrimSpace(server.ID)
			if id == "" {
				problems = append(problems, location+".id is required")
				continue
			}
			if !serverIDPattern.MatchString(id) {
				problems = append(problems, location+".id must match ^[a-z][a-z0-9_.-]*$")
			}
			if seen[id] {
				problems = append(problems, fmt.Sprintf("duplicate server id %q", id))
			}
			seen[id] = true
			problems = append(problems, validateServer(location, server)...)
			problems = append(problems, validateToolPolicy(location+".tool_policy", server.ToolPolicy)...)
			problems = append(problems, validateArgumentPolicy(location+".argument_policy", server.ArgumentPolicy)...)
			if server.DefaultEnabled && len(server.SecretNames()) > 0 {
				problems = append(problems, location+" cannot be default_enabled while requiring secrets")
			}
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func validateServer(location string, server Server) []string {
	var problems []string
	switch server.Transport {
	case "stdio":
		if len(server.Command) == 0 {
			problems = append(problems, location+".command is required for stdio transport")
		}
		for i, arg := range server.Command {
			if strings.TrimSpace(arg) == "" {
				problems = append(problems, fmt.Sprintf("%s.command[%d] must not be empty", location, i))
			}
		}
	case "http", "streamable-http", "sse":
		if strings.TrimSpace(server.URL) == "" {
			problems = append(problems, location+".url is required for HTTP transport")
		}
	default:
		problems = append(problems, location+".transport must be stdio, http, streamable-http, or sse")
	}
	switch server.Risk {
	case "", "low", "medium", "high":
	default:
		problems = append(problems, location+".risk must be low, medium, or high")
	}
	return problems
}

func validateToolPolicy(location string, policy ToolPolicy) []string {
	var problems []string
	if policy.DefaultAction != "" && !validToolAction(policy.DefaultAction) {
		problems = append(problems, location+".default_action must be allow, require_approval, or deny")
	}
	seen := map[string]bool{}
	for index, rule := range policy.Rules {
		ruleLocation := fmt.Sprintf("%s.rules[%d]", location, index)
		tool := strings.TrimSpace(rule.Tool)
		if tool == "" {
			problems = append(problems, ruleLocation+".tool is required")
		}
		if seen[tool] {
			problems = append(problems, fmt.Sprintf("%s duplicate tool rule %q", location, tool))
		}
		seen[tool] = true
		if !validToolAction(rule.Action) {
			problems = append(problems, ruleLocation+".action must be allow, require_approval, or deny")
		}
	}
	return problems
}

func validToolAction(action string) bool {
	switch action {
	case ToolActionAllow, ToolActionRequireApproval, ToolActionDeny:
		return true
	default:
		return false
	}
}

func (m Manifest) ServerViews(includeOptional bool) []ServerView {
	views := make([]ServerView, 0, len(m.Servers)+len(m.OptionalServers))
	for _, server := range m.Servers {
		views = append(views, ServerView{Server: server, Optional: false, Enabled: server.DefaultEnabled})
	}
	if includeOptional {
		for _, server := range m.OptionalServers {
			views = append(views, ServerView{Server: server, Optional: true, Enabled: server.DefaultEnabled})
		}
	}
	sort.SliceStable(views, func(i, j int) bool {
		if views[i].Optional != views[j].Optional {
			return !views[i].Optional
		}
		return views[i].ID < views[j].ID
	})
	return views
}

func (s Server) SecretNames() []string {
	raw, ok := s.Permissions["secrets"]
	if !ok || raw == nil {
		return nil
	}
	var out []string
	switch value := raw.(type) {
	case []any:
		for _, item := range value {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
	case []string:
		for _, item := range value {
			if strings.TrimSpace(item) != "" {
				out = append(out, strings.TrimSpace(item))
			}
		}
	}
	sort.Strings(out)
	return out
}

func (s Server) ToolDecision(toolName string) ToolDecision {
	defaultAction := s.ToolPolicy.DefaultAction
	if defaultAction == "" {
		defaultAction = ToolActionAllow
	}
	decision := ToolDecision{
		Action: defaultAction,
		Reason: "server default policy",
		Rule:   "default",
	}
	for _, rule := range s.ToolPolicy.Rules {
		if rule.Tool != toolName {
			continue
		}
		reason := rule.Reason
		if reason == "" {
			reason = "matched explicit tool policy"
		}
		return ToolDecision{
			Action: rule.Action,
			Reason: reason,
			Rule:   rule.Tool,
		}
	}
	return decision
}

func RenderBundle(manifest Manifest, rawManifest []byte) (Bundle, error) {
	if err := manifest.Validate(); err != nil {
		return Bundle{}, err
	}
	if len(rawManifest) == 0 {
		rendered, err := yaml.Marshal(manifest)
		if err != nil {
			return Bundle{}, err
		}
		rawManifest = rendered
	}

	allowlist, err := renderAllowlist(manifest)
	if err != nil {
		return Bundle{}, err
	}
	openCode, err := renderOpenCodeConfig(manifest)
	if err != nil {
		return Bundle{}, err
	}
	hermes, err := renderHermesConfig(manifest)
	if err != nil {
		return Bundle{}, err
	}
	openClaw, err := renderOpenClawConfig(manifest)
	if err != nil {
		return Bundle{}, err
	}

	return Bundle{
		Manifest: manifest,
		Files: map[string][]byte{
			WorkspaceManifestPath: rawManifest,
			AllowlistPath:         allowlist,
			OpenCodeConfigPath:    openCode,
			HermesConfigPath:      hermes,
			OpenClawConfigPath:    openClaw,
		},
	}, nil
}

func LoadAndRender(filePath string) (Bundle, error) {
	manifest, raw, err := LoadFile(filePath)
	if err != nil {
		return Bundle{}, err
	}
	return RenderBundle(manifest, raw)
}

func WriteBundleToDir(bundle Bundle, workspaceDir string) error {
	for filePath, content := range bundle.Files {
		if err := validateRelativeWorkspacePath(filePath); err != nil {
			return err
		}
		target := path.Clean(filePath)
		hostTarget := strings.ReplaceAll(target, "/", string(os.PathSeparator))
		fullPath := workspaceDir + string(os.PathSeparator) + hostTarget
		if err := os.MkdirAll(parentDir(fullPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(fullPath, content, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func validateRelativeWorkspacePath(filePath string) error {
	clean := path.Clean(strings.ReplaceAll(filePath, "\\", "/"))
	if clean == "." || strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, "../") || clean == ".." || strings.Contains(clean, "\x00") {
		return fmt.Errorf("invalid workspace path: %q", filePath)
	}
	return nil
}

func parentDir(filePath string) string {
	idx := strings.LastIndex(filePath, string(os.PathSeparator))
	if idx == -1 {
		return "."
	}
	return filePath[:idx]
}

func renderAllowlist(manifest Manifest) ([]byte, error) {
	type allowlistServer struct {
		ID          string         `json:"id"`
		Title       string         `json:"title,omitempty"`
		Enabled     bool           `json:"enabled"`
		Optional    bool           `json:"optional"`
		Risk        string         `json:"risk,omitempty"`
		Transport   string         `json:"transport"`
		Command     []string       `json:"command,omitempty"`
		URL         string         `json:"url,omitempty"`
		Secrets     []string       `json:"secrets,omitempty"`
		Permissions map[string]any `json:"permissions,omitempty"`
		ToolPolicy  ToolPolicy     `json:"tool_policy,omitempty"`
		ArgPolicy   ArgumentPolicy `json:"argument_policy,omitempty"`
	}
	doc := struct {
		Version         int               `json:"version"`
		Name            string            `json:"name"`
		Description     string            `json:"description,omitempty"`
		Policy          Policy            `json:"policy"`
		ExecutionPolicy ExecutionPolicy   `json:"execution_policy,omitempty"`
		Workspace       Workspace         `json:"workspace"`
		Servers         []allowlistServer `json:"servers"`
		BlockedClasses  []string          `json:"blocked_classes,omitempty"`
	}{
		Version:         manifest.Version,
		Name:            manifest.Name,
		Description:     manifest.Description,
		Policy:          manifest.Policy,
		ExecutionPolicy: manifest.ExecutionPolicy,
		Workspace:       manifest.Workspace,
		BlockedClasses:  append([]string(nil), manifest.BlockedClasses...),
	}
	for _, view := range manifest.ServerViews(true) {
		doc.Servers = append(doc.Servers, allowlistServer{
			ID:          view.ID,
			Title:       view.Title,
			Enabled:     view.Enabled,
			Optional:    view.Optional,
			Risk:        view.Risk,
			Transport:   view.Transport,
			Command:     append([]string(nil), view.Command...),
			URL:         view.URL,
			Secrets:     view.SecretNames(),
			Permissions: view.Permissions,
			ToolPolicy:  view.ToolPolicy,
			ArgPolicy:   view.ArgumentPolicy,
		})
	}
	return json.MarshalIndent(doc, "", "  ")
}

func renderOpenCodeConfig(manifest Manifest) ([]byte, error) {
	doc := map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"mcp":     map[string]any{},
	}
	servers := doc["mcp"].(map[string]any)
	for _, view := range manifest.ServerViews(true) {
		item := map[string]any{"enabled": view.Enabled}
		switch view.Transport {
		case "stdio":
			item["type"] = "local"
			item["command"] = append([]string(nil), view.Command...)
		case "http", "streamable-http", "sse":
			item["type"] = "remote"
			item["url"] = view.URL
		default:
			return nil, fmt.Errorf("unsupported transport for OpenCode render: %s", view.Transport)
		}
		servers[view.ID] = item
	}
	return json.MarshalIndent(doc, "", "  ")
}

func renderHermesConfig(manifest Manifest) ([]byte, error) {
	servers := map[string]any{}
	for _, view := range manifest.ServerViews(false) {
		if !view.Enabled {
			continue
		}
		item := map[string]any{}
		switch view.Transport {
		case "stdio":
			item["command"] = view.Command[0]
			if len(view.Command) > 1 {
				item["args"] = append([]string(nil), view.Command[1:]...)
			}
		case "http", "streamable-http", "sse":
			item["url"] = view.URL
		default:
			return nil, fmt.Errorf("unsupported transport for Hermes render: %s", view.Transport)
		}
		servers[view.ID] = item
	}
	doc := map[string]any{"mcp_servers": servers}
	return yaml.Marshal(doc)
}

func renderOpenClawConfig(manifest Manifest) ([]byte, error) {
	servers := map[string]any{}
	for _, view := range manifest.ServerViews(true) {
		item := map[string]any{"enabled": view.Enabled}
		switch view.Transport {
		case "stdio":
			continue
		case "http":
			item["url"] = view.URL
			item["transport"] = "streamable-http"
		case "streamable-http", "sse":
			item["url"] = view.URL
			item["transport"] = view.Transport
		default:
			return nil, fmt.Errorf("unsupported transport for OpenClaw render: %s", view.Transport)
		}
		servers[view.ID] = item
	}
	doc := map[string]any{
		"mcp": map[string]any{
			"servers": servers,
		},
	}
	return json.MarshalIndent(doc, "", "  ")
}
