package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"stola-os/internal/secrets"
)

func TestCheckProviderOpenAICompatibleUsesModelsEndpoint(t *testing.T) {
	var gotAuth string
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"model-a"},{"id":"model-b"}]}`))
	}))
	defer server.Close()

	manifest := Manifest{
		Version:         1,
		Name:            "test-providers",
		DefaultProvider: "openai",
		Providers: []Provider{{
			ID:        "openai",
			Title:     "OpenAI",
			Kind:      "openai_compatible",
			BaseURL:   server.URL + "/v1",
			APIKeyEnv: "OPENAI_API_KEY",
			Enabled:   true,
		}},
	}
	bundle := loadTestProviderSecrets(t, "OPENAI_API_KEY=sk-test-secret\n", []string{"OPENAI_API_KEY"})

	result, err := CheckProvider(context.Background(), manifest, "openai", bundle, CheckOptions{})
	if err != nil {
		t.Fatalf("CheckProvider returned error: %v", err)
	}
	if gotAuth != "Bearer sk-test-secret" {
		t.Fatalf("unexpected Authorization header: %q", gotAuth)
	}
	if gotPath != "/v1/models" {
		t.Fatalf("unexpected request path: %q", gotPath)
	}
	if result.ModelCount != 2 || result.StatusCode != http.StatusOK || result.Endpoint != server.URL+"/v1/models" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestCheckProviderOpenRouterUsesOptionalHeaders(t *testing.T) {
	var gotReferer string
	var gotTitle string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		gotReferer = r.Header.Get("HTTP-Referer")
		gotTitle = r.Header.Get("X-Title")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"openai/gpt-test"}]}`))
	}))
	defer server.Close()

	manifest := Manifest{
		Version:         1,
		Name:            "test-providers",
		DefaultProvider: "openrouter",
		Providers: []Provider{{
			ID:        "openrouter",
			Title:     "OpenRouter",
			Kind:      "openai_compatible",
			BaseURL:   server.URL + "/api/v1",
			APIKeyEnv: "OPENROUTER_API_KEY",
			ExtraEnv:  []string{"OR_SITE_URL", "OR_APP_NAME"},
			Enabled:   true,
		}},
	}
	bundle := loadTestProviderSecrets(t, "OPENROUTER_API_KEY=or-test\nOR_SITE_URL=https://stola.local\nOR_APP_NAME=Stola\n", []string{"OPENROUTER_API_KEY", "OR_SITE_URL", "OR_APP_NAME"})

	result, err := CheckProvider(context.Background(), manifest, "openrouter", bundle, CheckOptions{})
	if err != nil {
		t.Fatalf("CheckProvider returned error: %v", err)
	}
	if gotReferer != "https://stola.local" || gotTitle != "Stola" {
		t.Fatalf("unexpected OpenRouter headers referer=%q title=%q", gotReferer, gotTitle)
	}
	if result.ModelCount != 1 || strings.Join(result.UsedSecretNames, ",") != "OPENROUTER_API_KEY,OR_APP_NAME,OR_SITE_URL" {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestCheckProviderRequiresProviderSecret(t *testing.T) {
	manifest := Manifest{
		Version:         1,
		Name:            "test-providers",
		DefaultProvider: "openai",
		Providers: []Provider{{
			ID:        "openai",
			Title:     "OpenAI",
			Kind:      "openai_compatible",
			BaseURL:   "https://api.openai.com/v1",
			APIKeyEnv: "OPENAI_API_KEY",
			Enabled:   true,
		}},
	}
	bundle := loadTestProviderSecrets(t, "ANTHROPIC_API_KEY=anthropic-test\n", []string{"ANTHROPIC_API_KEY"})

	_, err := CheckProvider(context.Background(), manifest, "openai", bundle, CheckOptions{})
	if err == nil || !strings.Contains(err.Error(), "requires secret OPENAI_API_KEY") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckProviderHTTPErrorDoesNotExposeSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"bad key sk-test-secret"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	manifest := Manifest{
		Version:         1,
		Name:            "test-providers",
		DefaultProvider: "openai",
		Providers: []Provider{{
			ID:        "openai",
			Title:     "OpenAI",
			Kind:      "openai_compatible",
			BaseURL:   server.URL + "/v1",
			APIKeyEnv: "OPENAI_API_KEY",
			Enabled:   true,
		}},
	}
	bundle := loadTestProviderSecrets(t, "OPENAI_API_KEY=sk-test-secret\n", []string{"OPENAI_API_KEY"})

	result, err := CheckProvider(context.Background(), manifest, "openai", bundle, CheckOptions{})
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	if result.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected status code: %#v", result)
	}
	if strings.Contains(err.Error(), "sk-test-secret") {
		t.Fatalf("error exposed secret: %v", err)
	}
}

func loadTestProviderSecrets(t *testing.T, content string, allowed []string) secrets.Bundle {
	t.Helper()
	dir := t.TempDir()
	filePath := filepath.Join(dir, "provider.env")
	if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	bundle, err := secrets.LoadProviderEnvFile(filePath, allowed)
	if err != nil {
		t.Fatalf("LoadProviderEnvFile returned error: %v", err)
	}
	return bundle
}
