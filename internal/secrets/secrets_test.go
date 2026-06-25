package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProviderEnvFileFiltersAndRendersWithoutManifestValues(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "provider.env")
	if err := os.WriteFile(filePath, []byte(`
# local dev secrets
OPENAI_API_KEY=sk-test-secret
ANTHROPIC_API_KEY='anthropic-secret'
IGNORED_LOCAL_SECRET=do-not-inject
`), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	bundle, err := LoadProviderEnvFile(filePath, []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY"})
	if err != nil {
		t.Fatalf("LoadProviderEnvFile returned error: %v", err)
	}
	names := strings.Join(Names(bundle.Secrets), ",")
	if names != "ANTHROPIC_API_KEY,OPENAI_API_KEY" {
		t.Fatalf("unexpected secret names: %s", names)
	}
	envText := string(bundle.Files[ContainerSecretsEnvPath])
	if !strings.Contains(envText, "sk-test-secret") || !strings.Contains(envText, "anthropic-secret") {
		t.Fatalf("container env did not include expected secret values:\n%s", envText)
	}
	manifestText := string(bundle.Files[WorkspaceManifestPath])
	if strings.Contains(manifestText, "sk-test-secret") || strings.Contains(manifestText, "anthropic-secret") || strings.Contains(manifestText, "do-not-inject") {
		t.Fatalf("manifest leaked secret values:\n%s", manifestText)
	}
	if strings.Contains(envText, "IGNORED_LOCAL_SECRET") {
		t.Fatalf("env rendered non-provider secret:\n%s", envText)
	}
}

func TestLoadProviderEnvFileRejectsNoMatchingProviderSecrets(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "provider.env")
	if err := os.WriteFile(filePath, []byte("UNRELATED_SECRET=value\n"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	_, err := LoadProviderEnvFile(filePath, []string{"OPENAI_API_KEY"})
	if err == nil {
		t.Fatal("expected no matching variables error")
	}
	if !strings.Contains(err.Error(), "no variables matching") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWriteManifestToDirWritesOnlyNames(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "provider.env")
	if err := os.WriteFile(filePath, []byte("OPENAI_API_KEY=sk-test-secret\n"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	bundle, err := LoadProviderEnvFile(filePath, []string{"OPENAI_API_KEY"})
	if err != nil {
		t.Fatalf("LoadProviderEnvFile returned error: %v", err)
	}
	workspace := filepath.Join(dir, "workspace")
	if err := WriteManifestToDir(bundle, workspace); err != nil {
		t.Fatalf("WriteManifestToDir returned error: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(WorkspaceManifestPath)))
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if strings.Contains(string(content), "sk-test-secret") || !strings.Contains(string(content), "OPENAI_API_KEY") {
		t.Fatalf("unexpected manifest content:\n%s", string(content))
	}
}
