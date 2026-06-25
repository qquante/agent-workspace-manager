package providers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultProviderManifestValidatesAndRenders(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "manifests", "providers", "default.yaml")
	manifest, raw, err := LoadFile(manifestPath)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}
	if manifest.Name != "stola-default-providers" {
		t.Fatalf("unexpected manifest name: %s", manifest.Name)
	}
	if manifest.DefaultProvider != "openrouter" {
		t.Fatalf("unexpected default provider: %s", manifest.DefaultProvider)
	}
	if len(manifest.Providers) < 20 {
		t.Fatalf("expected broad provider catalog, got %d providers", len(manifest.Providers))
	}

	bundle, err := RenderBundle(manifest, raw)
	if err != nil {
		t.Fatalf("RenderBundle returned error: %v", err)
	}
	for _, name := range []string{
		WorkspaceManifestPath,
		WorkspaceCatalogPath,
		WorkspaceEnvExamplePath,
		WorkspaceLiteLLMConfigPath,
		WorkspaceOpenAICompatPath,
	} {
		if len(bundle.Files[name]) == 0 {
			t.Fatalf("expected rendered file %s", name)
		}
	}
	envExample := string(bundle.Files[WorkspaceEnvExamplePath])
	for _, envName := range []string{"OPENROUTER_API_KEY", "OPENAI_API_KEY", "ANTHROPIC_API_KEY"} {
		if !strings.Contains(envExample, envName) {
			t.Fatalf("env example missing %s:\n%s", envName, envExample)
		}
	}
	litellm := string(bundle.Files[WorkspaceLiteLLMConfigPath])
	for _, model := range []string{"openrouter/openai/gpt-5.5", "openai/gpt-5.5", "anthropic/claude-fable-5"} {
		if !strings.Contains(litellm, model) {
			t.Fatalf("LiteLLM config missing %s:\n%s", model, litellm)
		}
	}
}

func TestWriteBundleToDirWritesProviderFiles(t *testing.T) {
	manifest := Manifest{
		Version:         1,
		Name:            "test-providers",
		DefaultProvider: "openai",
		Providers: []Provider{
			{
				ID:           "openai",
				Title:        "OpenAI",
				Kind:         "openai_compatible",
				BaseURL:      "https://api.openai.com/v1",
				APIKeyEnv:    "OPENAI_API_KEY",
				LiteLLMModel: "openai/gpt-5.5",
				Enabled:      true,
			},
		},
	}
	bundle, err := RenderBundle(manifest, nil)
	if err != nil {
		t.Fatalf("RenderBundle returned error: %v", err)
	}
	dir := t.TempDir()
	if err := WriteBundleToDir(bundle, dir); err != nil {
		t.Fatalf("WriteBundleToDir returned error: %v", err)
	}
	for _, rel := range []string{
		WorkspaceManifestPath,
		WorkspaceCatalogPath,
		WorkspaceEnvExamplePath,
		WorkspaceLiteLLMConfigPath,
		WorkspaceOpenAICompatPath,
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected rendered provider file %s: %v", rel, err)
		}
	}
}
