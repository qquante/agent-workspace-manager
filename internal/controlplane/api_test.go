package controlplane

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
	"time"

	"stola-os/internal/config"
	"stola-os/internal/state"
)

func TestAuthenticatorRBAC(t *testing.T) {
	dir := t.TempDir()
	token := "dev-token"
	usersPath := filepath.Join(dir, "users.yaml")
	if err := os.WriteFile(usersPath, []byte(`
version: 1
roles:
  admin:
    permissions: ["mcp.approve", "policy.read", "policy.reload"]
users:
  - id: user-a
    roles: ["admin"]
    token_sha256: `+TokenSHA256(token)+`
`), 0o600); err != nil {
		t.Fatalf("WriteFile users returned error: %v", err)
	}
	auth, err := LoadAuthenticator(usersPath)
	if err != nil {
		t.Fatalf("LoadAuthenticator returned error: %v", err)
	}
	principal, err := auth.AuthenticateBearer("Bearer " + token)
	if err != nil {
		t.Fatalf("AuthenticateBearer returned error: %v", err)
	}
	if principal.ID != "user-a" || !principal.HasPermission(PermissionPolicyReload) {
		t.Fatalf("unexpected principal: %+v", principal)
	}
	if _, err := auth.AuthenticateBearer("Bearer wrong"); err == nil {
		t.Fatal("expected invalid bearer token error")
	}
}

func TestApprovalTokenSignVerify(t *testing.T) {
	key := bytes.Repeat([]byte{1}, 32)
	payload := ApprovalPayload{
		ApprovalID:     "mcpappr-a",
		VPSID:          "vps-a",
		Server:         "filesystem",
		Tool:           "write_file",
		ArgumentDigest: "digest-a",
		UserID:         "user-a",
		PolicyVersion:  "policy-a",
		ExpiresAt:      time.Now().UTC().Add(time.Minute),
	}
	token, err := SignApproval(key, payload)
	if err != nil {
		t.Fatalf("SignApproval returned error: %v", err)
	}
	verified, err := VerifyApprovalToken(key, token, time.Now().UTC())
	if err != nil {
		t.Fatalf("VerifyApprovalToken returned error: %v", err)
	}
	if verified.ApprovalID != payload.ApprovalID || verified.UserID != payload.UserID {
		t.Fatalf("unexpected payload: %+v", verified)
	}
	if _, err := VerifyApprovalToken(bytes.Repeat([]byte{2}, 32), token, time.Now().UTC()); err == nil {
		t.Fatal("expected bad signature error")
	}
}

func TestAPIApprovalCreateReturnsSignedToken(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "mcp.yaml")
	if err := os.WriteFile(manifestPath, []byte(`
version: 1
name: test-mcp
workspace:
  root: /workspace
policy:
  require_explicit_enable: true
  secrets_from_env_only: true
  audit_all_tool_calls: true
servers:
  - id: filesystem
    default_enabled: true
    risk: medium
    transport: stdio
    command: ["uvx", "mcp-server-filesystem", "/workspace"]
`), 0o600); err != nil {
		t.Fatalf("WriteFile manifest returned error: %v", err)
	}
	usersPath := filepath.Join(dir, "users.yaml")
	token := "control-plane-token"
	if err := os.WriteFile(usersPath, []byte(`
version: 1
roles:
  operator:
    permissions: ["mcp.approve", "policy.read", "policy.reload"]
users:
  - id: user-a
    roles: ["operator"]
    token_sha256: `+TokenSHA256(token)+`
`), 0o600); err != nil {
		t.Fatalf("WriteFile users returned error: %v", err)
	}
	store, err := state.Open(filepath.Join(dir, "stola.db"))
	if err != nil {
		t.Fatalf("state.Open returned error: %v", err)
	}
	defer store.Close()
	if err := store.CreateVPS(context.Background(), state.VPS{ID: "vps-a", Name: "vps-a", Plan: "nano", Status: "running", Backend: "docker"}); err != nil {
		t.Fatalf("CreateVPS returned error: %v", err)
	}
	cfg := config.Default()
	cfg.API.UsersFile = usersPath
	cfg.API.SigningKeyFile = filepath.Join(dir, "control-plane.key")
	cfg.API.MCPManifest = manifestPath
	cfg.Paths.State = filepath.Join(dir, "stola.db")
	cfg.Paths.AuditLog = filepath.Join(dir, "audit.jsonl")
	server, err := NewAPIServer(cfg, store, nil)
	if err != nil {
		t.Fatalf("NewAPIServer returned error: %v", err)
	}
	body := strings.NewReader(`{"vps_id":"vps-a","server":"filesystem","tool":"write_file","argument_digest":"digest-a","argument_keys":["path"],"expires_in":"5m"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/mcp/approvals", body)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp ApprovalCreateResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response returned error: %v", err)
	}
	if resp.ApprovedBy != "user-a" || resp.PolicyVersion == "" || resp.ApprovalToken == "" {
		t.Fatalf("unexpected approval response: %+v", resp)
	}
	key, err := LoadSigningKey(cfg.API.SigningKeyFile)
	if err != nil {
		t.Fatalf("LoadSigningKey returned error: %v", err)
	}
	payload, err := VerifyApprovalToken(key, resp.ApprovalToken, time.Now().UTC())
	if err != nil {
		t.Fatalf("VerifyApprovalToken returned error: %v", err)
	}
	if payload.ApprovalID != resp.ApprovalID || payload.UserID != "user-a" || payload.PolicyVersion != resp.PolicyVersion {
		t.Fatalf("unexpected token payload: %+v", payload)
	}
	stored, err := store.GetMCPApproval(context.Background(), resp.ApprovalID)
	if err != nil {
		t.Fatalf("GetMCPApproval returned error: %v", err)
	}
	if stored.ApprovedBy != "user-a" || stored.PolicyVersion != resp.PolicyVersion {
		t.Fatalf("approval metadata not persisted: %+v", stored)
	}
}

func TestAPIWorkspaceCreateListGet(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "mcp.yaml")
	if err := os.WriteFile(manifestPath, []byte(`
version: 1
name: test-mcp
workspace:
  root: /workspace
policy:
  require_explicit_enable: true
  secrets_from_env_only: true
  audit_all_tool_calls: true
servers:
  - id: filesystem
    default_enabled: true
    risk: medium
    transport: stdio
    command: ["uvx", "mcp-server-filesystem", "/workspace"]
`), 0o600); err != nil {
		t.Fatalf("WriteFile manifest returned error: %v", err)
	}
	usersPath := filepath.Join(dir, "users.yaml")
	token := "workspace-token"
	if err := os.WriteFile(usersPath, []byte(`
version: 1
roles:
  operator:
    permissions: ["workspace.create", "workspace.read", "policy.read", "usage.write", "usage.read", "llm.key.create", "llm.key.read", "llm.key.rotate", "llm.key.revoke"]
users:
  - id: user-a
    roles: ["operator"]
    token_sha256: `+TokenSHA256(token)+`
`), 0o600); err != nil {
		t.Fatalf("WriteFile users returned error: %v", err)
	}
	store, err := state.Open(filepath.Join(dir, "stola.db"))
	if err != nil {
		t.Fatalf("state.Open returned error: %v", err)
	}
	defer store.Close()
	cfg := config.Default()
	cfg.Backend.Type = "mock"
	cfg.API.UsersFile = usersPath
	cfg.API.SigningKeyFile = filepath.Join(dir, "control-plane.key")
	cfg.API.MCPManifest = manifestPath
	cfg.Paths.State = filepath.Join(dir, "stola.db")
	cfg.Paths.AuditLog = filepath.Join(dir, "audit.jsonl")
	server, err := NewAPIServer(cfg, store, nil)
	if err != nil {
		t.Fatalf("NewAPIServer returned error: %v", err)
	}

	body := strings.NewReader(`{"name":"api-workspace","plan":"nano","memory_mb":512,"cores":1}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/workspaces", body)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var createResp WorkspaceCreateResponse
	if err := json.NewDecoder(rec.Body).Decode(&createResp); err != nil {
		t.Fatalf("Decode create response returned error: %v", err)
	}
	if createResp.Workspace.Name != "api-workspace" || createResp.Workspace.Backend != "mock" {
		t.Fatalf("unexpected create response: %+v", createResp)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/workspaces", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp WorkspaceListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("Decode list response returned error: %v", err)
	}
	if len(listResp.Workspaces) != 1 || listResp.Workspaces[0].ID != createResp.Workspace.ID {
		t.Fatalf("unexpected list response: %+v", listResp)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/workspaces/"+createResp.Workspace.ID, nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected get 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	var getResp WorkspaceView
	if err := json.NewDecoder(getRec.Body).Decode(&getResp); err != nil {
		t.Fatalf("Decode get response returned error: %v", err)
	}
	if getResp.ID != createResp.Workspace.ID || getResp.Name != "api-workspace" {
		t.Fatalf("unexpected get response: %+v", getResp)
	}

	usageBody := strings.NewReader(`{"workspace_id":"` + createResp.Workspace.ID + `","provider_id":"openrouter","model":"openrouter/openai/gpt-4o-mini","input_tokens":10,"output_tokens":5,"cost_usd":0.001,"latency_ms":321}`)
	usageReq := httptest.NewRequest(http.MethodPost, "/v1/usage", usageBody)
	usageReq.Header.Set("Authorization", "Bearer "+token)
	usageRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(usageRec, usageReq)
	if usageRec.Code != http.StatusCreated {
		t.Fatalf("expected usage record 201, got %d body=%s", usageRec.Code, usageRec.Body.String())
	}
	var usageResp UsageRecordResponse
	if err := json.NewDecoder(usageRec.Body).Decode(&usageResp); err != nil {
		t.Fatalf("Decode usage response returned error: %v", err)
	}
	if usageResp.Usage.WorkspaceID != createResp.Workspace.ID || usageResp.Usage.TotalTokens != 15 || usageResp.Usage.UserID != "user-a" {
		t.Fatalf("unexpected usage response: %+v", usageResp)
	}

	usageListReq := httptest.NewRequest(http.MethodGet, "/v1/usage?workspace_id="+createResp.Workspace.ID, nil)
	usageListReq.Header.Set("Authorization", "Bearer "+token)
	usageListRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(usageListRec, usageListReq)
	if usageListRec.Code != http.StatusOK {
		t.Fatalf("expected usage list 200, got %d body=%s", usageListRec.Code, usageListRec.Body.String())
	}
	var usageListResp UsageListResponse
	if err := json.NewDecoder(usageListRec.Body).Decode(&usageListResp); err != nil {
		t.Fatalf("Decode usage list response returned error: %v", err)
	}
	if len(usageListResp.Usage) != 1 || usageListResp.Usage[0].ProviderID != "openrouter" {
		t.Fatalf("unexpected usage list response: %+v", usageListResp)
	}

	summaryReq := httptest.NewRequest(http.MethodGet, "/v1/usage/summary?workspace_id="+createResp.Workspace.ID+"&provider_id=openrouter", nil)
	summaryReq.Header.Set("Authorization", "Bearer "+token)
	summaryRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(summaryRec, summaryReq)
	if summaryRec.Code != http.StatusOK {
		t.Fatalf("expected usage summary 200, got %d body=%s", summaryRec.Code, summaryRec.Body.String())
	}
	var summaryResp UsageSummaryResponse
	if err := json.NewDecoder(summaryRec.Body).Decode(&summaryResp); err != nil {
		t.Fatalf("Decode usage summary response returned error: %v", err)
	}
	if summaryResp.Summary.Events != 1 || summaryResp.Summary.TotalTokens != 15 || summaryResp.Summary.CostUSD < 0.0009 {
		t.Fatalf("unexpected usage summary response: %+v", summaryResp)
	}

	keyBody := strings.NewReader(`{"workspace_id":"` + createResp.Workspace.ID + `","provider_id":"openrouter","models":["openai/gpt-4o-mini"],"budget_usd":0.01,"rpm":2,"tpm":100,"max_concurrency":1}`)
	keyReq := httptest.NewRequest(http.MethodPost, "/v1/llm/keys", keyBody)
	keyReq.Header.Set("Authorization", "Bearer "+token)
	keyRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(keyRec, keyReq)
	if keyRec.Code != http.StatusCreated {
		t.Fatalf("expected llm key create 201, got %d body=%s", keyRec.Code, keyRec.Body.String())
	}
	var keyResp LLMKeyCreateResponse
	if err := json.NewDecoder(keyRec.Body).Decode(&keyResp); err != nil {
		t.Fatalf("Decode key create response returned error: %v", err)
	}
	if keyResp.Token == "" || !strings.HasPrefix(keyResp.Token, "sk-stolas-") || keyResp.Key.ProviderID != "openrouter" || keyResp.Key.SpendUSD != 0 || keyResp.Key.TPM != 100 || keyResp.Key.MaxConcurrency != 1 {
		t.Fatalf("unexpected key create response: %+v", keyResp)
	}
	if strings.Contains(keyResp.Key.ID, keyResp.Token) {
		t.Fatalf("key view exposed raw token")
	}

	keyListReq := httptest.NewRequest(http.MethodGet, "/v1/llm/keys?workspace_id="+createResp.Workspace.ID, nil)
	keyListReq.Header.Set("Authorization", "Bearer "+token)
	keyListRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(keyListRec, keyListReq)
	if keyListRec.Code != http.StatusOK {
		t.Fatalf("expected llm key list 200, got %d body=%s", keyListRec.Code, keyListRec.Body.String())
	}
	if strings.Contains(keyListRec.Body.String(), keyResp.Token) {
		t.Fatalf("llm key list exposed raw token: %s", keyListRec.Body.String())
	}
	var keyListResp LLMKeyListResponse
	if err := json.NewDecoder(keyListRec.Body).Decode(&keyListResp); err != nil {
		t.Fatalf("Decode key list response returned error: %v", err)
	}
	if len(keyListResp.Keys) != 1 || keyListResp.Keys[0].ID != keyResp.Key.ID {
		t.Fatalf("unexpected key list response: %+v", keyListResp)
	}

	rotateReq := httptest.NewRequest(http.MethodPost, "/v1/llm/keys/"+keyResp.Key.ID+"/rotate", nil)
	rotateReq.Header.Set("Authorization", "Bearer "+token)
	rotateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rotateRec, rotateReq)
	if rotateRec.Code != http.StatusOK {
		t.Fatalf("expected llm key rotate 200, got %d body=%s", rotateRec.Code, rotateRec.Body.String())
	}
	var rotateResp LLMKeyRotateResponse
	if err := json.NewDecoder(rotateRec.Body).Decode(&rotateResp); err != nil {
		t.Fatalf("Decode key rotate response returned error: %v", err)
	}
	if rotateResp.Token == "" || rotateResp.Token == keyResp.Token || rotateResp.Key.ID != keyResp.Key.ID || rotateResp.Key.Status != "active" {
		t.Fatalf("unexpected key rotate response: %+v", rotateResp)
	}

	revokeReq := httptest.NewRequest(http.MethodPost, "/v1/llm/keys/"+keyResp.Key.ID+"/revoke", nil)
	revokeReq.Header.Set("Authorization", "Bearer "+token)
	revokeRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(revokeRec, revokeReq)
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("expected llm key revoke 200, got %d body=%s", revokeRec.Code, revokeRec.Body.String())
	}
	var revokeResp LLMKeyView
	if err := json.NewDecoder(revokeRec.Body).Decode(&revokeResp); err != nil {
		t.Fatalf("Decode key revoke response returned error: %v", err)
	}
	if revokeResp.ID != keyResp.Key.ID || revokeResp.Status != "revoked" {
		t.Fatalf("unexpected key revoke response: %+v", revokeResp)
	}
}

func TestAPIApprovalApproveSignsExistingIntent(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "mcp.yaml")
	if err := os.WriteFile(manifestPath, []byte(`
version: 1
name: test-mcp
workspace:
  root: /workspace
policy:
  require_explicit_enable: true
  secrets_from_env_only: true
  audit_all_tool_calls: true
servers:
  - id: filesystem
    default_enabled: true
    risk: medium
    transport: stdio
    command: ["uvx", "mcp-server-filesystem", "/workspace"]
`), 0o600); err != nil {
		t.Fatalf("WriteFile manifest returned error: %v", err)
	}
	usersPath := filepath.Join(dir, "users.yaml")
	token := "control-plane-token"
	if err := os.WriteFile(usersPath, []byte(`
version: 1
roles:
  operator:
    permissions: ["mcp.approve", "policy.read", "policy.reload"]
users:
  - id: user-a
    roles: ["operator"]
    token_sha256: `+TokenSHA256(token)+`
`), 0o600); err != nil {
		t.Fatalf("WriteFile users returned error: %v", err)
	}
	store, err := state.Open(filepath.Join(dir, "stola.db"))
	if err != nil {
		t.Fatalf("state.Open returned error: %v", err)
	}
	defer store.Close()
	now := time.Now().UTC()
	created, err := store.CreateMCPApproval(context.Background(), state.MCPApproval{
		VPSID:          "vps-a",
		Server:         "filesystem",
		Tool:           "write_file",
		ArgumentDigest: "digest-a",
		ArgumentKeys:   []string{"path"},
		CreatedAt:      now,
		ExpiresAt:      now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateMCPApproval returned error: %v", err)
	}
	cfg := config.Default()
	cfg.API.UsersFile = usersPath
	cfg.API.SigningKeyFile = filepath.Join(dir, "control-plane.key")
	cfg.API.MCPManifest = manifestPath
	cfg.Paths.State = filepath.Join(dir, "stola.db")
	cfg.Paths.AuditLog = filepath.Join(dir, "audit.jsonl")
	server, err := NewAPIServer(cfg, store, nil)
	if err != nil {
		t.Fatalf("NewAPIServer returned error: %v", err)
	}
	listReq := httptest.NewRequest(http.MethodGet, "/v1/mcp/approvals?status=pending", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp ApprovalListResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("Decode list response returned error: %v", err)
	}
	if len(listResp.Approvals) != 1 || listResp.Approvals[0].ID != created.ID {
		t.Fatalf("unexpected list response: %+v", listResp)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/mcp/approvals/"+created.ID+"/approve", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp ApprovalCreateResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("Decode response returned error: %v", err)
	}
	if resp.ApprovalID != created.ID || resp.ApprovedBy != "user-a" || resp.ApprovalToken == "" {
		t.Fatalf("unexpected approval response: %+v", resp)
	}
	stored, err := store.GetMCPApproval(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetMCPApproval returned error: %v", err)
	}
	if stored.ApprovedBy != "user-a" || stored.PolicyVersion != resp.PolicyVersion {
		t.Fatalf("approval metadata not persisted: %+v", stored)
	}
}
