package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordRedactsSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	logger := New(path)

	err := logger.Record(Event{
		Actor:   "test",
		Action:  "secret.test",
		Target:  "node",
		Outcome: "success",
		Details: map[string]any{
			"token":      "super-secret",
			"safe_value": "visible",
			"nested": map[string]any{
				"password": "hidden",
			},
		},
	})
	if err != nil {
		t.Fatalf("Record returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "super-secret") || strings.Contains(text, "hidden") {
		t.Fatalf("audit log leaked secret: %s", text)
	}
	if !strings.Contains(text, Redacted) {
		t.Fatalf("audit log did not contain redaction marker: %s", text)
	}
	if !strings.Contains(text, "visible") {
		t.Fatalf("audit log lost safe value: %s", text)
	}
}
