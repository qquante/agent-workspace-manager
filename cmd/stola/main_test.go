package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"stola-os/internal/config"
)

func TestCLIConfigValidateAndDoctor(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "stola.yaml")
	cfg := config.Default()
	cfg.Node.ID = "cli-node"
	cfg.Paths.State = filepath.Join(dir, "stola.db")
	cfg.Paths.AuditLog = filepath.Join(dir, "audit.jsonl")
	cfg.Paths.RuntimeDir = filepath.Join(dir, "run")
	if err := config.WriteFile(configPath, cfg, false); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"--config", configPath, "config", "validate"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("config validate failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "config valid") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run(context.Background(), []string{"--config", configPath, "doctor"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("doctor failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "backend: mock") {
		t.Fatalf("doctor did not report mock backend: %s", stdout.String())
	}
}

func TestCLIMCPValidateListAndRender(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "manifests", "mcp", "default.yaml")
	workspace := filepath.Join(t.TempDir(), "workspace")

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"mcp", "validate", "--manifest", manifestPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("mcp validate failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "stola-default-mcp") {
		t.Fatalf("unexpected validate stdout: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run(context.Background(), []string{"mcp", "list", "--manifest", manifestPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("mcp list failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "id=filesystem") || !strings.Contains(stdout.String(), "id=context7") {
		t.Fatalf("unexpected list stdout: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run(context.Background(), []string{"mcp", "render", "--manifest", manifestPath, "--workspace", workspace}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("mcp render failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	for _, rel := range []string{
		"opencode.json",
		filepath.Join(".stola", "mcp", "allowlist.json"),
		filepath.Join(".stola", "mcp", "hermes.config.yaml"),
		filepath.Join(".stola", "mcp", "openclaw.json"),
	} {
		if _, err := os.Stat(filepath.Join(workspace, rel)); err != nil {
			t.Fatalf("expected rendered file %s: %v", rel, err)
		}
	}
}

func TestCLIProvidersValidateListAndRender(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "manifests", "providers", "default.yaml")
	workspace := filepath.Join(t.TempDir(), "workspace")

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"providers", "validate", "--manifest", manifestPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("providers validate failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "stola-default-providers") || !strings.Contains(stdout.String(), "default=openrouter") {
		t.Fatalf("unexpected validate stdout: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run(context.Background(), []string{"providers", "list", "--manifest", manifestPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("providers list failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "id=openrouter") || !strings.Contains(stdout.String(), "id=openai") || !strings.Contains(stdout.String(), "id=anthropic") {
		t.Fatalf("unexpected list stdout: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "sk-") {
		t.Fatalf("providers list exposed a secret-looking value: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run(context.Background(), []string{"providers", "render", "--manifest", manifestPath, "--workspace", workspace}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("providers render failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	for _, rel := range []string{
		filepath.Join(".stola", "providers", "default.yaml"),
		filepath.Join(".stola", "providers", "catalog.json"),
		filepath.Join(".stola", "providers", "env.example"),
		filepath.Join(".stola", "providers", "litellm.config.yaml"),
		filepath.Join(".stola", "providers", "openai-compatible.json"),
	} {
		if _, err := os.Stat(filepath.Join(workspace, rel)); err != nil {
			t.Fatalf("expected rendered provider file %s: %v", rel, err)
		}
	}
}

func TestCLIProvidersCheck(t *testing.T) {
	dir := t.TempDir()
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		writeCLITestJSON(t, w, map[string]any{
			"object": "list",
			"data": []map[string]any{
				{"id": "model-a"},
			},
		})
	}))
	defer server.Close()

	manifestPath := filepath.Join(dir, "providers.yaml")
	if err := os.WriteFile(manifestPath, []byte(`
version: 1
name: test-providers
default_provider: openai
providers:
  - id: openai
    title: OpenAI
    kind: openai_compatible
    base_url: `+server.URL+`/v1
    api_key_env: OPENAI_API_KEY
    litellm_model: openai/test
    enabled: true
`), 0o600); err != nil {
		t.Fatalf("WriteFile manifest returned error: %v", err)
	}
	secretsPath := filepath.Join(dir, "provider.env")
	if err := os.WriteFile(secretsPath, []byte("OPENAI_API_KEY=sk-test-secret\n"), 0o600); err != nil {
		t.Fatalf("WriteFile secrets returned error: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"providers", "check", "--manifest", manifestPath, "--provider", "openai", "--secrets-file", secretsPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("providers check failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if gotAuth != "Bearer sk-test-secret" {
		t.Fatalf("unexpected Authorization header: %q", gotAuth)
	}
	if !strings.Contains(stdout.String(), "provider check ok: id=openai") || !strings.Contains(stdout.String(), "models=1") || !strings.Contains(stdout.String(), "secrets=OPENAI_API_KEY") {
		t.Fatalf("unexpected stdout: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), "sk-test-secret") || strings.Contains(stderr.String(), "sk-test-secret") {
		t.Fatalf("providers check exposed secret stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestCLISecretsValidateAndList(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "manifests", "providers", "default.yaml")
	dir := t.TempDir()
	secretsPath := filepath.Join(dir, "provider.env")
	if err := os.WriteFile(secretsPath, []byte("OPENAI_API_KEY=sk-test-secret\nUNRELATED_SECRET=ignore-me\n"), 0o600); err != nil {
		t.Fatalf("WriteFile secrets returned error: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"secrets", "validate", "--file", secretsPath, "--providers-manifest", manifestPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("secrets validate failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "names=1") || strings.Contains(stdout.String(), "sk-test-secret") {
		t.Fatalf("unexpected validate stdout: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run(context.Background(), []string{"secrets", "list", "--file", secretsPath, "--providers-manifest", manifestPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("secrets list failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "name=OPENAI_API_KEY") || strings.Contains(stdout.String(), "sk-test-secret") || strings.Contains(stdout.String(), "UNRELATED_SECRET") {
		t.Fatalf("unexpected list stdout: %s", stdout.String())
	}
}

func TestCLIWorkspaceAliasCreateAndList(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "stolas.yaml")
	cfg := config.Default()
	cfg.Node.ID = "cli-node"
	cfg.Paths.State = filepath.Join(dir, "stolas.db")
	cfg.Paths.AuditLog = filepath.Join(dir, "audit.jsonl")
	cfg.Paths.RuntimeDir = filepath.Join(dir, "run")
	if err := config.WriteFile(configPath, cfg, false); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"--config", configPath, "workspace", "create", "--name", "workspace-a", "--plan", "nano"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("workspace create failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "created workspace:") {
		t.Fatalf("workspace create did not use workspace wording: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run(context.Background(), []string{"--config", configPath, "workspace", "list"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("workspace list failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "workspace-a") {
		t.Fatalf("workspace list missing workspace: %s", stdout.String())
	}
}

func TestExecAuditDetailsDoesNotKeepRawArguments(t *testing.T) {
	details := execAuditDetails([]string{"sh", "-lc", "echo $OPENAI_API_KEY && sk-test-secret"})
	if details["argc"] != 3 || details["command"] != "sh" {
		t.Fatalf("unexpected details: %#v", details)
	}
	if _, ok := details["argv"]; ok {
		t.Fatalf("exec audit details kept raw argv: %#v", details)
	}
}

func TestCLIProxmoxCheckAndVPSCreate(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "pve.token")
	if err := os.WriteFile(tokenPath, []byte("root@pam!stola=secret"), 0o600); err != nil {
		t.Fatalf("WriteFile token returned error: %v", err)
	}
	var createCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "PVEAPIToken=root@pam!stola=secret" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		switch r.URL.Path {
		case "/api2/json/version":
			writeCLITestJSON(t, w, map[string]any{"data": map[string]any{"version": "9.0", "release": "9.0", "repoid": "test"}})
		case "/api2/json/nodes":
			writeCLITestJSON(t, w, map[string]any{"data": []map[string]any{{"node": "pve", "status": "online"}}})
		case "/api2/json/nodes/pve/storage":
			writeCLITestJSON(t, w, map[string]any{"data": []map[string]any{{"storage": "local-lvm", "type": "lvmthin", "content": "rootdir", "active": 1}}})
		case "/api2/json/nodes/pve/lxc":
			createCalled = true
			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm returned error: %v", err)
			}
			if got := r.PostForm.Get("vmid"); got != "101" {
				t.Fatalf("unexpected vmid: %q", got)
			}
			writeCLITestJSON(t, w, map[string]any{"data": "UPID:pve:101"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configPath := filepath.Join(dir, "stola.yaml")
	cfg := config.Default()
	cfg.Node.ID = "cli-node"
	cfg.Backend.Type = "proxmox"
	cfg.Backend.Proxmox.Endpoint = server.URL
	cfg.Backend.Proxmox.Node = "pve"
	cfg.Backend.Proxmox.TokenFile = tokenPath
	cfg.Paths.State = filepath.Join(dir, "stola.db")
	cfg.Paths.AuditLog = filepath.Join(dir, "audit.jsonl")
	cfg.Paths.RuntimeDir = filepath.Join(dir, "run")
	if err := config.WriteFile(configPath, cfg, false); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"--config", configPath, "proxmox", "check"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("proxmox check failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "storage: local-lvm") {
		t.Fatalf("unexpected proxmox check stdout: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run(context.Background(), []string{"--config", configPath, "vps", "create", "--name", "nano-a", "--vmid", "101", "--template", "local:vztmpl/debian-12-standard.tar.zst", "--storage", "local-lvm"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("vps create failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !createCalled {
		t.Fatal("expected LXC create endpoint to be called")
	}
	if !strings.Contains(stdout.String(), "task_id: UPID:pve:101") {
		t.Fatalf("unexpected vps create stdout: %s", stdout.String())
	}
}

func writeCLITestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("Encode returned error: %v", err)
	}
}

func TestCLIProvisionerMockCreateAndDaemonOnce(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "stola.yaml")
	cfg := config.Default()
	cfg.Node.ID = "cli-node"
	cfg.Paths.State = filepath.Join(dir, "stola.db")
	cfg.Paths.AuditLog = filepath.Join(dir, "audit.jsonl")
	cfg.Paths.RuntimeDir = filepath.Join(dir, "run")
	if err := config.WriteFile(configPath, cfg, false); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"--config", configPath, "provisioner", "mock-create", "--name", "nano-a", "--plan", "nano"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("mock-create failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "created mock vps") {
		t.Fatalf("unexpected mock-create stdout: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run(context.Background(), []string{"--config", configPath, "daemon", "provisioner", "--once"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("daemon --once failed code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "stola-provisioner heartbeat written") {
		t.Fatalf("unexpected daemon stdout: %s", stdout.String())
	}
}
