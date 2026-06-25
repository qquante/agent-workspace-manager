package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultManifestParsesAndRenders(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "manifests", "mcp", "default.yaml")
	manifest, raw, err := LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}
	if manifest.Name != "stola-default-mcp" {
		t.Fatalf("unexpected manifest name: %q", manifest.Name)
	}

	bundle, err := RenderBundle(manifest, raw)
	if err != nil {
		t.Fatalf("RenderBundle returned error: %v", err)
	}
	for _, path := range []string{WorkspaceManifestPath, AllowlistPath, OpenCodeConfigPath, HermesConfigPath, OpenClawConfigPath} {
		if len(bundle.Files[path]) == 0 {
			t.Fatalf("expected rendered file %s", path)
		}
	}

	var openCode map[string]any
	if err := json.Unmarshal(bundle.Files[OpenCodeConfigPath], &openCode); err != nil {
		t.Fatalf("opencode config is not JSON: %v", err)
	}
	mcpBlock := openCode["mcp"].(map[string]any)
	filesystem := mcpBlock["filesystem"].(map[string]any)
	if filesystem["type"] != "local" || filesystem["enabled"] != true {
		t.Fatalf("unexpected filesystem OpenCode config: %#v", filesystem)
	}
	context7 := mcpBlock["context7"].(map[string]any)
	if context7["type"] != "remote" || context7["enabled"] != false {
		t.Fatalf("unexpected context7 OpenCode config: %#v", context7)
	}
	if strings.Contains(string(bundle.Files[OpenCodeConfigPath]), "GITHUB_TOKEN") {
		t.Fatal("OpenCode config should not inject secret env placeholders by default")
	}
	openClawConfig := string(bundle.Files[OpenClawConfigPath])
	if strings.Contains(openClawConfig, `"transport": "stdio"`) {
		t.Fatalf("OpenClaw config should omit stdio servers unsupported by current OpenClaw config schema: %s", openClawConfig)
	}
	if !strings.Contains(openClawConfig, `"context7"`) || !strings.Contains(openClawConfig, `"streamable-http"`) {
		t.Fatalf("OpenClaw config should include remote optional servers: %s", openClawConfig)
	}
}

func TestManifestRejectsDuplicateServerIDs(t *testing.T) {
	raw := []byte(`
version: 1
name: duplicate-test
workspace:
  root: /workspace
servers:
  - id: filesystem
    default_enabled: true
    transport: stdio
    command: [npx, -y, fs]
optional_servers:
  - id: filesystem
    default_enabled: false
    transport: http
    url: https://example.com/mcp
`)
	_, err := Parse(raw)
	if err == nil || !strings.Contains(err.Error(), "duplicate server id") {
		t.Fatalf("expected duplicate server id error, got %v", err)
	}
}

func TestToolPolicyDecisionAndValidation(t *testing.T) {
	server := Server{
		ID:        "filesystem",
		Transport: "stdio",
		Command:   []string{"fake"},
		ToolPolicy: ToolPolicy{
			DefaultAction: ToolActionAllow,
			Rules: []ToolRule{
				{Tool: "write_file", Action: ToolActionRequireApproval, Reason: "writes workspace files"},
				{Tool: "danger", Action: ToolActionDeny},
			},
		},
	}
	writeDecision := server.ToolDecision("write_file")
	if writeDecision.Action != ToolActionRequireApproval || writeDecision.Reason != "writes workspace files" {
		t.Fatalf("unexpected write decision: %#v", writeDecision)
	}
	readDecision := server.ToolDecision("read_text_file")
	if readDecision.Action != ToolActionAllow || readDecision.Rule != "default" {
		t.Fatalf("unexpected default decision: %#v", readDecision)
	}
	denyDecision := server.ToolDecision("danger")
	if denyDecision.Action != ToolActionDeny {
		t.Fatalf("unexpected deny decision: %#v", denyDecision)
	}

	raw := []byte(`
version: 1
name: bad-policy-test
workspace:
  root: /workspace
servers:
  - id: filesystem
    default_enabled: true
    transport: stdio
    command: [fake]
    tool_policy:
      default_action: explode
`)
	_, err := Parse(raw)
	if err == nil || !strings.Contains(err.Error(), "default_action") {
		t.Fatalf("expected invalid tool policy error, got %v", err)
	}
}

func TestValidateArgumentPathPolicyAllowsWorkspacePaths(t *testing.T) {
	server := Server{ID: "filesystem"}
	for _, input := range []string{
		`{"path":"/workspace/src/main.go","content":"ok"}`,
		`{"path":"src/main.go","content":"ok"}`,
	} {
		if err := ValidateArgumentPathPolicy("/workspace", server, "write_file", json.RawMessage(input)); err != nil {
			t.Fatalf("expected %s to pass path policy: %v", input, err)
		}
	}
}

func TestValidateArgumentPathPolicyDeniesWorkspaceEscapeBeforeApproval(t *testing.T) {
	server := Server{ID: "filesystem"}
	for _, item := range []struct {
		name  string
		tool  string
		input string
		rule  string
	}{
		{name: "absolute outside workspace", tool: "write_file", input: `{"path":"/etc/passwd","content":"x"}`, rule: "outside_workspace"},
		{name: "relative traversal", tool: "write_file", input: `{"path":"../secrets.txt","content":"x"}`, rule: "outside_workspace"},
		{name: "windows host path", tool: "write_file", input: `{"path":"C:\\Users\\PC\\.ssh\\id_rsa","content":"x"}`, rule: "host_path_syntax"},
		{name: "move destination outside workspace", tool: "move_file", input: `{"source":"/workspace/a","destination":"/tmp/a"}`, rule: "outside_workspace"},
	} {
		t.Run(item.name, func(t *testing.T) {
			err := ValidateArgumentPathPolicy("/workspace", server, item.tool, json.RawMessage(item.input))
			if err == nil {
				t.Fatal("expected path policy denial")
			}
			policyErr, ok := err.(*ArgumentPolicyError)
			if !ok {
				t.Fatalf("expected ArgumentPolicyError, got %T", err)
			}
			if policyErr.Rule != item.rule {
				t.Fatalf("expected rule %s, got %#v", item.rule, policyErr)
			}
		})
	}
}

func TestValidateArgumentPathPolicyDeniesProtectedWorkspaceData(t *testing.T) {
	server := Server{ID: "filesystem"}
	for _, item := range []struct {
		name  string
		tool  string
		input string
		rule  string
	}{
		{name: "stola metadata", tool: "write_file", input: `{"path":"/workspace/.stola/mcp/default.yaml","content":"x"}`, rule: "protected_stola_metadata"},
		{name: "opencode config", tool: "write_file", input: `{"path":"/workspace/opencode.json","content":"x"}`, rule: "protected_agent_config"},
		{name: "private key suffix", tool: "write_file", input: `{"path":"/workspace/deploy.pem","content":"x"}`, rule: "protected_key_file"},
		{name: "env file", tool: "read_text_file", input: `{"path":"/workspace/.env.local"}`, rule: "protected_env_file"},
		{name: "multiple file read", tool: "read_multiple_files", input: `{"paths":["/workspace/README.md","/workspace/.ssh/id_rsa"]}`, rule: "protected_secret_directory"},
	} {
		t.Run(item.name, func(t *testing.T) {
			err := ValidateArgumentPathPolicy("/workspace", server, item.tool, json.RawMessage(item.input))
			if err == nil {
				t.Fatal("expected protected path denial")
			}
			policyErr, ok := err.(*ArgumentPolicyError)
			if !ok {
				t.Fatalf("expected ArgumentPolicyError, got %T", err)
			}
			if policyErr.Rule != item.rule {
				t.Fatalf("expected rule %s, got %#v", item.rule, policyErr)
			}
			if len(policyErr.AuditDetails()["argument_path_fields"].([]string)) == 0 {
				t.Fatalf("expected sanitized path fields in audit details: %#v", policyErr.AuditDetails())
			}
		})
	}
}

func TestValidateArgumentPathPolicyDeniesUnknownFilesystemTools(t *testing.T) {
	err := ValidateArgumentPathPolicy("/workspace", Server{ID: "filesystem"}, "future_mutator", json.RawMessage(`{"path":"/workspace/a"}`))
	if err == nil {
		t.Fatal("expected unknown filesystem tool to be denied")
	}
	policyErr, ok := err.(*ArgumentPolicyError)
	if !ok || policyErr.Rule != "unsupported_filesystem_tool" {
		t.Fatalf("unexpected policy error: %#v", err)
	}
}

func TestValidateArgumentPathPolicyUsesPlanOverride(t *testing.T) {
	server := Server{
		ID: "filesystem",
		ArgumentPolicy: ArgumentPolicy{
			Overrides: []ArgumentPolicyOverride{
				{
					Scope: ArgumentPolicyScope{Plans: []string{"trusted"}},
					Policy: ArgumentPolicy{
						AllowProtectedBasenames: []string{"opencode.json"},
					},
				},
			},
		},
	}
	input := json.RawMessage(`{"path":"/workspace/opencode.json","content":"ok"}`)
	if err := ValidateArgumentPathPolicy("/workspace", server, "write_file", input, ArgumentPolicyContext{Plan: "nano"}); err == nil {
		t.Fatal("expected nano plan to keep opencode.json protected")
	}
	if err := ValidateArgumentPathPolicy("/workspace", server, "write_file", input, ArgumentPolicyContext{Plan: "trusted"}); err != nil {
		t.Fatalf("expected trusted plan override to allow opencode.json: %v", err)
	}
}

func TestValidateArgumentPathPolicyUsesVPSAndUserScopedOverride(t *testing.T) {
	server := Server{
		ID: "filesystem",
		ArgumentPolicy: ArgumentPolicy{
			Overrides: []ArgumentPolicyOverride{
				{
					Scope: ArgumentPolicyScope{VPSIDs: []string{"vps-a"}, UserIDs: []string{"user-a"}},
					Policy: ArgumentPolicy{
						Enabled: boolPtr(false),
					},
				},
			},
		},
	}
	input := json.RawMessage(`{"path":"/etc/passwd","content":"x"}`)
	if err := ValidateArgumentPathPolicy("/workspace", server, "write_file", input, ArgumentPolicyContext{VPSID: "vps-a", UserID: "user-b"}); err == nil {
		t.Fatal("expected mismatched user to keep policy enabled")
	}
	if err := ValidateArgumentPathPolicy("/workspace", server, "write_file", input, ArgumentPolicyContext{VPSID: "vps-a", UserID: "user-a"}); err != nil {
		t.Fatalf("expected matching vps/user override to disable policy: %v", err)
	}
}

func TestValidateArgumentPathPolicyAllowsManifestDefinedToolSchema(t *testing.T) {
	server := Server{
		ID: "filesystem",
		ArgumentPolicy: ArgumentPolicy{
			Tools: map[string][]ArgumentPathField{
				"future_mutator": {{Field: "target", Type: "string", Required: boolPtr(true)}},
			},
		},
	}
	if err := ValidateArgumentPathPolicy("/workspace", server, "future_mutator", json.RawMessage(`{"target":"/workspace/a.txt"}`)); err != nil {
		t.Fatalf("expected manifest-defined tool schema to pass: %v", err)
	}
	err := ValidateArgumentPathPolicy("/workspace", server, "future_mutator", json.RawMessage(`{"target":"/workspace/.stola/a.txt"}`))
	if err == nil {
		t.Fatal("expected manifest-defined tool to still enforce protected paths")
	}
	policyErr, ok := err.(*ArgumentPolicyError)
	if !ok || policyErr.Rule != "protected_stola_metadata" {
		t.Fatalf("unexpected policy error: %#v", err)
	}
}

func TestEvaluateExecutionPolicyRuleRequiresApproval(t *testing.T) {
	manifest := Manifest{
		ExecutionPolicy: ExecutionPolicy{
			Rules: []ExecutionRule{
				{
					ID:     "filesystem-write",
					Action: ToolActionRequireApproval,
					Match:  ExecutionRuleMatch{Servers: []string{"filesystem"}, Tools: []string{"write_file"}},
				},
			},
		},
	}
	server := Server{ID: "filesystem", Risk: "medium", Command: []string{"npx"}, Permissions: map[string]any{"network": "none"}}
	decision, err := EvaluateExecutionPolicy(manifest, server, "write_file", ExecutionPolicyContext{VPSID: "vps-a", Plan: "nano"})
	if err != nil {
		t.Fatalf("EvaluateExecutionPolicy returned error: %v", err)
	}
	if decision.Action != ToolActionRequireApproval || decision.Rule != "execution_policy:filesystem-write" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if decision.RateLimit == nil || decision.RateLimit.MaxCalls != 120 {
		t.Fatalf("expected default rate limit in decision: %#v", decision)
	}
}

func TestEvaluateExecutionPolicyDeniesNetworkAndCommand(t *testing.T) {
	manifest := Manifest{}
	networkServer := Server{ID: "scanner", Risk: "medium", Command: []string{"npx"}, Permissions: map[string]any{"network": "raw"}}
	decision, err := EvaluateExecutionPolicy(manifest, networkServer, "scan", ExecutionPolicyContext{})
	if err != nil {
		t.Fatalf("EvaluateExecutionPolicy returned error: %v", err)
	}
	if decision.Action != ToolActionDeny || decision.Rule != "execution_policy:network_deny" {
		t.Fatalf("unexpected network decision: %#v", decision)
	}

	commandServer := Server{ID: "shell", Risk: "medium", Command: []string{"bash"}, Permissions: map[string]any{"network": "none"}}
	decision, err = EvaluateExecutionPolicy(manifest, commandServer, "run", ExecutionPolicyContext{})
	if err != nil {
		t.Fatalf("EvaluateExecutionPolicy returned error: %v", err)
	}
	if decision.Action != ToolActionDeny || decision.Rule != "execution_policy:command_deny" {
		t.Fatalf("unexpected command decision: %#v", decision)
	}
}

func TestEvaluateExecutionPolicyRuleRateLimitOverride(t *testing.T) {
	manifest := Manifest{
		ExecutionPolicy: ExecutionPolicy{
			RateLimit: RateLimitPolicy{MaxCalls: 100, Window: "1m"},
			Rules: []ExecutionRule{
				{
					ID:        "fetch-tight-limit",
					Action:    ToolActionAllow,
					Match:     ExecutionRuleMatch{Servers: []string{"fetch"}},
					RateLimit: RateLimitPolicy{MaxCalls: 2, Window: "10s"},
				},
			},
		},
	}
	server := Server{ID: "fetch", Risk: "medium", Command: []string{"uvx"}, Permissions: map[string]any{"network": "https_only"}}
	decision, err := EvaluateExecutionPolicy(manifest, server, "fetch", ExecutionPolicyContext{VPSID: "vps-a", Plan: "nano", UserID: "user-a"})
	if err != nil {
		t.Fatalf("EvaluateExecutionPolicy returned error: %v", err)
	}
	if decision.Action != ToolActionAllow {
		t.Fatalf("unexpected action: %#v", decision)
	}
	if decision.RateLimit == nil || decision.RateLimit.MaxCalls != 2 || decision.RateLimit.Window.String() != "10s" {
		t.Fatalf("unexpected rate limit: %#v", decision.RateLimit)
	}
	if !strings.Contains(decision.RateLimit.Key, "vps=vps-a") || !strings.Contains(decision.RateLimit.Key, "server=fetch") {
		t.Fatalf("rate limit key missing scope: %s", decision.RateLimit.Key)
	}
}

func TestPolicyOverlayAppliesExecutionAndServerPolicy(t *testing.T) {
	manifest := Manifest{
		Version:   1,
		Name:      "test",
		Workspace: Workspace{Root: "/workspace"},
		Servers: []Server{
			{
				ID:             "filesystem",
				DefaultEnabled: true,
				Risk:           "medium",
				Transport:      "stdio",
				Command:        []string{"uvx", "mcp-server-filesystem", "/workspace"},
				Permissions:    map[string]any{"network": "none"},
				ToolPolicy:     ToolPolicy{DefaultAction: ToolActionAllow},
			},
		},
	}
	overlay, err := ParsePolicyOverlay([]byte(`
version: 1
id: lab-policy
execution_policy:
  rate_limit:
    max_calls: 2
    window: 10s
  rules:
    - id: user-write-approval
      action: require_approval
      match:
        user_ids: ["user-a"]
        tools: ["write_file"]
servers:
  - id: filesystem
    tool_policy:
      rules:
        - tool: delete_file
          action: deny
          reason: no deletes in lab
`))
	if err != nil {
		t.Fatalf("ParsePolicyOverlay returned error: %v", err)
	}
	effective, err := ApplyPolicyOverlay(manifest, overlay)
	if err != nil {
		t.Fatalf("ApplyPolicyOverlay returned error: %v", err)
	}
	decision, err := EvaluateExecutionPolicy(effective, effective.Servers[0], "write_file", ExecutionPolicyContext{UserID: "user-a"})
	if err != nil {
		t.Fatalf("EvaluateExecutionPolicy returned error: %v", err)
	}
	if decision.Action != ToolActionRequireApproval || decision.RateLimit == nil || decision.RateLimit.MaxCalls != 2 {
		t.Fatalf("unexpected write_file decision: %+v", decision)
	}
	decision, err = EvaluateExecutionPolicy(effective, effective.Servers[0], "delete_file", ExecutionPolicyContext{UserID: "user-a"})
	if err != nil {
		t.Fatalf("EvaluateExecutionPolicy returned error: %v", err)
	}
	if decision.Action != ToolActionDeny {
		t.Fatalf("expected delete_file deny, got %+v", decision)
	}
}

func TestWriteBundleToDir(t *testing.T) {
	manifest, raw, err := LoadFile(filepath.Join("..", "..", "manifests", "mcp", "default.yaml"))
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}
	bundle, err := RenderBundle(manifest, raw)
	if err != nil {
		t.Fatalf("RenderBundle returned error: %v", err)
	}
	dir := t.TempDir()
	if err := WriteBundleToDir(bundle, dir); err != nil {
		t.Fatalf("WriteBundleToDir returned error: %v", err)
	}
	for _, rel := range []string{WorkspaceManifestPath, AllowlistPath, OpenCodeConfigPath, HermesConfigPath, OpenClawConfigPath} {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if _, err := os.Stat(full); err != nil {
			t.Fatalf("expected rendered file %s: %v", rel, err)
		}
	}
}
