package llmproxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"stola-os/internal/config"
	"stola-os/internal/state"
)

func TestLLMProxyChatCompletionRecordsUsageAndEnforcesBudget(t *testing.T) {
	dir := t.TempDir()
	var gotAuth string
	var gotModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		var req struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		gotModel = req.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl-test",
			"object":"chat.completion",
			"model":"openai/gpt-4o-mini",
			"choices":[{"index":0,"message":{"role":"assistant","content":"STOLAS_PROXY_OK"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"cost_usd":0.002}
		}`))
	}))
	defer upstream.Close()

	manifestPath := filepath.Join(dir, "providers.yaml")
	if err := os.WriteFile(manifestPath, []byte(`
version: 1
name: test-providers
default_provider: openrouter
providers:
  - id: openrouter
    title: OpenRouter Test
    kind: openai_compatible
    base_url: `+upstream.URL+`/v1
    api_key_env: OPENROUTER_API_KEY
    litellm_model: openrouter/openai/gpt-4o-mini
    enabled: true
`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	secretsPath := filepath.Join(dir, "provider.env")
	if err := os.WriteFile(secretsPath, []byte("OPENROUTER_API_KEY=upstream-secret\n"), 0o600); err != nil {
		t.Fatalf("write secrets: %v", err)
	}
	store, err := state.Open(filepath.Join(dir, "stola.db"))
	if err != nil {
		t.Fatalf("state.Open returned error: %v", err)
	}
	defer store.Close()
	if err := store.CreateVPS(context.Background(), state.VPS{ID: "workspace-a", Name: "workspace-a", Plan: "nano", Status: "running", Backend: "mock"}); err != nil {
		t.Fatalf("CreateVPS returned error: %v", err)
	}
	rawKey, err := state.GenerateLLMProxyKey()
	if err != nil {
		t.Fatalf("GenerateLLMProxyKey returned error: %v", err)
	}
	key, err := store.CreateLLMProxyKey(context.Background(), state.LLMProxyKey{
		KeyHash:     state.HashLLMProxyKey(rawKey),
		WorkspaceID: "workspace-a",
		UserID:      "user-a",
		ProviderID:  "openrouter",
		Models:      []string{"openai/gpt-4o-mini"},
		BudgetUSD:   0.001,
		RPM:         10,
		TPM:         1000,
	})
	if err != nil {
		t.Fatalf("CreateLLMProxyKey returned error: %v", err)
	}
	server, err := NewServer(config.Config{
		LLMProxy: config.LLMProxyConfig{
			ProvidersManifest: manifestPath,
			SecretsFile:       secretsPath,
		},
	}, store, nil)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	body := strings.NewReader(`{"model":"openrouter/openai/gpt-4o-mini","messages":[{"role":"user","content":"test"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if gotAuth != "Bearer upstream-secret" {
		t.Fatalf("unexpected upstream auth: %q", gotAuth)
	}
	if gotModel != "openai/gpt-4o-mini" {
		t.Fatalf("unexpected upstream model: %q", gotModel)
	}
	if !strings.Contains(rec.Body.String(), "STOLAS_PROXY_OK") {
		t.Fatalf("unexpected response body: %s", rec.Body.String())
	}
	summary, err := store.SummarizeUsage(context.Background(), state.UsageFilter{KeyID: key.ID})
	if err != nil {
		t.Fatalf("SummarizeUsage returned error: %v", err)
	}
	if summary.Events != 1 || summary.TotalTokens != 15 || summary.CostUSD < 0.0019 {
		t.Fatalf("unexpected usage summary: %+v", summary)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"test"}]}`))
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected budget 429, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLLMProxyRejectsDisallowedModelAndEnforcesTPM(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "providers.yaml")
	if err := os.WriteFile(manifestPath, []byte(`
version: 1
name: test-providers
default_provider: openrouter
providers:
  - id: openrouter
    title: OpenRouter Test
    kind: openai_compatible
    base_url: http://127.0.0.1:1/v1
    api_key_env: OPENROUTER_API_KEY
    enabled: true
`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	secretsPath := filepath.Join(dir, "provider.env")
	if err := os.WriteFile(secretsPath, []byte("OPENROUTER_API_KEY=upstream-secret\n"), 0o600); err != nil {
		t.Fatalf("write secrets: %v", err)
	}
	store, err := state.Open(filepath.Join(dir, "stola.db"))
	if err != nil {
		t.Fatalf("state.Open returned error: %v", err)
	}
	defer store.Close()
	rawKey, err := state.GenerateLLMProxyKey()
	if err != nil {
		t.Fatalf("GenerateLLMProxyKey returned error: %v", err)
	}
	if _, err := store.CreateLLMProxyKey(context.Background(), state.LLMProxyKey{
		KeyHash:     state.HashLLMProxyKey(rawKey),
		WorkspaceID: "workspace-a",
		UserID:      "user-a",
		ProviderID:  "openrouter",
		Models:      []string{"openai/gpt-4o-mini"},
		TPM:         2,
	}); err != nil {
		t.Fatalf("CreateLLMProxyKey returned error: %v", err)
	}
	server, err := NewServer(config.Config{LLMProxy: config.LLMProxyConfig{ProvidersManifest: manifestPath, SecretsFile: secretsPath}}, store, nil)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"anthropic/claude","messages":[]}`))
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected disallowed model 403, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"this request is larger than two estimated tokens"}]}`))
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec = httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected tpm 429, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLLMProxyStreamingRecordsUsageWithCostFallback(t *testing.T) {
	dir := t.TempDir()
	var gotIncludeUsage bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		options, _ := req["stream_options"].(map[string]any)
		gotIncludeUsage, _ = options["include_usage"].(bool)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"STOLAS_\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"STREAM_OK\"}}],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":4,\"total_tokens\":12}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	manifestPath := filepath.Join(dir, "providers.yaml")
	if err := os.WriteFile(manifestPath, []byte(`
version: 1
name: test-providers
default_provider: openrouter
providers:
  - id: openrouter
    title: OpenRouter Test
    kind: openai_compatible
    base_url: `+upstream.URL+`/v1
    api_key_env: OPENROUTER_API_KEY
    enabled: true
`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	secretsPath := filepath.Join(dir, "provider.env")
	if err := os.WriteFile(secretsPath, []byte("OPENROUTER_API_KEY=upstream-secret\n"), 0o600); err != nil {
		t.Fatalf("write secrets: %v", err)
	}
	store, err := state.Open(filepath.Join(dir, "stola.db"))
	if err != nil {
		t.Fatalf("state.Open returned error: %v", err)
	}
	defer store.Close()
	rawKey, err := state.GenerateLLMProxyKey()
	if err != nil {
		t.Fatalf("GenerateLLMProxyKey returned error: %v", err)
	}
	key, err := store.CreateLLMProxyKey(context.Background(), state.LLMProxyKey{
		KeyHash:     state.HashLLMProxyKey(rawKey),
		WorkspaceID: "workspace-a",
		UserID:      "user-a",
		ProviderID:  "openrouter",
		Models:      []string{"openai/gpt-4o-mini"},
		BudgetUSD:   1,
		RPM:         10,
		TPM:         1000,
	})
	if err != nil {
		t.Fatalf("CreateLLMProxyKey returned error: %v", err)
	}
	server, err := NewServer(config.Config{LLMProxy: config.LLMProxyConfig{ProvidersManifest: manifestPath, SecretsFile: secretsPath}}, store, nil)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openrouter/openai/gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"test"}]}`))
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected streaming 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !gotIncludeUsage {
		t.Fatalf("proxy did not request stream_options.include_usage")
	}
	if !strings.Contains(rec.Body.String(), "STOLAS_") || !strings.Contains(rec.Body.String(), "STREAM_OK") {
		t.Fatalf("unexpected streaming body: %s", rec.Body.String())
	}
	summary, err := store.SummarizeUsage(context.Background(), state.UsageFilter{KeyID: key.ID})
	if err != nil {
		t.Fatalf("SummarizeUsage returned error: %v", err)
	}
	if summary.Events != 1 || summary.TotalTokens != 12 || summary.CostUSD <= 0 {
		t.Fatalf("unexpected streaming usage summary: %+v", summary)
	}
}
