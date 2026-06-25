package docker

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"stola-os/internal/config"
	"stola-os/internal/mcp"
	"stola-os/internal/providers"
	"stola-os/internal/provisioner"
	"stola-os/internal/secrets"
	"stola-os/internal/state"
)

const (
	defaultCLIPath       = "docker"
	defaultImage         = "debian:bookworm-slim"
	defaultNetworkPrefix = "stola-net"
	defaultVolumePrefix  = "stola-workspace"
	defaultPIDsLimit     = 256

	WorkspaceLLMProxyConfigPath   = ".stola/llm-proxy/config.json"
	ContainerLLMProxyEnvPath      = "/home/stola/.config/stola/llm-proxy.env"
	ContainerLLMProxyManifestPath = "/home/stola/.config/stola/llm-proxy.json"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type CombinedRunner interface {
	RunCombined(ctx context.Context, name string, args ...string) ([]byte, error)
}

type InputRunner interface {
	RunWithInput(ctx context.Context, input []byte, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		text := strings.TrimSpace(stderr.String())
		if text != "" {
			return out, fmt.Errorf("%w: %s", err, text)
		}
		return out, err
	}
	return out, nil
}

func (ExecRunner) RunCombined(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text != "" {
			return out, fmt.Errorf("%w: %s", err, text)
		}
		return out, err
	}
	return out, nil
}

func (ExecRunner) RunWithInput(ctx context.Context, input []byte, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		text := strings.TrimSpace(stderr.String())
		if text != "" {
			return out, fmt.Errorf("%w: %s", err, text)
		}
		return out, err
	}
	return out, nil
}

type Client struct {
	Config config.DockerConfig
	Runner Runner
}

type CheckResult struct {
	ServerVersion string
	Security      string
}

type LLMProxyInjection struct {
	URL        string
	Token      string
	ProviderID string
	Models     []string
	KeyID      string
}

func New(cfg config.DockerConfig) Client {
	return Client{Config: cfg, Runner: ExecRunner{}}
}

func (c Client) WithRunner(r Runner) Client {
	c.Runner = r
	return c
}

func (c Client) Type() string {
	return "docker"
}

func (c Client) Validate() error {
	if strings.TrimSpace(c.cliPath()) == "" {
		return errors.New("docker cli path is required")
	}
	return nil
}

func (c Client) Check(ctx context.Context) (CheckResult, error) {
	version, err := c.run(ctx, "version", "--format", "{{.Server.Version}}")
	if err != nil {
		return CheckResult{}, err
	}
	security, err := c.run(ctx, "info", "--format", "{{json .SecurityOptions}}")
	if err != nil {
		return CheckResult{}, err
	}
	return CheckResult{
		ServerVersion: strings.TrimSpace(string(version)),
		Security:      strings.TrimSpace(string(security)),
	}, nil
}

func (c Client) Create(ctx context.Context, spec provisioner.CreateSpec) (state.VPS, error) {
	resolved := resolveSpec(c.Config, spec)
	if err := resolved.validate(); err != nil {
		return state.VPS{}, err
	}
	if err := c.ensureNetwork(ctx, resolved.networkName); err != nil {
		return state.VPS{}, err
	}
	if err := c.ensureVolume(ctx, resolved.volumeName); err != nil {
		return state.VPS{}, err
	}

	args := []string{
		"run",
		"-d",
		"--name", resolved.containerName,
		"--hostname", resolved.hostname,
		"--network", resolved.networkName,
		"--memory", fmt.Sprintf("%dm", resolved.memoryMB),
		"--memory-swap", fmt.Sprintf("%dm", resolved.memoryMB+resolved.swapMB),
		"--cpus", strconv.Itoa(resolved.cores),
		"--pids-limit", strconv.Itoa(resolved.pidsLimit),
		"--cap-drop", "ALL",
		"--security-opt", "no-new-privileges:true",
		"--label", "stola.managed=true",
		"--label", fmt.Sprintf("stola.plan=%s", resolved.plan),
		"--label", fmt.Sprintf("stola.workspace=%s", resolved.hostname),
		"-v", fmt.Sprintf("%s:/workspace", resolved.volumeName),
		"-w", "/workspace",
		resolved.image,
		"sh", "-lc", "while true; do sleep 3600; done",
	}
	out, err := c.run(ctx, args...)
	if err != nil {
		return state.VPS{}, err
	}
	containerID := strings.TrimSpace(string(out))
	now := time.Now().UTC()
	vps := state.VPS{
		ID:         "docker-" + resolved.containerName,
		Name:       resolved.name,
		Plan:       resolved.plan,
		Status:     "running",
		Backend:    c.Type(),
		CreatedAt:  now,
		ExternalID: containerID,
		TaskID:     "",
	}
	if spec.InjectMCP {
		manifestPath := mcp.ResolveManifestPath(spec.MCPManifest)
		if err := c.InjectMCP(ctx, vps, manifestPath); err != nil {
			c.cleanupCreated(ctx, resolved, containerID)
			return state.VPS{}, fmt.Errorf("mcp injection failed: %w", err)
		}
	}
	if spec.InjectProviders {
		manifestPath := providers.ResolveManifestPath(spec.ProvidersManifest)
		if err := c.InjectProviders(ctx, vps, manifestPath); err != nil {
			c.cleanupCreated(ctx, resolved, containerID)
			return state.VPS{}, fmt.Errorf("provider injection failed: %w", err)
		}
	}
	if spec.InjectSecrets {
		manifestPath := providers.ResolveManifestPath(spec.ProvidersManifest)
		if err := c.InjectSecrets(ctx, vps, spec.SecretsFile, manifestPath); err != nil {
			c.cleanupCreated(ctx, resolved, containerID)
			return state.VPS{}, fmt.Errorf("secret injection failed: %w", err)
		}
	}
	if spec.InjectLLMProxy {
		if err := c.InjectLLMProxy(ctx, vps, LLMProxyInjection{
			URL:        spec.LLMProxyURL,
			Token:      spec.LLMProxyKey,
			ProviderID: spec.LLMProxyProvider,
			Models:     spec.LLMProxyModels,
			KeyID:      spec.LLMProxyKeyID,
		}); err != nil {
			c.cleanupCreated(ctx, resolved, containerID)
			return state.VPS{}, fmt.Errorf("llm proxy injection failed: %w", err)
		}
	}
	return vps, nil
}

func (c Client) InjectMCP(ctx context.Context, vps state.VPS, manifestPath string) error {
	bundle, err := mcp.LoadAndRender(manifestPath)
	if err != nil {
		return err
	}
	if err := c.WriteWorkspaceFiles(ctx, vps, bundle.Files); err != nil {
		return err
	}
	return c.WriteContainerFiles(ctx, vps, map[string][]byte{
		mcp.ContainerOpenCodeConfigPath: bundle.Files[mcp.OpenCodeConfigPath],
		mcp.ContainerHermesConfigPath:   bundle.Files[mcp.HermesConfigPath],
		mcp.ContainerOpenClawConfigPath: bundle.Files[mcp.OpenClawConfigPath],
	})
}

func (c Client) InjectProviders(ctx context.Context, vps state.VPS, manifestPath string) error {
	bundle, err := providers.LoadAndRender(manifestPath)
	if err != nil {
		return err
	}
	if err := c.WriteWorkspaceFiles(ctx, vps, bundle.Files); err != nil {
		return err
	}
	return c.WriteContainerFiles(ctx, vps, map[string][]byte{
		providers.ContainerLiteLLMConfigPath: bundle.Files[providers.WorkspaceLiteLLMConfigPath],
	})
}

func (c Client) InjectSecrets(ctx context.Context, vps state.VPS, secretsFile string, providersManifestPath string) error {
	manifest, _, err := providers.LoadFile(providersManifestPath)
	if err != nil {
		return err
	}
	bundle, err := secrets.LoadProviderEnvFile(secretsFile, providers.EnvNames(manifest))
	if err != nil {
		return err
	}
	if err := c.WriteWorkspaceFiles(ctx, vps, map[string][]byte{
		secrets.WorkspaceManifestPath: bundle.Files[secrets.WorkspaceManifestPath],
	}); err != nil {
		return err
	}
	if err := c.WriteContainerSecretFiles(ctx, vps, map[string][]byte{
		secrets.ContainerSecretsEnvPath:      bundle.Files[secrets.ContainerSecretsEnvPath],
		secrets.ContainerSecretsManifestPath: bundle.Files[secrets.ContainerSecretsManifestPath],
	}); err != nil {
		return err
	}
	return c.ensureSecretsProfileSource(ctx, vps)
}

func (c Client) InjectLLMProxy(ctx context.Context, vps state.VPS, injection LLMProxyInjection) error {
	if err := injection.validate(); err != nil {
		return err
	}
	workspaceConfig, containerManifest, envFile, liteLLMConfig, err := renderLLMProxyFiles(injection)
	if err != nil {
		return err
	}
	if err := c.WriteWorkspaceFiles(ctx, vps, map[string][]byte{
		WorkspaceLLMProxyConfigPath: workspaceConfig,
	}); err != nil {
		return err
	}
	if err := c.WriteContainerFiles(ctx, vps, map[string][]byte{
		providers.ContainerLiteLLMConfigPath: liteLLMConfig,
	}); err != nil {
		return err
	}
	if err := c.WriteContainerSecretFiles(ctx, vps, map[string][]byte{
		ContainerLLMProxyEnvPath:      envFile,
		ContainerLLMProxyManifestPath: containerManifest,
	}); err != nil {
		return err
	}
	return c.ensureProfileSource(ctx, vps, ContainerLLMProxyEnvPath)
}

func (c Client) WriteWorkspaceFiles(ctx context.Context, vps state.VPS, files map[string][]byte) error {
	target, err := containerTarget(vps)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sortStrings(names)
	for _, name := range names {
		if err := validateWorkspacePath(name); err != nil {
			return err
		}
		clean := path.Clean(strings.ReplaceAll(name, "\\", "/"))
		if err := c.writeContainerFile(ctx, target, "/workspace/"+clean, files[name]); err != nil {
			return err
		}
	}
	return nil
}

func (c Client) WriteContainerFiles(ctx context.Context, vps state.VPS, files map[string][]byte) error {
	target, err := containerTarget(vps)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sortStrings(names)
	for _, name := range names {
		if err := validateContainerConfigPath(name); err != nil {
			return err
		}
		if err := c.writeContainerFile(ctx, target, name, files[name]); err != nil {
			return err
		}
	}
	return nil
}

func (c Client) WriteContainerSecretFiles(ctx context.Context, vps state.VPS, files map[string][]byte) error {
	target, err := containerTarget(vps)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sortStrings(names)
	for _, name := range names {
		if err := validateContainerSecretPath(name); err != nil {
			return err
		}
		if err := c.writeContainerFileWithInput(ctx, target, name, files[name], 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (c Client) writeContainerFile(ctx context.Context, target string, containerPath string, content []byte) error {
	dir := path.Dir(containerPath)
	encoded := base64.StdEncoding.EncodeToString(content)
	script := fmt.Sprintf("mkdir -p %s && printf %%s %s | base64 -d > %s",
		shellQuote(dir),
		shellQuote(encoded),
		shellQuote(containerPath),
	)
	_, err := c.run(ctx, "exec", target, "sh", "-lc", script)
	return err
}

func (c Client) writeContainerFileWithInput(ctx context.Context, target string, containerPath string, content []byte, mode int) error {
	dir := path.Dir(containerPath)
	script := fmt.Sprintf("umask 077 && mkdir -p %s && cat > %s && chmod %03o %s",
		shellQuote(dir),
		shellQuote(containerPath),
		mode,
		shellQuote(containerPath),
	)
	_, err := c.runWithInput(ctx, content, "exec", "-i", target, "sh", "-lc", script)
	return err
}

func (c Client) ensureSecretsProfileSource(ctx context.Context, vps state.VPS) error {
	return c.ensureProfileSource(ctx, vps, secrets.ContainerSecretsEnvPath)
}

func (c Client) ensureProfileSource(ctx context.Context, vps state.VPS, sourcePath string) error {
	target, err := containerTarget(vps)
	if err != nil {
		return err
	}
	line := ". " + sourcePath
	script := fmt.Sprintf("touch %s && grep -qxF %s %s || printf '\\n%%s\\n' %s >> %s",
		shellQuote(secrets.ContainerProfilePath),
		shellQuote(line),
		shellQuote(secrets.ContainerProfilePath),
		shellQuote(line),
		shellQuote(secrets.ContainerProfilePath),
	)
	_, err = c.run(ctx, "exec", target, "sh", "-lc", script)
	return err
}

func (c Client) Status(ctx context.Context, vps state.VPS) (string, error) {
	target, err := containerTarget(vps)
	if err != nil {
		return "", err
	}
	out, err := c.run(ctx, "inspect", "--format", "{{.State.Status}}", target)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (c Client) Logs(ctx context.Context, vps state.VPS, tail int) (string, error) {
	target, err := containerTarget(vps)
	if err != nil {
		return "", err
	}
	if tail <= 0 {
		tail = 100
	}
	out, err := c.runCombined(ctx, "logs", "--tail", strconv.Itoa(tail), target)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (c Client) Exec(ctx context.Context, vps state.VPS, command []string) (string, error) {
	target, err := containerTarget(vps)
	if err != nil {
		return "", err
	}
	if len(command) == 0 {
		return "", errors.New("exec command is required")
	}
	args := append([]string{"exec", target}, command...)
	out, err := c.runCombined(ctx, args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (c Client) MCPListTools(ctx context.Context, vps state.VPS, server mcp.Server, timeout time.Duration) (mcp.ToolsListResult, error) {
	input, err := mcp.BuildToolsListInput()
	if err != nil {
		return mcp.ToolsListResult{}, err
	}
	out, err := c.runMCPServer(ctx, vps, server, input, timeout)
	if err != nil {
		return mcp.ToolsListResult{}, err
	}
	return mcp.ParseToolsListOutput(out)
}

func (c Client) MCPCallTool(ctx context.Context, vps state.VPS, server mcp.Server, toolName string, arguments []byte, timeout time.Duration) (mcp.ToolCallResult, error) {
	input, err := mcp.BuildToolCallInput(toolName, arguments)
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	out, err := c.runMCPServer(ctx, vps, server, input, timeout)
	if err != nil {
		return mcp.ToolCallResult{}, err
	}
	return mcp.ParseToolCallOutput(out)
}

func (c Client) Stop(ctx context.Context, vps state.VPS) error {
	target, err := containerTarget(vps)
	if err != nil {
		return err
	}
	_, err = c.run(ctx, "stop", target)
	return err
}

func (c Client) Start(ctx context.Context, vps state.VPS) error {
	target, err := containerTarget(vps)
	if err != nil {
		return err
	}
	_, err = c.run(ctx, "start", target)
	return err
}

func (c Client) Delete(ctx context.Context, vps state.VPS) error {
	target, err := containerTarget(vps)
	if err != nil {
		return err
	}
	if _, err := c.run(ctx, "rm", "-f", target); err != nil {
		return err
	}
	resolved := resolveSpec(c.Config, provisioner.CreateSpec{Name: vps.Name})
	if err := c.removeObjectIfExists(ctx, "network", resolved.networkName); err != nil {
		return err
	}
	if err := c.removeObjectIfExists(ctx, "volume", resolved.volumeName); err != nil {
		return err
	}
	return nil
}

func (c Client) ensureNetwork(ctx context.Context, name string) error {
	if _, err := c.run(ctx, "network", "inspect", name); err == nil {
		return nil
	}
	_, err := c.run(ctx, "network", "create", "--driver", "bridge", name)
	return err
}

func (c Client) ensureVolume(ctx context.Context, name string) error {
	if _, err := c.run(ctx, "volume", "inspect", name); err == nil {
		return nil
	}
	_, err := c.run(ctx, "volume", "create", name)
	return err
}

func (c Client) removeObjectIfExists(ctx context.Context, kind string, name string) error {
	if _, err := c.run(ctx, kind, "inspect", name); err != nil {
		return nil
	}
	_, err := c.run(ctx, kind, "rm", name)
	return err
}

func (c Client) cleanupCreated(ctx context.Context, resolved resolvedCreateSpec, containerID string) {
	if strings.TrimSpace(containerID) != "" {
		_, _ = c.run(ctx, "rm", "-f", containerID)
	}
	_ = c.removeObjectIfExists(ctx, "network", resolved.networkName)
	_ = c.removeObjectIfExists(ctx, "volume", resolved.volumeName)
}

func (c Client) run(ctx context.Context, args ...string) ([]byte, error) {
	if c.Runner == nil {
		c.Runner = ExecRunner{}
	}
	return c.Runner.Run(ctx, c.cliPath(), args...)
}

func (c Client) runCombined(ctx context.Context, args ...string) ([]byte, error) {
	if c.Runner == nil {
		c.Runner = ExecRunner{}
	}
	if runner, ok := c.Runner.(CombinedRunner); ok {
		return runner.RunCombined(ctx, c.cliPath(), args...)
	}
	return c.Runner.Run(ctx, c.cliPath(), args...)
}

func (c Client) runWithInput(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	if c.Runner == nil {
		c.Runner = ExecRunner{}
	}
	if runner, ok := c.Runner.(InputRunner); ok {
		return runner.RunWithInput(ctx, input, c.cliPath(), args...)
	}
	return nil, errors.New("docker runner does not support stdin")
}

func (c Client) cliPath() string {
	if strings.TrimSpace(c.Config.CLIPath) != "" {
		return c.Config.CLIPath
	}
	return defaultCLIPath
}

type resolvedCreateSpec struct {
	name          string
	plan          string
	image         string
	hostname      string
	containerName string
	networkName   string
	volumeName    string
	memoryMB      int
	swapMB        int
	cores         int
	pidsLimit     int
}

func resolveSpec(cfg config.DockerConfig, spec provisioner.CreateSpec) resolvedCreateSpec {
	name := spec.Name
	if name == "" {
		name = "stola-nano"
	}
	plan := spec.Plan
	if plan == "" {
		plan = "nano"
	}
	image := spec.Image
	if image == "" {
		image = cfg.Image
	}
	if image == "" {
		image = defaultImage
	}
	networkPrefix := cfg.NetworkPrefix
	if networkPrefix == "" {
		networkPrefix = defaultNetworkPrefix
	}
	volumePrefix := cfg.VolumePrefix
	if volumePrefix == "" {
		volumePrefix = defaultVolumePrefix
	}
	memoryMB := spec.MemoryMB
	if memoryMB == 0 {
		memoryMB = 1024
	}
	swapMB := spec.SwapMB
	if swapMB < 0 {
		swapMB = 0
	}
	cores := spec.Cores
	if cores == 0 {
		cores = 1
	}
	pidsLimit := spec.PIDsLimit
	if pidsLimit == 0 {
		pidsLimit = defaultPIDsLimit
	}
	safeName := safeDockerName(name)
	return resolvedCreateSpec{
		name:          name,
		plan:          plan,
		image:         image,
		hostname:      safeName,
		containerName: "stola-" + safeName,
		networkName:   networkPrefix + "-" + safeName,
		volumeName:    volumePrefix + "-" + safeName,
		memoryMB:      memoryMB,
		swapMB:        swapMB,
		cores:         cores,
		pidsLimit:     pidsLimit,
	}
}

func (i LLMProxyInjection) validate() error {
	var problems []string
	if strings.TrimSpace(i.URL) == "" {
		problems = append(problems, "llm proxy url is required")
	}
	if strings.TrimSpace(i.Token) == "" {
		problems = append(problems, "llm proxy token is required")
	}
	if strings.TrimSpace(i.ProviderID) == "" {
		problems = append(problems, "llm proxy provider is required")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func renderLLMProxyFiles(injection LLMProxyInjection) ([]byte, []byte, []byte, []byte, error) {
	url := strings.TrimRight(strings.TrimSpace(injection.URL), "/")
	providerID := strings.TrimSpace(injection.ProviderID)
	models := normalizeStrings(injection.Models)
	defaultModel := "stolas-default"
	if len(models) > 0 {
		defaultModel = models[0]
	}
	liteLLMModel := defaultModel
	if !strings.Contains(liteLLMModel, "/") {
		liteLLMModel = "openai/" + liteLLMModel
	}
	openAIBaseURL := url + "/v1"
	workspaceConfig, err := json.MarshalIndent(map[string]any{
		"version":         1,
		"proxy_url":       url,
		"openai_base_url": openAIBaseURL,
		"provider_id":     providerID,
		"models":          models,
		"default_model":   defaultModel,
		"key_id":          strings.TrimSpace(injection.KeyID),
	}, "", "  ")
	if err != nil {
		return nil, nil, nil, nil, err
	}
	containerManifest, err := json.MarshalIndent(map[string]any{
		"version":         1,
		"proxy_url":       url,
		"openai_base_url": openAIBaseURL,
		"provider_id":     providerID,
		"models":          models,
		"default_model":   defaultModel,
		"key_id":          strings.TrimSpace(injection.KeyID),
		"secret_env":      []string{"STOLAS_LLM_PROXY_KEY", "OPENAI_API_KEY", "LITELLM_API_KEY"},
	}, "", "  ")
	if err != nil {
		return nil, nil, nil, nil, err
	}
	envFile := []byte(strings.Join([]string{
		"# Generated by Stolas. Contains virtual workspace credentials.",
		envLine("STOLAS_LLM_PROXY_URL", url),
		envLine("STOLAS_LLM_PROXY_KEY", strings.TrimSpace(injection.Token)),
		envLine("STOLAS_LLM_PROXY_PROVIDER", providerID),
		envLine("STOLAS_LLM_PROXY_MODEL", defaultModel),
		envLine("OPENAI_BASE_URL", openAIBaseURL),
		envLine("OPENAI_API_BASE", openAIBaseURL),
		envLine("OPENAI_API_KEY", strings.TrimSpace(injection.Token)),
		envLine("LITELLM_BASE_URL", openAIBaseURL),
		envLine("LITELLM_API_KEY", strings.TrimSpace(injection.Token)),
		"",
	}, "\n"))
	liteLLMConfig := []byte(strings.Join([]string{
		"model_list:",
		"  - model_name: stolas",
		"    litellm_params:",
		"      model: " + yamlSingleQuote(liteLLMModel),
		"      api_base: " + yamlSingleQuote(openAIBaseURL),
		"      api_key: os.environ/OPENAI_API_KEY",
		"general_settings:",
		"  master_key: os.environ/LITELLM_API_KEY",
		"",
	}, "\n"))
	return workspaceConfig, containerManifest, envFile, liteLLMConfig, nil
}

func normalizeStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sortStrings(out)
	return out
}

func envLine(name string, value string) string {
	return name + "='" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func yamlSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func (s resolvedCreateSpec) validate() error {
	var problems []string
	if strings.TrimSpace(s.name) == "" {
		problems = append(problems, "name is required")
	}
	if strings.TrimSpace(s.image) == "" {
		problems = append(problems, "docker image is required")
	}
	if s.memoryMB <= 0 {
		problems = append(problems, "memory must be positive")
	}
	if s.swapMB < 0 {
		problems = append(problems, "swap cannot be negative")
	}
	if s.cores <= 0 {
		problems = append(problems, "cores must be positive")
	}
	if s.pidsLimit <= 0 {
		problems = append(problems, "pids limit must be positive")
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

var unsafeDockerName = regexp.MustCompile(`[^a-zA-Z0-9_.-]+`)

func safeDockerName(name string) string {
	out := strings.Trim(unsafeDockerName.ReplaceAllString(strings.ToLower(name), "-"), "-_.")
	if out == "" {
		return "workspace"
	}
	return out
}

func containerTarget(vps state.VPS) (string, error) {
	if strings.TrimSpace(vps.ExternalID) != "" {
		return strings.TrimSpace(vps.ExternalID), nil
	}
	if strings.TrimSpace(vps.Name) != "" {
		return "stola-" + safeDockerName(vps.Name), nil
	}
	if strings.HasPrefix(vps.ID, "docker-") {
		return strings.TrimPrefix(vps.ID, "docker-"), nil
	}
	return "", errors.New("docker container id/name is required")
}

func (c Client) runMCPServer(ctx context.Context, vps state.VPS, server mcp.Server, input []byte, timeout time.Duration) ([]byte, error) {
	target, err := containerTarget(vps)
	if err != nil {
		return nil, err
	}
	if server.Transport != "stdio" {
		return nil, fmt.Errorf("mcp server %s transport %s is not executable via Docker stdio", server.ID, server.Transport)
	}
	if len(server.Command) == 0 {
		return nil, fmt.Errorf("mcp server %s command is required", server.ID)
	}
	runCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()
	args := []string{"exec", "-i", "-w", "/workspace", target}
	args = append(args, server.Command...)
	return c.runWithInput(runCtx, input, args...)
}

func validateWorkspacePath(filePath string) error {
	clean := path.Clean(strings.ReplaceAll(filePath, "\\", "/"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") || strings.Contains(clean, "\x00") {
		return fmt.Errorf("invalid workspace path: %q", filePath)
	}
	return nil
}

func validateContainerConfigPath(filePath string) error {
	clean := path.Clean(strings.ReplaceAll(filePath, "\\", "/"))
	switch clean {
	case mcp.ContainerOpenCodeConfigPath, mcp.ContainerHermesConfigPath, mcp.ContainerOpenClawConfigPath, providers.ContainerLiteLLMConfigPath:
		return nil
	default:
		return fmt.Errorf("invalid container config path: %q", filePath)
	}
}

func validateContainerSecretPath(filePath string) error {
	clean := path.Clean(strings.ReplaceAll(filePath, "\\", "/"))
	switch clean {
	case secrets.ContainerSecretsEnvPath, secrets.ContainerSecretsManifestPath, ContainerLLMProxyEnvPath, ContainerLLMProxyManifestPath:
		return nil
	default:
		return fmt.Errorf("invalid container secret path: %q", filePath)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
