package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseValidConfig(t *testing.T) {
	cfg, err := ParseString(`
node:
  id: test-node
backend:
  type: mock
paths:
  state: /tmp/stola.db
  audit_log: /tmp/audit.jsonl
  runtime_dir: /tmp/stola
api:
  listen: 127.0.0.1:8750
  users_file: /etc/stola/users.yaml
  signing_key_file: /etc/stola/secrets/control-plane.key
  mcp_manifest: /etc/stola/manifests/mcp/default.yaml
  policy_file: /etc/stola/policy.yaml
`)
	if err != nil {
		t.Fatalf("ParseString returned error: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if cfg.Node.ID != "test-node" {
		t.Fatalf("node id mismatch: %s", cfg.Node.ID)
	}
	if cfg.Backend.Type != "mock" {
		t.Fatalf("backend mismatch: %s", cfg.Backend.Type)
	}
	if cfg.API.UsersFile != "/etc/stola/users.yaml" || cfg.API.PolicyFile != "/etc/stola/policy.yaml" {
		t.Fatalf("api files mismatch: %+v", cfg.API)
	}
}

func TestParseAcceptsUTF8BOM(t *testing.T) {
	cfg, err := ParseString("\ufeff" + `
node:
  id: bom-node
backend:
  type: mock
paths:
  state: /tmp/stola.db
  audit_log: /tmp/audit.jsonl
  runtime_dir: /tmp/stola
api:
  listen: 127.0.0.1:8750
`)
	if err != nil {
		t.Fatalf("ParseString returned error: %v", err)
	}
	if cfg.Node.ID != "bom-node" {
		t.Fatalf("node id mismatch: %s", cfg.Node.ID)
	}
}

func TestValidateRejectsMissingBackendAndPaths(t *testing.T) {
	cfg := Config{
		Node: NodeConfig{ID: "test-node"},
		API:  APIConfig{Listen: "127.0.0.1:8750"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, want := range []string{"backend.type is required", "paths.audit_log is required", "paths.state is required"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %q", want, err.Error())
		}
	}
}

func TestWriteAndLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stola.yaml")
	cfg := Default()
	cfg.Node.ID = "disk-node"
	cfg.Paths.State = filepath.Join(dir, "stola.db")
	cfg.Paths.AuditLog = filepath.Join(dir, "audit.jsonl")
	cfg.Paths.RuntimeDir = filepath.Join(dir, "run")

	if err := WriteFile(path, cfg, false); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if err := loaded.Validate(); err != nil {
		t.Fatalf("loaded config invalid: %v", err)
	}
	if loaded.Node.ID != "disk-node" {
		t.Fatalf("loaded node mismatch: %s", loaded.Node.ID)
	}
	if loaded.API.SigningKeyFile == "" || loaded.API.UsersFile == "" {
		t.Fatalf("loaded api identity paths missing: %+v", loaded.API)
	}
}

func TestParseProxmoxConfigUsesTokenFile(t *testing.T) {
	cfg, err := ParseString(`
node:
  id: pve-node
backend:
  type: proxmox
  proxmox:
    endpoint: https://pve.example:8006
    node: pve
    token_file: /etc/stola/secrets/proxmox.token
    insecure_skip_tls_verify: true
paths:
  state: /var/lib/stola/stola.db
  audit_log: /var/log/stola/audit.jsonl
  runtime_dir: /run/stola
api:
  listen: 127.0.0.1:8750
`)
	if err != nil {
		t.Fatalf("ParseString returned error: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if cfg.Backend.Proxmox.TokenFile != "/etc/stola/secrets/proxmox.token" {
		t.Fatalf("token_file mismatch: %s", cfg.Backend.Proxmox.TokenFile)
	}
	if !cfg.Backend.Proxmox.InsecureSkipTLSVerify {
		t.Fatal("expected insecure_skip_tls_verify to be parsed")
	}
}

func TestParseDockerConfig(t *testing.T) {
	cfg, err := ParseString(`
node:
  id: docker-node
backend:
  type: docker
  docker:
    cli_path: docker
    image: ghcr.io/stola/agent-base:dev
    network_prefix: stola-net
    volume_prefix: stola-workspace
paths:
  state: /var/lib/stola/stola.db
  audit_log: /var/log/stola/audit.jsonl
  runtime_dir: /run/stola
api:
  listen: 127.0.0.1:8750
`)
	if err != nil {
		t.Fatalf("ParseString returned error: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if cfg.Backend.Type != "docker" {
		t.Fatalf("backend mismatch: %s", cfg.Backend.Type)
	}
	if cfg.Backend.Docker.Image != "ghcr.io/stola/agent-base:dev" {
		t.Fatalf("docker image mismatch: %s", cfg.Backend.Docker.Image)
	}
}

func TestParseRejectsDirectSecretKey(t *testing.T) {
	_, err := ParseString(`
node:
  id: test-node
backend:
  type: proxmox
  proxmox:
    endpoint: https://pve.local:8006
    node: pve
    token: should-not-be-here
paths:
  state: /tmp/stola.db
  audit_log: /tmp/audit.jsonl
  runtime_dir: /tmp/stola
api:
  listen: 127.0.0.1:8750
`)
	if err == nil {
		t.Fatal("expected unsupported token key error")
	}
	if !strings.Contains(err.Error(), "backend.proxmox.token") {
		t.Fatalf("unexpected error: %v", err)
	}
}
