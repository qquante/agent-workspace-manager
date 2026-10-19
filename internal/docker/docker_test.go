package docker

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"stola-os/internal/config"
	"stola-os/internal/mcp"
	"stola-os/internal/providers"
	"stola-os/internal/provisioner"
	"stola-os/internal/secrets"
	"stola-os/internal/state"
)

func TestCheckReadsDockerVersionAndSecurityOptions(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]string{
			"docker\x00version\x00--format\x00{{.Server.Version}}":    "29.4.3\n",
			"docker\x00info\x00--format\x00{{json .SecurityOptions}}": `["name=seccomp","name=cgroupns"]` + "\n",
		},
	}
	client := New(config.DockerConfig{}).WithRunner(runner)

	result, err := client.Check(context.Background())
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result.ServerVersion != "29.4.3" {
		t.Fatalf("unexpected server version: %q", result.ServerVersion)
	}
	if !strings.Contains(result.Security, "seccomp") {
		t.Fatalf("unexpected security options: %q", result.Security)
	}
}

func TestCreateGeneratesHardenedWorkspaceContainer(t *testing.T) {
	runner := &fakeRunner{
		failures: map[string]error{
			"docker\x00network\x00inspect\x00stola-net-nano-a":      errors.New("missing network"),
			"docker\x00volume\x00inspect\x00stola-workspace-nano-a": errors.New("missing volume"),
		},
		outputs: map[string]string{
			"docker\x00network\x00create\x00--driver\x00bridge\x00stola-net-nano-a": "netid\n",
			"docker\x00volume\x00create\x00stola-workspace-nano-a":                  "volid\n",
			"docker\x00run\x00-d\x00--name\x00stola-nano-a\x00--hostname\x00nano-a\x00--network\x00stola-net-nano-a\x00--memory\x00512m\x00--memory-swap\x00768m\x00--cpus\x001\x00--pids-limit\x00128\x00--cap-drop\x00ALL\x00--security-opt\x00no-new-privileges:true\x00--label\x00stola.managed=true\x00--label\x00stola.plan=nano\x00--label\x00stola.workspace=nano-a\x00-v\x00stola-workspace-nano-a:/workspace\x00-w\x00/workspace\x00ghcr.io/stola/agent-base:dev\x00sh\x00-lc\x00while true; do sleep 3600; done": "abc123\n",
		},
	}
	client := New(config.DockerConfig{}).WithRunner(runner)

	vps, err := client.Create(context.Background(), provisioner.CreateSpec{
		Name:      "Nano A",
		Plan:      "nano",
		Image:     "ghcr.io/stola/agent-base:dev",
		MemoryMB:  512,
		SwapMB:    256,
		Cores:     1,
		PIDsLimit: 128,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if vps.ID != "docker-stola-nano-a" || vps.ExternalID != "abc123" || vps.Status != "running" {
		t.Fatalf("unexpected VPS: %+v", vps)
	}
	if len(runner.calls) != 5 {
		t.Fatalf("unexpected call count: %d calls=%v", len(runner.calls), runner.calls)
	}
}

func TestCreateInjectsMCPBundleIntoWorkspace(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "default.yaml")
	if err := os.WriteFile(manifestPath, []byte(`
version: 1
name: inject-test
workspace:
  root: /workspace
policy:
  secrets_from_env_only: true
servers:
  - id: filesystem
    title: Filesystem
    default_enabled: true
    risk: medium
    transport: stdio
    command: [npx, -y, "@modelcontextprotocol/server-filesystem@2025.8.21", /workspace]
    permissions:
      secrets: []
optional_servers:
  - id: context7
    title: Context7
    default_enabled: false
    risk: low
    transport: http
    url: https://mcp.context7.com/mcp
    permissions:
      secrets:
        - CONTEXT7_API_KEY
`), 0o600); err != nil {
		t.Fatalf("WriteFile manifest returned error: %v", err)
	}
	runner := &fakeRunner{
		failures: map[string]error{
			"docker\x00network\x00inspect\x00stola-net-nano-a":      errors.New("missing network"),
			"docker\x00volume\x00inspect\x00stola-workspace-nano-a": errors.New("missing volume"),
		},
		outputs: map[string]string{
			"docker\x00network\x00create\x00--driver\x00bridge\x00stola-net-nano-a": "netid\n",
			"docker\x00volume\x00create\x00stola-workspace-nano-a":                  "volid\n",
			"docker\x00run\x00-d\x00--name\x00stola-nano-a\x00--hostname\x00nano-a\x00--network\x00stola-net-nano-a\x00--memory\x001024m\x00--memory-swap\x001024m\x00--cpus\x001\x00--pids-limit\x00256\x00--cap-drop\x00ALL\x00--security-opt\x00no-new-privileges:true\x00--label\x00stola.managed=true\x00--label\x00stola.plan=nano\x00--label\x00stola.workspace=nano-a\x00-v\x00stola-workspace-nano-a:/workspace\x00-w\x00/workspace\x00debian:bookworm-slim\x00sh\x00-lc\x00while true; do sleep 3600; done": "abc123\n",
		},
		prefixOutputs: map[string]string{
			"docker\x00exec\x00abc123\x00sh\x00-lc\x00": "",
		},
	}
	client := New(config.DockerConfig{}).WithRunner(runner)

	_, err := client.Create(context.Background(), provisioner.CreateSpec{
		Name:        "Nano A",
		InjectMCP:   true,
		MCPManifest: manifestPath,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	calls := strings.Join(runner.calls, "\n")
	for _, path := range []string{
		"/workspace/.stola/mcp/default.yaml",
		"/workspace/.stola/mcp/allowlist.json",
		"/workspace/.stola/mcp/hermes.config.yaml",
		"/workspace/.stola/mcp/openclaw.json",
		"/workspace/opencode.json",
		"/home/stola/.config/opencode/opencode.json",
		"/home/stola/.hermes/config.yaml",
		"/home/stola/.openclaw/openclaw.json",
	} {
		if !strings.Contains(calls, path) {
			t.Fatalf("expected injected file %s in docker exec calls:\n%s", path, calls)
		}
	}
	if !strings.Contains(calls, "base64 -d") {
		t.Fatalf("expected base64-based workspace writes:\n%s", calls)
	}
}

func TestCreateInjectsProviderBundleIntoWorkspace(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "providers.yaml")
	if err := os.WriteFile(manifestPath, []byte(`
version: 1
name: provider-inject-test
default_provider: openai
providers:
  - id: openai
    title: OpenAI
    kind: openai_compatible
    base_url: https://api.openai.com/v1
    api_key_env: OPENAI_API_KEY
    litellm_model: openai/gpt-5.5
    enabled: true
`), 0o600); err != nil {
		t.Fatalf("WriteFile manifest returned error: %v", err)
	}
	runner := &fakeRunner{
		failures: map[string]error{
			"docker\x00network\x00inspect\x00stola-net-nano-a":      errors.New("missing network"),
			"docker\x00volume\x00inspect\x00stola-workspace-nano-a": errors.New("missing volume"),
		},
		outputs: map[string]string{
			"docker\x00network\x00create\x00--driver\x00bridge\x00stola-net-nano-a": "netid\n",
			"docker\x00volume\x00create\x00stola-workspace-nano-a":                  "volid\n",
			"docker\x00run\x00-d\x00--name\x00stola-nano-a\x00--hostname\x00nano-a\x00--network\x00stola-net-nano-a\x00--memory\x001024m\x00--memory-swap\x001024m\x00--cpus\x001\x00--pids-limit\x00256\x00--cap-drop\x00ALL\x00--security-opt\x00no-new-privileges:true\x00--label\x00stola.managed=true\x00--label\x00stola.plan=nano\x00--label\x00stola.workspace=nano-a\x00-v\x00stola-workspace-nano-a:/workspace\x00-w\x00/workspace\x00debian:bookworm-slim\x00sh\x00-lc\x00while true; do sleep 3600; done": "abc123\n",
		},
		prefixOutputs: map[string]string{
			"docker\x00exec\x00abc123\x00sh\x00-lc\x00": "",
		},
	}
	client := New(config.DockerConfig{}).WithRunner(runner)

	_, err := client.Create(context.Background(), provisioner.CreateSpec{
		Name:              "Nano A",
		InjectProviders:   true,
		ProvidersManifest: manifestPath,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	calls := strings.Join(runner.calls, "\n")
	for _, path := range []string{
		"/workspace/" + providers.WorkspaceManifestPath,
		"/workspace/" + providers.WorkspaceCatalogPath,
		"/workspace/" + providers.WorkspaceEnvExamplePath,
		"/workspace/" + providers.WorkspaceLiteLLMConfigPath,
		"/workspace/" + providers.WorkspaceOpenAICompatPath,
		providers.ContainerLiteLLMConfigPath,
	} {
		if !strings.Contains(calls, path) {
			t.Fatalf("expected injected provider file %s in docker exec calls:\n%s", path, calls)
		}
	}
}

func TestCreateInjectsProviderSecretsViaStdin(t *testing.T) {
	dir := t.TempDir()
	providersPath := filepath.Join(dir, "providers.yaml")
	if err := os.WriteFile(providersPath, []byte(`
version: 1
name: provider-secret-test
default_provider: openai
providers:
  - id: openai
    title: OpenAI
    kind: openai_compatible
    base_url: https://api.openai.com/v1
    api_key_env: OPENAI_API_KEY
    litellm_model: openai/gpt-5.5
    enabled: true
`), 0o600); err != nil {
		t.Fatalf("WriteFile provider manifest returned error: %v", err)
	}
	secretsPath := filepath.Join(dir, "provider.env")
	if err := os.WriteFile(secretsPath, []byte("OPENAI_API_KEY=sk-test-secret\nUNRELATED_SECRET=ignore-me\n"), 0o600); err != nil {
		t.Fatalf("WriteFile secrets returned error: %v", err)
	}
	runner := &fakeRunner{
		failures: map[string]error{
			"docker\x00network\x00inspect\x00stola-net-nano-a":      errors.New("missing network"),
			"docker\x00volume\x00inspect\x00stola-workspace-nano-a": errors.New("missing volume"),
		},
		outputs: map[string]string{
			"docker\x00network\x00create\x00--driver\x00bridge\x00stola-net-nano-a": "netid\n",
			"docker\x00volume\x00create\x00stola-workspace-nano-a":                  "volid\n",
			"docker\x00run\x00-d\x00--name\x00stola-nano-a\x00--hostname\x00nano-a\x00--network\x00stola-net-nano-a\x00--memory\x001024m\x00--memory-swap\x001024m\x00--cpus\x001\x00--pids-limit\x00256\x00--cap-drop\x00ALL\x00--security-opt\x00no-new-privileges:true\x00--label\x00stola.managed=true\x00--label\x00stola.plan=nano\x00--label\x00stola.workspace=nano-a\x00-v\x00stola-workspace-nano-a:/workspace\x00-w\x00/workspace\x00debian:bookworm-slim\x00sh\x00-lc\x00while true; do sleep 3600; done": "abc123\n",
		},
		prefixOutputs: map[string]string{
			"docker\x00exec\x00abc123\x00sh\x00-lc\x00": "",
		},
		inputPrefixOutputs: map[string]string{
			"docker\x00exec\x00-i\x00abc123\x00sh\x00-lc\x00umask 077 && mkdir -p '/home/stola/.config/stola' && cat > ": "",
		},
	}
	client := New(config.DockerConfig{}).WithRunner(runner)

	_, err := client.Create(context.Background(), provisioner.CreateSpec{
		Name:              "Nano A",
		InjectProviders:   true,
		ProvidersManifest: providersPath,
		InjectSecrets:     true,
		SecretsFile:       secretsPath,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	calls := strings.Join(runner.calls, "\n")
	for _, path := range []string{
		"/workspace/" + secrets.WorkspaceManifestPath,
		secrets.ContainerSecretsEnvPath,
		secrets.ContainerSecretsManifestPath,
		secrets.ContainerProfilePath,
	} {
		if !strings.Contains(calls, path) {
			t.Fatalf("expected injected secret path %s in docker calls:\n%s", path, calls)
		}
	}
	if strings.Contains(calls, "sk-test-secret") || strings.Contains(calls, "ignore-me") {
		t.Fatalf("docker command arguments leaked secret values:\n%s", calls)
	}
	inputs := string(bytes.Join(runner.inputs, []byte("\n")))
	if !strings.Contains(inputs, "sk-test-secret") || strings.Contains(inputs, "UNRELATED_SECRET") {
		t.Fatalf("unexpected secret stdin payloads:\n%s", inputs)
	}
}

func TestCreateInjectsLLMProxyViaStdin(t *testing.T) {
	runner := &fakeRunner{
		failures: map[string]error{
			"docker\x00network\x00inspect\x00stola-net-nano-a":      errors.New("missing network"),
			"docker\x00volume\x00inspect\x00stola-workspace-nano-a": errors.New("missing volume"),
		},
		outputs: map[string]string{
			"docker\x00network\x00create\x00--driver\x00bridge\x00stola-net-nano-a": "netid\n",
			"docker\x00volume\x00create\x00stola-workspace-nano-a":                  "volid\n",
			"docker\x00run\x00-d\x00--name\x00stola-nano-a\x00--hostname\x00nano-a\x00--network\x00stola-net-nano-a\x00--memory\x001024m\x00--memory-swap\x001024m\x00--cpus\x001\x00--pids-limit\x00256\x00--cap-drop\x00ALL\x00--security-opt\x00no-new-privileges:true\x00--label\x00stola.managed=true\x00--label\x00stola.plan=nano\x00--label\x00stola.workspace=nano-a\x00-v\x00stola-workspace-nano-a:/workspace\x00-w\x00/workspace\x00debian:bookworm-slim\x00sh\x00-lc\x00while true; do sleep 3600; done": "abc123\n",
		},
		prefixOutputs: map[string]string{
			"docker\x00exec\x00abc123\x00sh\x00-lc\x00": "",
		},
		inputPrefixOutputs: map[string]string{
			"docker\x00exec\x00-i\x00abc123\x00sh\x00-lc\x00umask 077 && mkdir -p '/home/stola/.config/stola' && cat > ": "",
		},
	}
	client := New(config.DockerConfig{}).WithRunner(runner)
	token := "sk-stolas-test-virtual-key"

	_, err := client.Create(context.Background(), provisioner.CreateSpec{
		Name:             "Nano A",
		InjectLLMProxy:   true,
		LLMProxyURL:      "http://127.0.0.1:8760",
		LLMProxyKey:      token,
		LLMProxyKeyID:    "llmkey-a",
		LLMProxyProvider: "openrouter",
		LLMProxyModels:   []string{"openai/gpt-4o-mini"},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	calls := strings.Join(runner.calls, "\n")
	for _, path := range []string{
		"/workspace/" + WorkspaceLLMProxyConfigPath,
		providers.ContainerLiteLLMConfigPath,
		ContainerLLMProxyEnvPath,
		ContainerLLMProxyManifestPath,
		secrets.ContainerProfilePath,
	} {
		if !strings.Contains(calls, path) {
			t.Fatalf("expected injected llm proxy path %s in docker calls:\n%s", path, calls)
		}
	}
	if strings.Contains(calls, token) {
		t.Fatalf("docker command arguments leaked llm proxy token:\n%s", calls)
	}
	workspaceConfig, _, envFile, liteLLMConfig, err := renderLLMProxyFiles(LLMProxyInjection{
		URL:        "http://127.0.0.1:8760",
		Token:      token,
		ProviderID: "openrouter",
		Models:     []string{"openai/gpt-4o-mini"},
		KeyID:      "llmkey-a",
	})
	if err != nil {
		t.Fatalf("renderLLMProxyFiles returned error: %v", err)
	}
	if strings.Contains(string(workspaceConfig), token) {
		t.Fatalf("workspace config leaked llm proxy token: %s", string(workspaceConfig))
	}
	if !strings.Contains(string(liteLLMConfig), "model: 'openai/gpt-4o-mini'") || strings.Contains(string(liteLLMConfig), "openai/openai") {
		t.Fatalf("unexpected LiteLLM proxy config:\n%s", string(liteLLMConfig))
	}
	inputs := string(bytes.Join(runner.inputs, []byte("\n")))
	if !strings.Contains(inputs, token) || !strings.Contains(string(envFile), token) {
		t.Fatalf("llm proxy token was not written through secret stdin payloads:\n%s", inputs)
	}
}

func TestCreateReusesExistingNetworkAndVolume(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]string{
			"docker\x00network\x00inspect\x00stola-net-nano-a":      "{}\n",
			"docker\x00volume\x00inspect\x00stola-workspace-nano-a": "{}\n",
			"docker\x00run\x00-d\x00--name\x00stola-nano-a\x00--hostname\x00nano-a\x00--network\x00stola-net-nano-a\x00--memory\x001024m\x00--memory-swap\x001024m\x00--cpus\x001\x00--pids-limit\x00256\x00--cap-drop\x00ALL\x00--security-opt\x00no-new-privileges:true\x00--label\x00stola.managed=true\x00--label\x00stola.plan=nano\x00--label\x00stola.workspace=nano-a\x00-v\x00stola-workspace-nano-a:/workspace\x00-w\x00/workspace\x00debian:bookworm-slim\x00sh\x00-lc\x00while true; do sleep 3600; done": "def456\n",
		},
	}
	client := New(config.DockerConfig{}).WithRunner(runner)

	_, err := client.Create(context.Background(), provisioner.CreateSpec{Name: "nano-a"})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	wantCalls := []string{
		"docker\x00network\x00inspect\x00stola-net-nano-a",
		"docker\x00volume\x00inspect\x00stola-workspace-nano-a",
		"docker\x00run\x00-d\x00--name\x00stola-nano-a\x00--hostname\x00nano-a\x00--network\x00stola-net-nano-a\x00--memory\x001024m\x00--memory-swap\x001024m\x00--cpus\x001\x00--pids-limit\x00256\x00--cap-drop\x00ALL\x00--security-opt\x00no-new-privileges:true\x00--label\x00stola.managed=true\x00--label\x00stola.plan=nano\x00--label\x00stola.workspace=nano-a\x00-v\x00stola-workspace-nano-a:/workspace\x00-w\x00/workspace\x00debian:bookworm-slim\x00sh\x00-lc\x00while true; do sleep 3600; done",
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("unexpected calls:\ngot  %#v\nwant %#v", runner.calls, wantCalls)
	}
}

func TestLifecycleCommandsUseStoredContainerID(t *testing.T) {
	runner := &fakeRunner{
		outputs: map[string]string{
			"docker\x00inspect\x00--format\x00{{.State.Status}}\x00abc123": "running\n",
			"docker\x00logs\x00--tail\x0050\x00abc123":                     "hello\n",
			"docker\x00exec\x00abc123\x00sh\x00-lc\x00echo ok":             "ok\n",
			"docker\x00stop\x00abc123":                                     "abc123\n",
			"docker\x00start\x00abc123":                                    "abc123\n",
			"docker\x00rm\x00-f\x00abc123":                                 "abc123\n",
			"docker\x00network\x00inspect\x00stola-net-nano-a":             "{}\n",
			"docker\x00network\x00rm\x00stola-net-nano-a":                  "stola-net-nano-a\n",
			"docker\x00volume\x00inspect\x00stola-workspace-nano-a":        "{}\n",
			"docker\x00volume\x00rm\x00stola-workspace-nano-a":             "stola-workspace-nano-a\n",
		},
	}
	client := New(config.DockerConfig{}).WithRunner(runner)
	vps := stateVPS("nano-a", "abc123")

	status, err := client.Status(context.Background(), vps)
	if err != nil || status != "running" {
		t.Fatalf("Status = %q, %v", status, err)
	}
	logs, err := client.Logs(context.Background(), vps, 50)
	if err != nil || logs != "hello\n" {
		t.Fatalf("Logs = %q, %v", logs, err)
	}
	out, err := client.Exec(context.Background(), vps, []string{"sh", "-lc", "echo ok"})
	if err != nil || out != "ok\n" {
		t.Fatalf("Exec = %q, %v", out, err)
	}
	if err := client.Stop(context.Background(), vps); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
	if err := client.Start(context.Background(), vps); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := client.Delete(context.Background(), vps); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
}

func TestMCPListToolsAndCallUseDockerExecWithStdin(t *testing.T) {
	runner := &fakeRunner{
		inputOutputs: map[string]string{
			"docker\x00exec\x00-i\x00-w\x00/workspace\x00abc123\x00fake-mcp": `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{}}}}
{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"read_file","description":"Read file"}]}}
`,
			"docker\x00exec\x00-i\x00-w\x00/workspace\x00abc123\x00fake-mcp\x00--call": `{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","capabilities":{"tools":{}}}}
{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"ok"}],"isError":false}}
`,
		},
	}
	client := New(config.DockerConfig{}).WithRunner(runner)
	vps := stateVPS("nano-a", "abc123")

	list, err := client.MCPListTools(context.Background(), vps, mcp.Server{
		ID:        "fake",
		Transport: "stdio",
		Command:   []string{"fake-mcp"},
	}, time.Second)
	if err != nil {
		t.Fatalf("MCPListTools returned error: %v", err)
	}
	if len(list.Tools) != 1 || list.Tools[0].Name != "read_file" {
		t.Fatalf("unexpected tools: %#v", list.Tools)
	}

	call, err := client.MCPCallTool(context.Background(), vps, mcp.Server{
		ID:        "fake",
		Transport: "stdio",
		Command:   []string{"fake-mcp", "--call"},
	}, "read_file", []byte(`{"path":"README.md"}`), time.Second)
	if err != nil {
		t.Fatalf("MCPCallTool returned error: %v", err)
	}
	if len(call.Content) != 1 || call.Content[0]["text"] != "ok" {
		t.Fatalf("unexpected call result: %#v", call)
	}

	if len(runner.inputs) != 2 {
		t.Fatalf("expected 2 stdin payloads, got %d", len(runner.inputs))
	}
	if !strings.Contains(string(runner.inputs[0]), `"method":"tools/list"`) {
		t.Fatalf("tools/list stdin missing method: %s", string(runner.inputs[0]))
	}
	if !strings.Contains(string(runner.inputs[1]), `"method":"tools/call"`) {
		t.Fatalf("tools/call stdin missing method: %s", string(runner.inputs[1]))
	}
}

func stateVPS(name string, externalID string) state.VPS {
	return state.VPS{
		ID:         "docker-stola-" + strings.ReplaceAll(name, " ", "-"),
		Name:       name,
		Plan:       "nano",
		Status:     "running",
		Backend:    "docker",
		ExternalID: externalID,
	}
}

type fakeRunner struct {
	calls              []string
	inputs             [][]byte
	outputs            map[string]string
	inputOutputs       map[string]string
	prefixOutputs      map[string]string
	inputPrefixOutputs map[string]string
	failures           map[string]error
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := strings.Join(append([]string{name}, args...), "\x00")
	r.calls = append(r.calls, key)
	if err, ok := r.failures[key]; ok {
		return nil, err
	}
	if out, ok := r.outputs[key]; ok {
		return []byte(out), nil
	}
	for prefix, out := range r.prefixOutputs {
		if strings.HasPrefix(key, prefix) {
			return []byte(out), nil
		}
	}
	return nil, errors.New("unexpected docker command: " + strings.ReplaceAll(key, "\x00", " "))
}

func (r *fakeRunner) RunWithInput(_ context.Context, input []byte, name string, args ...string) ([]byte, error) {
	key := strings.Join(append([]string{name}, args...), "\x00")
	r.calls = append(r.calls, key)
	r.inputs = append(r.inputs, append([]byte(nil), input...))
	if err, ok := r.failures[key]; ok {
		return nil, err
	}
	if out, ok := r.inputOutputs[key]; ok {
		return []byte(out), nil
	}
	for prefix, out := range r.inputPrefixOutputs {
		if strings.HasPrefix(key, prefix) {
			return []byte(out), nil
		}
	}
	return nil, errors.New("unexpected docker stdin command: " + strings.ReplaceAll(key, "\x00", " "))
}
