package controlplane

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ApprovalPayload struct {
	ApprovalID     string    `json:"approval_id"`
	VPSID          string    `json:"vps_id"`
	Server         string    `json:"server"`
	Tool           string    `json:"tool"`
	ArgumentDigest string    `json:"argument_digest"`
	ArgumentKeys   []string  `json:"argument_keys,omitempty"`
	UserID         string    `json:"user_id"`
	PolicyVersion  string    `json:"policy_version"`
	ExpiresAt      time.Time `json:"expires_at"`
}

func LoadOrCreateSigningKey(path string) ([]byte, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("api.signing_key_file is required")
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		return parseSigningKey(raw)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generate signing key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func LoadSigningKey(path string) ([]byte, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("api.signing_key_file is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseSigningKey(raw)
}

func SignApproval(key []byte, payload ApprovalPayload) (string, error) {
	if len(key) < 32 {
		return "", errors.New("approval signing key must be at least 32 bytes")
	}
	if err := payload.Validate(time.Time{}); err != nil {
		return "", err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(raw)
	signature := approvalSignature(key, encodedPayload)
	return encodedPayload + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func VerifyApprovalToken(key []byte, token string, now time.Time) (ApprovalPayload, error) {
	if len(key) < 32 {
		return ApprovalPayload{}, errors.New("approval signing key must be at least 32 bytes")
	}
	token = strings.TrimSpace(token)
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ApprovalPayload{}, errors.New("invalid approval token format")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ApprovalPayload{}, errors.New("invalid approval token signature encoding")
	}
	expected := approvalSignature(key, parts[0])
	if !hmac.Equal(signature, expected) {
		return ApprovalPayload{}, errors.New("invalid approval token signature")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return ApprovalPayload{}, errors.New("invalid approval token payload encoding")
	}
	var payload ApprovalPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ApprovalPayload{}, err
	}
	if err := payload.Validate(now); err != nil {
		return ApprovalPayload{}, err
	}
	return payload, nil
}

func (p ApprovalPayload) Validate(now time.Time) error {
	if strings.TrimSpace(p.ApprovalID) == "" {
		return errors.New("approval_id is required")
	}
	if strings.TrimSpace(p.VPSID) == "" {
		return errors.New("vps_id is required")
	}
	if strings.TrimSpace(p.Server) == "" {
		return errors.New("server is required")
	}
	if strings.TrimSpace(p.Tool) == "" {
		return errors.New("tool is required")
	}
	if strings.TrimSpace(p.ArgumentDigest) == "" {
		return errors.New("argument_digest is required")
	}
	if strings.TrimSpace(p.UserID) == "" {
		return errors.New("user_id is required")
	}
	if strings.TrimSpace(p.PolicyVersion) == "" {
		return errors.New("policy_version is required")
	}
	if p.ExpiresAt.IsZero() {
		return errors.New("expires_at is required")
	}
	if !now.IsZero() && !now.Before(p.ExpiresAt) {
		return fmt.Errorf("approval token expired at %s", p.ExpiresAt.Format(time.RFC3339Nano))
	}
	return nil
}

func parseSigningKey(raw []byte) ([]byte, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil, errors.New("approval signing key file is empty")
	}
	if decoded, err := hex.DecodeString(text); err == nil {
		if len(decoded) < 32 {
			return nil, errors.New("approval signing key must be at least 32 bytes")
		}
		return decoded, nil
	}
	if len([]byte(text)) < 32 {
		return nil, errors.New("approval signing key must be at least 32 bytes")
	}
	return []byte(text), nil
}

func approvalSignature(key []byte, encodedPayload string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(encodedPayload))
	return mac.Sum(nil)
}
