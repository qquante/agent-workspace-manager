package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const Redacted = "[REDACTED]"

type Event struct {
	ID      string         `json:"id"`
	Time    time.Time      `json:"time"`
	Actor   string         `json:"actor"`
	Action  string         `json:"action"`
	Target  string         `json:"target"`
	Outcome string         `json:"outcome"`
	Details map[string]any `json:"details,omitempty"`
}

type Logger struct {
	path string
	mu   sync.Mutex
}

func New(path string) *Logger {
	return &Logger{path: path}
}

func (l *Logger) Path() string {
	return l.path
}

func (l *Logger) Record(event Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if event.ID == "" {
		event.ID = fmt.Sprintf("audit-%d", time.Now().UnixNano())
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	event.Details = RedactMap(event.Details)

	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(event)
}

func RedactMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		if sensitiveKey(key) {
			out[key] = Redacted
			continue
		}
		out[key] = redactValue(value)
	}
	return out
}

func redactValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return RedactMap(v)
	case map[string]string:
		out := make(map[string]any, len(v))
		for key, value := range v {
			if sensitiveKey(key) {
				out[key] = Redacted
			} else {
				out[key] = value
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = redactValue(item)
		}
		return out
	default:
		return value
	}
}

func sensitiveKey(key string) bool {
	k := strings.ToLower(key)
	for _, needle := range []string{"token", "secret", "password", "passwd", "api_key", "apikey", "credential", "private_key"} {
		if strings.Contains(k, needle) {
			return true
		}
	}
	return false
}
