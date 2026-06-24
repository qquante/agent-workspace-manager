package state

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateAndListVPS(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "stola.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	err = store.CreateVPS(ctx, VPS{
		ID:      "mock-1",
		Name:    "nano-1",
		Plan:    "nano",
		Status:  "created",
		Backend: "mock",
	})
	if err != nil {
		t.Fatalf("CreateVPS returned error: %v", err)
	}
	list, err := store.ListVPS(ctx)
	if err != nil {
		t.Fatalf("ListVPS returned error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one VPS, got %d", len(list))
	}
	if list[0].ID != "mock-1" || list[0].Plan != "nano" {
		t.Fatalf("unexpected VPS: %+v", list[0])
	}
}

func TestCreateAndListProxmoxVPSMetadata(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "stola.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	err = store.CreateVPS(ctx, VPS{
		ID:         "proxmox-lxc-101",
		Name:       "nano-a",
		Plan:       "nano",
		Status:     "creating",
		Backend:    "proxmox",
		ExternalID: "101",
		TaskID:     "UPID:pve:101",
	})
	if err != nil {
		t.Fatalf("CreateVPS returned error: %v", err)
	}
	list, err := store.ListVPS(ctx)
	if err != nil {
		t.Fatalf("ListVPS returned error: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one VPS, got %d", len(list))
	}
	if list[0].ExternalID != "101" || list[0].TaskID != "UPID:pve:101" {
		t.Fatalf("unexpected Proxmox metadata: %+v", list[0])
	}
}

func TestGetUpdateAndDeleteVPS(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "stola.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	err = store.CreateVPS(ctx, VPS{
		ID:         "docker-stola-nano-a",
		Name:       "nano-a",
		Plan:       "nano",
		Status:     "running",
		Backend:    "docker",
		ExternalID: "abc123",
	})
	if err != nil {
		t.Fatalf("CreateVPS returned error: %v", err)
	}
	vps, err := store.GetVPS(ctx, "docker-stola-nano-a")
	if err != nil {
		t.Fatalf("GetVPS returned error: %v", err)
	}
	if vps.ExternalID != "abc123" || vps.Status != "running" {
		t.Fatalf("unexpected VPS: %+v", vps)
	}
	if err := store.UpdateVPSStatus(ctx, "docker-stola-nano-a", "stopped"); err != nil {
		t.Fatalf("UpdateVPSStatus returned error: %v", err)
	}
	vps, err = store.GetVPS(ctx, "docker-stola-nano-a")
	if err != nil {
		t.Fatalf("GetVPS after update returned error: %v", err)
	}
	if vps.Status != "stopped" {
		t.Fatalf("status was not updated: %+v", vps)
	}
	if err := store.DeleteVPS(ctx, "docker-stola-nano-a"); err != nil {
		t.Fatalf("DeleteVPS returned error: %v", err)
	}
	list, err := store.ListVPS(ctx)
	if err != nil {
		t.Fatalf("ListVPS returned error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no VPS rows, got %+v", list)
	}
}

func TestHeartbeat(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "stola.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	at := time.Now().UTC()
	if err := store.SetHeartbeat(ctx, "stola-provisioner", "running", at); err != nil {
		t.Fatalf("SetHeartbeat returned error: %v", err)
	}
	hb, err := store.GetHeartbeat(ctx, "stola-provisioner")
	if err != nil {
		t.Fatalf("GetHeartbeat returned error: %v", err)
	}
	if hb.Service != "stola-provisioner" || hb.Status != "running" {
		t.Fatalf("unexpected heartbeat: %+v", hb)
	}
}

func TestMCPApprovalCreateConsumeOneShot(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "stola.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	created, err := store.CreateMCPApproval(ctx, MCPApproval{
		VPSID:          "docker-stola-a",
		Server:         "filesystem",
		Tool:           "write_file",
		ArgumentDigest: "digest-a",
		ArgumentKeys:   []string{"content", "path", "path"},
		CreatedAt:      now,
		ExpiresAt:      now.Add(time.Minute),
		Reason:         "writes file",
		Rule:           "write_file",
		ApprovedBy:     "user-a",
		PolicyVersion:  "policy-a",
	})
	if err != nil {
		t.Fatalf("CreateMCPApproval returned error: %v", err)
	}
	if !strings.HasPrefix(created.ID, "mcpappr-") {
		t.Fatalf("unexpected approval id: %s", created.ID)
	}
	if created.Status != MCPApprovalStatusPending {
		t.Fatalf("expected pending approval, got %+v", created)
	}
	if strings.Join(created.ArgumentKeys, ",") != "content,path" {
		t.Fatalf("argument keys were not normalized: %#v", created.ArgumentKeys)
	}
	if created.ApprovedBy != "user-a" || created.PolicyVersion != "policy-a" {
		t.Fatalf("approval metadata mismatch: %+v", created)
	}

	consumed, err := store.ConsumeMCPApproval(ctx, created.ID, "docker-stola-a", "filesystem", "write_file", "digest-a", now.Add(time.Second))
	if err != nil {
		t.Fatalf("ConsumeMCPApproval returned error: %v", err)
	}
	if consumed.Status != MCPApprovalStatusUsed || consumed.UsedAt.IsZero() {
		t.Fatalf("approval was not consumed: %+v", consumed)
	}
	if consumed.ApprovedBy != "user-a" || consumed.PolicyVersion != "policy-a" {
		t.Fatalf("consumed approval metadata mismatch: %+v", consumed)
	}
	if _, err := store.ConsumeMCPApproval(ctx, created.ID, "docker-stola-a", "filesystem", "write_file", "digest-a", now.Add(2*time.Second)); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("expected already-used error, got %v", err)
	}
}

func TestMCPApprovalRejectsMismatchedRequestWithoutConsuming(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "stola.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	created, err := store.CreateMCPApproval(ctx, MCPApproval{
		VPSID:          "docker-stola-a",
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

	if _, err := store.ConsumeMCPApproval(ctx, created.ID, "docker-stola-a", "filesystem", "write_file", "digest-b", now.Add(time.Second)); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected mismatch error, got %v", err)
	}
	stored, err := store.GetMCPApproval(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetMCPApproval returned error: %v", err)
	}
	if stored.Status != MCPApprovalStatusPending {
		t.Fatalf("mismatched approval should remain pending: %+v", stored)
	}
}

func TestMCPApprovalRejectsExpiredApprovalWithoutConsuming(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "stola.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	created, err := store.CreateMCPApproval(ctx, MCPApproval{
		VPSID:          "docker-stola-a",
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

	if _, err := store.ConsumeMCPApproval(ctx, created.ID, "docker-stola-a", "filesystem", "write_file", "digest-a", now.Add(2*time.Minute)); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired error, got %v", err)
	}
	stored, err := store.GetMCPApproval(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetMCPApproval returned error: %v", err)
	}
	if stored.Status != MCPApprovalStatusPending {
		t.Fatalf("expired approval should remain pending: %+v", stored)
	}
}

func TestApproveMCPApprovalStoresControlPlaneMetadata(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "stola.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	created, err := store.CreateMCPApproval(ctx, MCPApproval{
		VPSID:          "docker-stola-a",
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
	approved, err := store.ApproveMCPApproval(ctx, created.ID, "user-a", "policy-a", now.Add(time.Second))
	if err != nil {
		t.Fatalf("ApproveMCPApproval returned error: %v", err)
	}
	if approved.ApprovedBy != "user-a" || approved.PolicyVersion != "policy-a" {
		t.Fatalf("approval metadata mismatch: %+v", approved)
	}
	consumed, err := store.ConsumeMCPApproval(ctx, created.ID, "docker-stola-a", "filesystem", "write_file", "digest-a", now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("ConsumeMCPApproval returned error: %v", err)
	}
	if consumed.ApprovedBy != "user-a" || consumed.PolicyVersion != "policy-a" {
		t.Fatalf("consumed metadata mismatch: %+v", consumed)
	}
}

func TestListMCPApprovalsFiltersStatus(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "stola.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	pending, err := store.CreateMCPApproval(ctx, MCPApproval{
		VPSID:          "docker-stola-a",
		Server:         "filesystem",
		Tool:           "write_file",
		ArgumentDigest: "digest-a",
		ArgumentKeys:   []string{"path"},
		CreatedAt:      now,
		ExpiresAt:      now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateMCPApproval pending returned error: %v", err)
	}
	used, err := store.CreateMCPApproval(ctx, MCPApproval{
		VPSID:          "docker-stola-a",
		Server:         "filesystem",
		Tool:           "write_file",
		ArgumentDigest: "digest-b",
		ArgumentKeys:   []string{"path"},
		CreatedAt:      now.Add(time.Second),
		ExpiresAt:      now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateMCPApproval used returned error: %v", err)
	}
	if _, err := store.ConsumeMCPApproval(ctx, used.ID, "docker-stola-a", "filesystem", "write_file", "digest-b", now.Add(2*time.Second)); err != nil {
		t.Fatalf("ConsumeMCPApproval returned error: %v", err)
	}
	list, err := store.ListMCPApprovals(ctx, MCPApprovalFilter{Status: MCPApprovalStatusPending, Limit: 10})
	if err != nil {
		t.Fatalf("ListMCPApprovals returned error: %v", err)
	}
	if len(list) != 1 || list[0].ID != pending.ID {
		t.Fatalf("unexpected pending list: %+v", list)
	}
}

func TestApproveMCPApprovalRejectsExpiredIntent(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "stola.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	created, err := store.CreateMCPApproval(ctx, MCPApproval{
		VPSID:          "docker-stola-a",
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
	if _, err := store.ApproveMCPApproval(ctx, created.ID, "user-a", "policy-a", now.Add(2*time.Minute)); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired error, got %v", err)
	}
}

func TestConsumeMCPRateLimit(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "stola.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	first, err := store.ConsumeMCPRateLimit(ctx, "mcp|vps=a|server=filesystem|tool=read", 2, time.Minute, now)
	if err != nil {
		t.Fatalf("first ConsumeMCPRateLimit returned error: %v", err)
	}
	if first.Count != 1 || first.Remaining != 1 {
		t.Fatalf("unexpected first rate limit result: %+v", first)
	}
	second, err := store.ConsumeMCPRateLimit(ctx, "mcp|vps=a|server=filesystem|tool=read", 2, time.Minute, now.Add(time.Second))
	if err != nil {
		t.Fatalf("second ConsumeMCPRateLimit returned error: %v", err)
	}
	if second.Count != 2 || second.Remaining != 0 {
		t.Fatalf("unexpected second rate limit result: %+v", second)
	}
	_, err = store.ConsumeMCPRateLimit(ctx, "mcp|vps=a|server=filesystem|tool=read", 2, time.Minute, now.Add(2*time.Second))
	var exceeded MCPRateLimitExceededError
	if err == nil || !errors.As(err, &exceeded) {
		t.Fatalf("expected rate limit exceeded error, got %v", err)
	}
	if !exceeded.ResetAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("unexpected reset time: %+v", exceeded)
	}
	reset, err := store.ConsumeMCPRateLimit(ctx, "mcp|vps=a|server=filesystem|tool=read", 2, time.Minute, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("reset ConsumeMCPRateLimit returned error: %v", err)
	}
	if reset.Count != 1 || reset.Remaining != 1 {
		t.Fatalf("unexpected reset result: %+v", reset)
	}

	tokenCount, err := store.ConsumeRateLimitAmount(ctx, "llm|key=a|tpm", 15, 20, time.Minute, now)
	if err != nil {
		t.Fatalf("ConsumeRateLimitAmount returned error: %v", err)
	}
	if tokenCount.Count != 15 || tokenCount.Remaining != 5 {
		t.Fatalf("unexpected token rate result: %+v", tokenCount)
	}
	_, err = store.ConsumeRateLimitAmount(ctx, "llm|key=a|tpm", 6, 20, time.Minute, now.Add(time.Second))
	if err == nil || !errors.As(err, &exceeded) {
		t.Fatalf("expected token rate limit exceeded error, got %v", err)
	}
}

func TestUsageRecordListAndSummary(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "stola.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	first, err := store.RecordUsage(ctx, UsageEvent{
		WorkspaceID:  "workspace-a",
		UserID:       "user-a",
		ProviderID:   "openrouter",
		Model:        "openrouter/openai/gpt-4o-mini",
		InputTokens:  10,
		OutputTokens: 5,
		CostUSD:      0.001,
		LatencyMS:    250,
		CreatedAt:    now,
	})
	if err != nil {
		t.Fatalf("RecordUsage first returned error: %v", err)
	}
	if first.TotalTokens != 15 || first.Operation != "chat_completion" || first.Status != "success" {
		t.Fatalf("usage defaults were not applied: %+v", first)
	}
	if _, err := store.RecordUsage(ctx, UsageEvent{
		WorkspaceID: "workspace-a",
		UserID:      "user-a",
		ProviderID:  "openrouter",
		Model:       "openrouter/openai/gpt-4o-mini",
		TotalTokens: -1,
	}); err == nil || !strings.Contains(err.Error(), "must not be negative") {
		t.Fatalf("expected negative counter error, got %v", err)
	}
	_, err = store.RecordUsage(ctx, UsageEvent{
		WorkspaceID:  "workspace-a",
		UserID:       "user-a",
		ProviderID:   "openrouter",
		Model:        "openrouter/openai/gpt-4o-mini",
		Operation:    "chat_completion",
		InputTokens:  3,
		OutputTokens: 2,
		TotalTokens:  5,
		CostUSD:      0.002,
		LatencyMS:    400,
		Status:       "error",
		Error:        "upstream timeout",
		CreatedAt:    now.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("RecordUsage second returned error: %v", err)
	}
	list, err := store.ListUsage(ctx, UsageFilter{WorkspaceID: "workspace-a", Limit: 10})
	if err != nil {
		t.Fatalf("ListUsage returned error: %v", err)
	}
	if len(list) != 2 || list[0].Status != "error" || list[1].Status != "success" {
		t.Fatalf("unexpected usage list: %+v", list)
	}
	summary, err := store.SummarizeUsage(ctx, UsageFilter{WorkspaceID: "workspace-a", ProviderID: "openrouter"})
	if err != nil {
		t.Fatalf("SummarizeUsage returned error: %v", err)
	}
	if summary.Events != 2 || summary.InputTokens != 13 || summary.OutputTokens != 7 || summary.TotalTokens != 20 || summary.Errors != 1 {
		t.Fatalf("unexpected usage summary: %+v", summary)
	}
	if summary.CostUSD < 0.0029 || summary.CostUSD > 0.0031 {
		t.Fatalf("unexpected usage cost: %+v", summary)
	}
}

func TestLLMProxyKeyCreateLookupAndUsageSummary(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "stola.db"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	rawKey, err := GenerateLLMProxyKey()
	if err != nil {
		t.Fatalf("GenerateLLMProxyKey returned error: %v", err)
	}
	if !strings.HasPrefix(rawKey, "sk-stolas-") {
		t.Fatalf("unexpected raw key prefix: %s", rawKey)
	}
	created, err := store.CreateLLMProxyKey(ctx, LLMProxyKey{
		KeyHash:        HashLLMProxyKey(rawKey),
		WorkspaceID:    "workspace-a",
		UserID:         "user-a",
		ProviderID:     "openrouter",
		Models:         []string{"openai/gpt-4o-mini", "openai/gpt-4o-mini", ""},
		BudgetUSD:      0.01,
		RPM:            5,
		TPM:            100,
		MaxConcurrency: 2,
	})
	if err != nil {
		t.Fatalf("CreateLLMProxyKey returned error: %v", err)
	}
	if created.ID == "" || created.Status != "active" || created.TPM != 100 || created.MaxConcurrency != 2 || strings.Join(created.Models, ",") != "openai/gpt-4o-mini" {
		t.Fatalf("unexpected created key: %+v", created)
	}
	lookedUp, err := store.FindLLMProxyKeyByHash(ctx, HashLLMProxyKey(rawKey))
	if err != nil {
		t.Fatalf("FindLLMProxyKeyByHash returned error: %v", err)
	}
	if lookedUp.ID != created.ID || lookedUp.KeyHash != created.KeyHash {
		t.Fatalf("unexpected lookup result: %+v", lookedUp)
	}
	if strings.Contains(lookedUp.KeyHash, rawKey) {
		t.Fatalf("stored key hash contains raw key")
	}
	if err := store.TouchLLMProxyKeyUsedAt(ctx, created.ID, time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("TouchLLMProxyKeyUsedAt returned error: %v", err)
	}
	list, err := store.ListLLMProxyKeys(ctx, LLMProxyKeyFilter{WorkspaceID: "workspace-a", Status: "active"})
	if err != nil {
		t.Fatalf("ListLLMProxyKeys returned error: %v", err)
	}
	if len(list) != 1 || list[0].LastUsedAt.IsZero() {
		t.Fatalf("unexpected key list: %+v", list)
	}
	if _, err := store.RecordUsage(ctx, UsageEvent{
		KeyID:       created.ID,
		WorkspaceID: "workspace-a",
		UserID:      "user-a",
		ProviderID:  "openrouter",
		Model:       "openai/gpt-4o-mini",
		TotalTokens: 12,
		CostUSD:     0.004,
	}); err != nil {
		t.Fatalf("RecordUsage returned error: %v", err)
	}
	summary, err := store.SummarizeUsage(ctx, UsageFilter{KeyID: created.ID})
	if err != nil {
		t.Fatalf("SummarizeUsage returned error: %v", err)
	}
	if summary.Events != 1 || summary.TotalTokens != 12 || summary.CostUSD < 0.0039 {
		t.Fatalf("unexpected key usage summary: %+v", summary)
	}

	rotatedRaw, err := GenerateLLMProxyKey()
	if err != nil {
		t.Fatalf("GenerateLLMProxyKey rotated returned error: %v", err)
	}
	rotated, err := store.RotateLLMProxyKey(ctx, created.ID, HashLLMProxyKey(rotatedRaw), time.Now().UTC())
	if err != nil {
		t.Fatalf("RotateLLMProxyKey returned error: %v", err)
	}
	if rotated.KeyHash == created.KeyHash || rotated.Status != "active" || !rotated.LastUsedAt.IsZero() {
		t.Fatalf("unexpected rotated key: %+v", rotated)
	}
	if _, err := store.FindLLMProxyKeyByHash(ctx, HashLLMProxyKey(rawKey)); err == nil {
		t.Fatal("old rotated raw key should not look up")
	}
	if _, err := store.FindLLMProxyKeyByHash(ctx, HashLLMProxyKey(rotatedRaw)); err != nil {
		t.Fatalf("rotated raw key lookup failed: %v", err)
	}
	revoked, err := store.RevokeLLMProxyKey(ctx, created.ID)
	if err != nil {
		t.Fatalf("RevokeLLMProxyKey returned error: %v", err)
	}
	if revoked.Status != "revoked" {
		t.Fatalf("key was not revoked: %+v", revoked)
	}
}
