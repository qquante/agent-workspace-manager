package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"stola-os/internal/secrets"
)

const DefaultCheckTimeout = 10 * time.Second

type CheckOptions struct {
	HTTPClient *http.Client
	Timeout    time.Duration
}

type CheckResult struct {
	ProviderID      string
	Title           string
	Kind            string
	Endpoint        string
	StatusCode      int
	ModelCount      int
	Duration        time.Duration
	UsedSecretNames []string
}

func CheckProvider(ctx context.Context, manifest Manifest, providerID string, bundle secrets.Bundle, opts CheckOptions) (CheckResult, error) {
	provider, ok := FindProvider(manifest, providerID)
	if !ok {
		return CheckResult{}, fmt.Errorf("provider %q not found", providerID)
	}
	if !provider.Enabled {
		return CheckResult{}, fmt.Errorf("provider %q is disabled", provider.ID)
	}
	if !supportsModelListCheck(provider) {
		return CheckResult{}, fmt.Errorf("provider %q kind %q is not supported by providers check yet", provider.ID, provider.Kind)
	}

	values := secrets.ValuesByName(bundle.Secrets)
	apiKey, ok := values[provider.APIKeyEnv]
	if strings.TrimSpace(provider.APIKeyEnv) == "" {
		return CheckResult{}, fmt.Errorf("provider %q has no api_key_env in provider manifest", provider.ID)
	}
	if !ok || strings.TrimSpace(apiKey) == "" {
		return CheckResult{}, fmt.Errorf("provider %q requires secret %s in --secrets-file", provider.ID, provider.APIKeyEnv)
	}
	endpoint, err := providerModelsEndpoint(provider, values)
	if err != nil {
		return CheckResult{}, err
	}
	usedSecretNames := []string{provider.APIKeyEnv}
	for _, name := range openRouterHeaderSecretNames(provider, values) {
		usedSecretNames = append(usedSecretNames, name)
	}
	sort.Strings(usedSecretNames)

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultCheckTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return CheckResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	addOpenRouterHeaders(req, provider, values)

	start := time.Now()
	resp, err := client.Do(req)
	duration := time.Since(start)
	result := CheckResult{
		ProviderID:      provider.ID,
		Title:           provider.Title,
		Kind:            provider.Kind,
		Endpoint:        endpoint,
		Duration:        duration,
		UsedSecretNames: usedSecretNames,
	}
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	result.StatusCode = resp.StatusCode

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return result, fmt.Errorf("provider %q returned HTTP %d", provider.ID, resp.StatusCode)
	}
	var parsed struct {
		Data []json.RawMessage `json:"data"`
	}
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 8*1024*1024))
	if err := decoder.Decode(&parsed); err != nil {
		return result, fmt.Errorf("provider %q returned invalid models JSON: %w", provider.ID, err)
	}
	result.ModelCount = len(parsed.Data)
	return result, nil
}

func supportsModelListCheck(provider Provider) bool {
	switch provider.Kind {
	case "openai_compatible", "litellm_proxy", "local":
		return true
	default:
		return false
	}
}

func providerModelsEndpoint(provider Provider, values map[string]string) (string, error) {
	baseURL := strings.TrimSpace(provider.BaseURL)
	if baseURL == "" {
		return "", fmt.Errorf("provider %q has no base_url for model list check", provider.ID)
	}
	missing := map[string]bool{}
	expanded := os.Expand(baseURL, func(name string) string {
		if value, ok := values[name]; ok {
			return value
		}
		missing[name] = true
		return ""
	})
	if len(missing) > 0 {
		names := make([]string, 0, len(missing))
		for name := range missing {
			names = append(names, name)
		}
		sort.Strings(names)
		return "", fmt.Errorf("provider %q base_url requires secret(s): %s", provider.ID, strings.Join(names, ","))
	}
	expanded = strings.TrimRight(strings.TrimSpace(expanded), "/")
	if expanded == "" || strings.Contains(expanded, "${") {
		return "", fmt.Errorf("provider %q has invalid base_url", provider.ID)
	}
	return expanded + "/models", nil
}

func addOpenRouterHeaders(req *http.Request, provider Provider, values map[string]string) {
	if provider.ID != "openrouter" {
		return
	}
	if referer := firstPresent(values, "OPENROUTER_HTTP_REFERER", "OR_SITE_URL"); referer != "" {
		req.Header.Set("HTTP-Referer", referer)
	}
	if title := firstPresent(values, "OPENROUTER_APP_TITLE", "OR_APP_NAME"); title != "" {
		req.Header.Set("X-Title", title)
	}
}

func openRouterHeaderSecretNames(provider Provider, values map[string]string) []string {
	if provider.ID != "openrouter" {
		return nil
	}
	var out []string
	for _, name := range []string{"OPENROUTER_HTTP_REFERER", "OR_SITE_URL", "OPENROUTER_APP_TITLE", "OR_APP_NAME"} {
		if value := strings.TrimSpace(values[name]); value != "" {
			out = append(out, name)
		}
	}
	return out
}

func firstPresent(values map[string]string, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(values[name]); value != "" {
			return value
		}
	}
	return ""
}
