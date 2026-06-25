package provisioner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"stola-os/internal/audit"
	"stola-os/internal/state"
)

func TestMockCreatePersistsVPSAndAudit(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "stola.db"))
	if err != nil {
		t.Fatalf("state.Open returned error: %v", err)
	}
	defer store.Close()

	auditPath := filepath.Join(dir, "audit.jsonl")
	service := Service{
		State: store,
		Audit: audit.New(auditPath),
		Back:  MockBackend{},
	}

	ctx := context.Background()
	vps, err := service.MockCreate(ctx, CreateSpec{Name: "nano-a", Plan: "nano"})
	if err != nil {
		t.Fatalf("MockCreate returned error: %v", err)
	}
	if vps.ID == "" || vps.Backend != "mock" {
		t.Fatalf("unexpected VPS: %+v", vps)
	}

	list, err := store.ListVPS(ctx)
	if err != nil {
		t.Fatalf("ListVPS returned error: %v", err)
	}
	if len(list) != 1 || list[0].ID != vps.ID {
		t.Fatalf("VPS not persisted: %+v", list)
	}

	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("audit log missing: %v", err)
	}
	if !strings.Contains(string(data), "provisioner.mock_create") {
		t.Fatalf("audit log missing mock create event: %s", string(data))
	}
}

func TestCreateAuditsSecretNamesWithoutValues(t *testing.T) {
	dir := t.TempDir()
	store, err := state.Open(filepath.Join(dir, "stola.db"))
	if err != nil {
		t.Fatalf("state.Open returned error: %v", err)
	}
	defer store.Close()

	auditPath := filepath.Join(dir, "audit.jsonl")
	service := Service{
		State: store,
		Audit: audit.New(auditPath),
		Back:  MockBackend{},
	}

	_, err = service.Create(context.Background(), CreateSpec{
		Name:          "secret-a",
		InjectSecrets: true,
		SecretNames:   []string{"OPENAI_API_KEY"},
		SecretsFile:   filepath.Join(dir, "provider.env"),
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("audit log missing: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "secrets.inject") || !strings.Contains(text, "OPENAI_API_KEY") {
		t.Fatalf("audit log missing secret injection metadata: %s", text)
	}
	if strings.Contains(text, "provider.env") || strings.Contains(text, "sk-test-secret") {
		t.Fatalf("audit log leaked secret path/value: %s", text)
	}
}
