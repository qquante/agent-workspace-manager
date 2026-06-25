package state

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	MCPApprovalStatusPending = "pending"
	MCPApprovalStatusUsed    = "used"
)

type Store struct {
	db *sql.DB
}

type VPS struct {
	ID         string
	Name       string
	Plan       string
	Status     string
	Backend    string
	CreatedAt  time.Time
	ExternalID string
	TaskID     string
}

type Heartbeat struct {
	Service  string
	LastSeen time.Time
	Status   string
}

type MCPApproval struct {
	ID             string
	VPSID          string
	Server         string
	Tool           string
	ArgumentDigest string
	ArgumentKeys   []string
	Status         string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	UsedAt         time.Time
	Reason         string
	Rule           string
	ApprovedBy     string
	PolicyVersion  string
}

type MCPRateLimit struct {
	Key         string
	WindowStart time.Time
	ResetAt     time.Time
	Count       int
	Limit       int
	Remaining   int
}

type MCPRateLimitExceededError struct {
	Key     string
	Limit   int
	ResetAt time.Time
}

type MCPApprovalFilter struct {
	Status string
	Limit  int
}

type UsageEvent struct {
	ID           string
	KeyID        string
	WorkspaceID  string
	UserID       string
	ProviderID   string
	Model        string
	Operation    string
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	CostUSD      float64
	LatencyMS    int
	Status       string
	Error        string
	CreatedAt    time.Time
}

type UsageFilter struct {
	KeyID       string
	WorkspaceID string
	UserID      string
	ProviderID  string
	Limit       int
}

type UsageSummary struct {
	Events       int
	InputTokens  int
	OutputTokens int
	TotalTokens  int
	CostUSD      float64
	Errors       int
}

type LLMProxyKey struct {
	ID             string
	KeyHash        string
	WorkspaceID    string
	UserID         string
	ProviderID     string
	Models         []string
	BudgetUSD      float64
	RPM            int
	TPM            int
	MaxConcurrency int
	Status         string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	LastUsedAt     time.Time
}

type LLMProxyKeyFilter struct {
	WorkspaceID string
	UserID      string
	ProviderID  string
	Status      string
	Limit       int
}

func (e MCPRateLimitExceededError) Error() string {
	return fmt.Sprintf("mcp rate limit exceeded for %s: limit=%d reset_at=%s", e.Key, e.Limit, e.ResetAt.Format(time.RFC3339Nano))
}

func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	store := &Store{db: db}
	if err := store.configure(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := store.Migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) configure() error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}
	for _, stmt := range pragmas {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS vps (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			plan TEXT NOT NULL,
			status TEXT NOT NULL,
			backend TEXT NOT NULL,
			created_at TEXT NOT NULL,
			external_id TEXT NOT NULL DEFAULT '',
			task_id TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS heartbeats (
			service TEXT PRIMARY KEY,
			last_seen TEXT NOT NULL,
			status TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS mcp_approvals (
			id TEXT PRIMARY KEY,
			vps_id TEXT NOT NULL,
			server TEXT NOT NULL,
			tool TEXT NOT NULL,
			argument_digest TEXT NOT NULL,
			argument_keys TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			used_at TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			rule TEXT NOT NULL DEFAULT '',
			approved_by TEXT NOT NULL DEFAULT '',
			policy_version TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_mcp_approvals_lookup
			ON mcp_approvals (vps_id, server, tool, status, expires_at)`,
		`CREATE TABLE IF NOT EXISTS mcp_rate_limits (
			key TEXT PRIMARY KEY,
			window_start TEXT NOT NULL,
			count INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS usage_events (
			id TEXT PRIMARY KEY,
			key_id TEXT NOT NULL DEFAULT '',
			workspace_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			model TEXT NOT NULL,
			operation TEXT NOT NULL,
			input_tokens INTEGER NOT NULL,
			output_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL,
			cost_usd REAL NOT NULL,
			latency_ms INTEGER NOT NULL,
			status TEXT NOT NULL,
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_workspace_created
			ON usage_events (workspace_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_key_created
			ON usage_events (key_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_user_provider_created
			ON usage_events (user_id, provider_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS llm_proxy_keys (
			id TEXT PRIMARY KEY,
			key_hash TEXT NOT NULL UNIQUE,
			workspace_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			provider_id TEXT NOT NULL,
			models TEXT NOT NULL,
			budget_usd REAL NOT NULL,
			rpm INTEGER NOT NULL,
			tpm INTEGER NOT NULL DEFAULT 0,
			max_concurrency INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL DEFAULT '',
			last_used_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_llm_proxy_keys_workspace
			ON llm_proxy_keys (workspace_id, status, created_at DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	for _, stmt := range []string{
		`ALTER TABLE vps ADD COLUMN external_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE vps ADD COLUMN task_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE mcp_approvals ADD COLUMN approved_by TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE mcp_approvals ADD COLUMN policy_version TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE usage_events ADD COLUMN key_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE llm_proxy_keys ADD COLUMN tpm INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE llm_proxy_keys ADD COLUMN max_concurrency INTEGER NOT NULL DEFAULT 0`,
	} {
		_, _ = s.db.ExecContext(ctx, stmt)
	}
	return nil
}

func GenerateLLMProxyKey() (string, error) {
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate llm proxy key: %w", err)
	}
	return "sk-stolas-" + hex.EncodeToString(random), nil
}

func HashLLMProxyKey(raw string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(raw)))
	return hex.EncodeToString(sum[:])
}

func (s *Store) CreateLLMProxyKey(ctx context.Context, key LLMProxyKey) (LLMProxyKey, error) {
	var err error
	if key.ID == "" {
		key.ID, err = generateLLMProxyKeyID()
		if err != nil {
			return LLMProxyKey{}, err
		}
	}
	key.KeyHash = strings.TrimSpace(key.KeyHash)
	key.WorkspaceID = strings.TrimSpace(key.WorkspaceID)
	key.UserID = strings.TrimSpace(key.UserID)
	key.ProviderID = strings.TrimSpace(key.ProviderID)
	key.Status = strings.TrimSpace(key.Status)
	if key.KeyHash == "" {
		return LLMProxyKey{}, errors.New("llm proxy key hash is required")
	}
	if key.WorkspaceID == "" {
		return LLMProxyKey{}, errors.New("llm proxy workspace id is required")
	}
	if key.UserID == "" {
		return LLMProxyKey{}, errors.New("llm proxy user id is required")
	}
	if key.ProviderID == "" {
		return LLMProxyKey{}, errors.New("llm proxy provider id is required")
	}
	if key.BudgetUSD < 0 || key.RPM < 0 || key.TPM < 0 || key.MaxConcurrency < 0 {
		return LLMProxyKey{}, errors.New("llm proxy budget, rpm, tpm, and max concurrency must not be negative")
	}
	if key.Status == "" {
		key.Status = "active"
	}
	if key.Status != "active" && key.Status != "revoked" {
		return LLMProxyKey{}, errors.New("llm proxy key status must be active or revoked")
	}
	if key.CreatedAt.IsZero() {
		key.CreatedAt = time.Now().UTC()
	}
	key.CreatedAt = key.CreatedAt.UTC()
	key.Models = normalizeStringList(key.Models)
	modelsJSON, err := json.Marshal(key.Models)
	if err != nil {
		return LLMProxyKey{}, err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO llm_proxy_keys
		 (id, key_hash, workspace_id, user_id, provider_id, models, budget_usd, rpm, tpm, max_concurrency, status, created_at, expires_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		key.ID,
		key.KeyHash,
		key.WorkspaceID,
		key.UserID,
		key.ProviderID,
		string(modelsJSON),
		key.BudgetUSD,
		key.RPM,
		key.TPM,
		key.MaxConcurrency,
		key.Status,
		key.CreatedAt.Format(time.RFC3339Nano),
		formatOptionalTime(key.ExpiresAt),
		formatOptionalTime(key.LastUsedAt),
	)
	if err != nil {
		return LLMProxyKey{}, err
	}
	return key, nil
}

func (s *Store) RevokeLLMProxyKey(ctx context.Context, id string) (LLMProxyKey, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return LLMProxyKey{}, errors.New("llm proxy key id is required")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE llm_proxy_keys SET status = 'revoked' WHERE id = ?`, id)
	if err != nil {
		return LLMProxyKey{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return LLMProxyKey{}, err
	}
	if affected == 0 {
		return LLMProxyKey{}, sql.ErrNoRows
	}
	return s.GetLLMProxyKey(ctx, id)
}

func (s *Store) RotateLLMProxyKey(ctx context.Context, id string, newHash string, _ time.Time) (LLMProxyKey, error) {
	id = strings.TrimSpace(id)
	newHash = strings.TrimSpace(newHash)
	if id == "" {
		return LLMProxyKey{}, errors.New("llm proxy key id is required")
	}
	if newHash == "" {
		return LLMProxyKey{}, errors.New("llm proxy new key hash is required")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE llm_proxy_keys SET key_hash = ?, status = 'active', last_used_at = '' WHERE id = ?`, newHash, id)
	if err != nil {
		return LLMProxyKey{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return LLMProxyKey{}, err
	}
	if affected == 0 {
		return LLMProxyKey{}, sql.ErrNoRows
	}
	return s.GetLLMProxyKey(ctx, id)
}

func (s *Store) GetLLMProxyKey(ctx context.Context, id string) (LLMProxyKey, error) {
	if strings.TrimSpace(id) == "" {
		return LLMProxyKey{}, errors.New("llm proxy key id is required")
	}
	return scanLLMProxyKey(s.db.QueryRowContext(ctx, llmProxyKeySelectSQL()+` WHERE id = ?`, id))
}

func (s *Store) FindLLMProxyKeyByHash(ctx context.Context, hash string) (LLMProxyKey, error) {
	if strings.TrimSpace(hash) == "" {
		return LLMProxyKey{}, errors.New("llm proxy key hash is required")
	}
	return scanLLMProxyKey(s.db.QueryRowContext(ctx, llmProxyKeySelectSQL()+` WHERE key_hash = ?`, strings.TrimSpace(hash)))
}

func (s *Store) ListLLMProxyKeys(ctx context.Context, filter LLMProxyKeyFilter) ([]LLMProxyKey, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	where, args := llmProxyKeyWhereClause(filter)
	query := llmProxyKeySelectSQL() + where + ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LLMProxyKey
	for rows.Next() {
		key, err := scanLLMProxyKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

func (s *Store) TouchLLMProxyKeyUsedAt(ctx context.Context, id string, at time.Time) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("llm proxy key id is required")
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	result, err := s.db.ExecContext(ctx, `UPDATE llm_proxy_keys SET last_used_at = ? WHERE id = ?`, at.UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) RecordUsage(ctx context.Context, event UsageEvent) (UsageEvent, error) {
	var err error
	if event.ID == "" {
		event.ID, err = generateUsageEventID()
		if err != nil {
			return UsageEvent{}, err
		}
	}
	event.KeyID = strings.TrimSpace(event.KeyID)
	event.WorkspaceID = strings.TrimSpace(event.WorkspaceID)
	event.UserID = strings.TrimSpace(event.UserID)
	event.ProviderID = strings.TrimSpace(event.ProviderID)
	event.Model = strings.TrimSpace(event.Model)
	event.Operation = strings.TrimSpace(event.Operation)
	event.Status = strings.TrimSpace(event.Status)
	if event.WorkspaceID == "" {
		return UsageEvent{}, errors.New("usage workspace id is required")
	}
	if event.UserID == "" {
		return UsageEvent{}, errors.New("usage user id is required")
	}
	if event.ProviderID == "" {
		return UsageEvent{}, errors.New("usage provider id is required")
	}
	if event.Model == "" {
		return UsageEvent{}, errors.New("usage model is required")
	}
	if event.Operation == "" {
		event.Operation = "chat_completion"
	}
	if event.Status == "" {
		event.Status = "success"
	}
	if event.InputTokens < 0 || event.OutputTokens < 0 || event.TotalTokens < 0 || event.CostUSD < 0 || event.LatencyMS < 0 {
		return UsageEvent{}, errors.New("usage counters must not be negative")
	}
	if event.TotalTokens == 0 {
		event.TotalTokens = event.InputTokens + event.OutputTokens
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	event.CreatedAt = event.CreatedAt.UTC()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO usage_events
		 (id, key_id, workspace_id, user_id, provider_id, model, operation, input_tokens, output_tokens, total_tokens, cost_usd, latency_ms, status, error, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID,
		event.KeyID,
		event.WorkspaceID,
		event.UserID,
		event.ProviderID,
		event.Model,
		event.Operation,
		event.InputTokens,
		event.OutputTokens,
		event.TotalTokens,
		event.CostUSD,
		event.LatencyMS,
		event.Status,
		event.Error,
		event.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return UsageEvent{}, err
	}
	return event, nil
}

func (s *Store) ListUsage(ctx context.Context, filter UsageFilter) ([]UsageEvent, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	where, args := usageWhereClause(filter)
	query := usageSelectSQL() + where + ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UsageEvent
	for rows.Next() {
		event, err := scanUsageEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *Store) SummarizeUsage(ctx context.Context, filter UsageFilter) (UsageSummary, error) {
	where, args := usageWhereClause(filter)
	row := s.db.QueryRowContext(ctx,
		`SELECT
		 COUNT(*),
		 COALESCE(SUM(input_tokens), 0),
		 COALESCE(SUM(output_tokens), 0),
		 COALESCE(SUM(total_tokens), 0),
		 COALESCE(SUM(cost_usd), 0),
		 COALESCE(SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END), 0)
		 FROM usage_events`+where,
		args...,
	)
	var summary UsageSummary
	if err := row.Scan(&summary.Events, &summary.InputTokens, &summary.OutputTokens, &summary.TotalTokens, &summary.CostUSD, &summary.Errors); err != nil {
		return UsageSummary{}, err
	}
	return summary, nil
}

func (s *Store) CreateVPS(ctx context.Context, v VPS) error {
	if v.ID == "" {
		return errors.New("vps id is required")
	}
	if v.Name == "" {
		return errors.New("vps name is required")
	}
	if v.Plan == "" {
		return errors.New("vps plan is required")
	}
	if v.Status == "" {
		v.Status = "created"
	}
	if v.Backend == "" {
		v.Backend = "mock"
	}
	if v.CreatedAt.IsZero() {
		v.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO vps (id, name, plan, status, backend, created_at, external_id, task_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, v.Name, v.Plan, v.Status, v.Backend, v.CreatedAt.Format(time.RFC3339Nano), v.ExternalID, v.TaskID,
	)
	return err
}

func (s *Store) ListVPS(ctx context.Context) ([]VPS, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, plan, status, backend, created_at, external_id, task_id FROM vps ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []VPS
	for rows.Next() {
		var v VPS
		var created string
		if err := rows.Scan(&v.ID, &v.Name, &v.Plan, &v.Status, &v.Backend, &created, &v.ExternalID, &v.TaskID); err != nil {
			return nil, err
		}
		if parsed, err := time.Parse(time.RFC3339Nano, created); err == nil {
			v.CreatedAt = parsed
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) GetVPS(ctx context.Context, id string) (VPS, error) {
	if id == "" {
		return VPS{}, errors.New("vps id is required")
	}
	var v VPS
	var created string
	err := s.db.QueryRowContext(ctx, `SELECT id, name, plan, status, backend, created_at, external_id, task_id FROM vps WHERE id = ?`, id).
		Scan(&v.ID, &v.Name, &v.Plan, &v.Status, &v.Backend, &created, &v.ExternalID, &v.TaskID)
	if err != nil {
		return VPS{}, err
	}
	if parsed, err := time.Parse(time.RFC3339Nano, created); err == nil {
		v.CreatedAt = parsed
	}
	return v, nil
}

func (s *Store) UpdateVPSStatus(ctx context.Context, id string, status string) error {
	if id == "" {
		return errors.New("vps id is required")
	}
	if status == "" {
		return errors.New("vps status is required")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE vps SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteVPS(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("vps id is required")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM vps WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) CreateMCPApproval(ctx context.Context, approval MCPApproval) (MCPApproval, error) {
	var err error
	if approval.ID == "" {
		approval.ID, err = generateMCPApprovalID()
		if err != nil {
			return MCPApproval{}, err
		}
	}
	if approval.VPSID == "" {
		return MCPApproval{}, errors.New("approval vps id is required")
	}
	if approval.Server == "" {
		return MCPApproval{}, errors.New("approval server is required")
	}
	if approval.Tool == "" {
		return MCPApproval{}, errors.New("approval tool is required")
	}
	if approval.ArgumentDigest == "" {
		return MCPApproval{}, errors.New("approval argument digest is required")
	}
	if approval.CreatedAt.IsZero() {
		approval.CreatedAt = time.Now().UTC()
	}
	if approval.ExpiresAt.IsZero() {
		approval.ExpiresAt = approval.CreatedAt.Add(10 * time.Minute)
	}
	if !approval.ExpiresAt.After(approval.CreatedAt) {
		return MCPApproval{}, errors.New("approval expires_at must be after created_at")
	}
	if approval.Status == "" {
		approval.Status = MCPApprovalStatusPending
	}
	if approval.Status != MCPApprovalStatusPending {
		return MCPApproval{}, errors.New("new approval status must be pending")
	}
	approval.ArgumentKeys = normalizeMCPApprovalKeys(approval.ArgumentKeys)
	keysJSON, err := encodeMCPApprovalKeys(approval.ArgumentKeys)
	if err != nil {
		return MCPApproval{}, err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO mcp_approvals
		 (id, vps_id, server, tool, argument_digest, argument_keys, status, created_at, expires_at, used_at, reason, rule, approved_by, policy_version)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		approval.ID,
		approval.VPSID,
		approval.Server,
		approval.Tool,
		approval.ArgumentDigest,
		keysJSON,
		approval.Status,
		approval.CreatedAt.Format(time.RFC3339Nano),
		approval.ExpiresAt.Format(time.RFC3339Nano),
		"",
		approval.Reason,
		approval.Rule,
		approval.ApprovedBy,
		approval.PolicyVersion,
	)
	if err != nil {
		return MCPApproval{}, err
	}
	return approval, nil
}

func (s *Store) GetMCPApproval(ctx context.Context, id string) (MCPApproval, error) {
	if strings.TrimSpace(id) == "" {
		return MCPApproval{}, errors.New("approval id is required")
	}
	return scanMCPApproval(s.db.QueryRowContext(ctx, mcpApprovalSelectSQL()+` WHERE id = ?`, id))
}

func (s *Store) ListMCPApprovals(ctx context.Context, filter MCPApprovalFilter) ([]MCPApproval, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	status := strings.TrimSpace(filter.Status)
	query := mcpApprovalSelectSQL()
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MCPApproval
	for rows.Next() {
		approval, err := scanMCPApproval(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, approval)
	}
	return out, rows.Err()
}

func (s *Store) ApproveMCPApproval(ctx context.Context, id string, approvedBy string, policyVersion string, now time.Time) (MCPApproval, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return MCPApproval{}, errors.New("approval id is required")
	}
	approvedBy = strings.TrimSpace(approvedBy)
	if approvedBy == "" {
		return MCPApproval{}, errors.New("approved_by is required")
	}
	policyVersion = strings.TrimSpace(policyVersion)
	if policyVersion == "" {
		return MCPApproval{}, errors.New("policy_version is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MCPApproval{}, err
	}
	defer tx.Rollback()

	approval, err := scanMCPApproval(tx.QueryRowContext(ctx, mcpApprovalSelectSQL()+` WHERE id = ?`, id))
	if err != nil {
		return MCPApproval{}, err
	}
	if approval.Status != MCPApprovalStatusPending {
		return MCPApproval{}, fmt.Errorf("approval %s is already used", id)
	}
	if !now.Before(approval.ExpiresAt) {
		return MCPApproval{}, fmt.Errorf("approval %s expired at %s", id, approval.ExpiresAt.Format(time.RFC3339Nano))
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE mcp_approvals SET approved_by = ?, policy_version = ? WHERE id = ? AND status = ?`,
		approvedBy,
		policyVersion,
		id,
		MCPApprovalStatusPending,
	); err != nil {
		return MCPApproval{}, err
	}
	if err := tx.Commit(); err != nil {
		return MCPApproval{}, err
	}
	approval.ApprovedBy = approvedBy
	approval.PolicyVersion = policyVersion
	return approval, nil
}

func (s *Store) ConsumeMCPApproval(ctx context.Context, id string, vpsID string, server string, tool string, argumentDigest string, now time.Time) (MCPApproval, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return MCPApproval{}, errors.New("approval id is required")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MCPApproval{}, err
	}
	defer tx.Rollback()

	approval, err := scanMCPApproval(tx.QueryRowContext(ctx, mcpApprovalSelectSQL()+` WHERE id = ?`, id))
	if err != nil {
		return MCPApproval{}, err
	}
	if approval.Status != MCPApprovalStatusPending {
		return MCPApproval{}, fmt.Errorf("approval %s is already used", id)
	}
	if !now.Before(approval.ExpiresAt) {
		return MCPApproval{}, fmt.Errorf("approval %s expired at %s", id, approval.ExpiresAt.Format(time.RFC3339Nano))
	}
	if approval.VPSID != vpsID || approval.Server != server || approval.Tool != tool || approval.ArgumentDigest != argumentDigest {
		return MCPApproval{}, fmt.Errorf("approval %s does not match request", id)
	}

	usedAt := now.UTC()
	result, err := tx.ExecContext(ctx,
		`UPDATE mcp_approvals SET status = ?, used_at = ? WHERE id = ? AND status = ?`,
		MCPApprovalStatusUsed,
		usedAt.Format(time.RFC3339Nano),
		id,
		MCPApprovalStatusPending,
	)
	if err != nil {
		return MCPApproval{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return MCPApproval{}, err
	}
	if affected == 0 {
		return MCPApproval{}, fmt.Errorf("approval %s is already used", id)
	}
	if err := tx.Commit(); err != nil {
		return MCPApproval{}, err
	}
	approval.Status = MCPApprovalStatusUsed
	approval.UsedAt = usedAt
	return approval, nil
}

func (s *Store) ConsumeMCPRateLimit(ctx context.Context, key string, limit int, window time.Duration, now time.Time) (MCPRateLimit, error) {
	return s.ConsumeRateLimitAmount(ctx, key, 1, limit, window, now)
}

func (s *Store) ConsumeRateLimitAmount(ctx context.Context, key string, amount int, limit int, window time.Duration, now time.Time) (MCPRateLimit, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return MCPRateLimit{}, errors.New("rate limit key is required")
	}
	if amount <= 0 || limit <= 0 || window <= 0 {
		return MCPRateLimit{}, errors.New("rate limit requires positive amount, limit, and window")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MCPRateLimit{}, err
	}
	defer tx.Rollback()

	var windowStartRaw string
	var count int
	err = tx.QueryRowContext(ctx, `SELECT window_start, count FROM mcp_rate_limits WHERE key = ?`, key).Scan(&windowStartRaw, &count)
	if errors.Is(err, sql.ErrNoRows) {
		windowStart := now
		if amount > limit {
			return MCPRateLimit{}, MCPRateLimitExceededError{
				Key:     key,
				Limit:   limit,
				ResetAt: windowStart.Add(window),
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO mcp_rate_limits (key, window_start, count) VALUES (?, ?, ?)`, key, windowStart.Format(time.RFC3339Nano), amount); err != nil {
			return MCPRateLimit{}, err
		}
		if err := tx.Commit(); err != nil {
			return MCPRateLimit{}, err
		}
		return MCPRateLimit{
			Key:         key,
			WindowStart: windowStart,
			ResetAt:     windowStart.Add(window),
			Count:       amount,
			Limit:       limit,
			Remaining:   limit - amount,
		}, nil
	}
	if err != nil {
		return MCPRateLimit{}, err
	}
	windowStart, err := time.Parse(time.RFC3339Nano, windowStartRaw)
	if err != nil {
		return MCPRateLimit{}, err
	}
	resetAt := windowStart.Add(window)
	if !now.Before(resetAt) {
		windowStart = now
		count = amount
		if amount > limit {
			return MCPRateLimit{}, MCPRateLimitExceededError{
				Key:     key,
				Limit:   limit,
				ResetAt: windowStart.Add(window),
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE mcp_rate_limits SET window_start = ?, count = ? WHERE key = ?`, windowStart.Format(time.RFC3339Nano), count, key); err != nil {
			return MCPRateLimit{}, err
		}
		if err := tx.Commit(); err != nil {
			return MCPRateLimit{}, err
		}
		return MCPRateLimit{
			Key:         key,
			WindowStart: windowStart,
			ResetAt:     windowStart.Add(window),
			Count:       count,
			Limit:       limit,
			Remaining:   limit - count,
		}, nil
	}
	if count+amount > limit {
		return MCPRateLimit{}, MCPRateLimitExceededError{
			Key:     key,
			Limit:   limit,
			ResetAt: resetAt,
		}
	}
	count += amount
	if _, err := tx.ExecContext(ctx, `UPDATE mcp_rate_limits SET count = ? WHERE key = ?`, count, key); err != nil {
		return MCPRateLimit{}, err
	}
	if err := tx.Commit(); err != nil {
		return MCPRateLimit{}, err
	}
	return MCPRateLimit{
		Key:         key,
		WindowStart: windowStart,
		ResetAt:     resetAt,
		Count:       count,
		Limit:       limit,
		Remaining:   limit - count,
	}, nil
}

func (s *Store) SetHeartbeat(ctx context.Context, service string, status string, at time.Time) error {
	if service == "" {
		return errors.New("service is required")
	}
	if status == "" {
		status = "ok"
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO heartbeats (service, last_seen, status) VALUES (?, ?, ?)
		 ON CONFLICT(service) DO UPDATE SET last_seen=excluded.last_seen, status=excluded.status`,
		service, at.Format(time.RFC3339Nano), status,
	)
	return err
}

func (s *Store) GetHeartbeat(ctx context.Context, service string) (Heartbeat, error) {
	var hb Heartbeat
	var lastSeen string
	err := s.db.QueryRowContext(ctx, `SELECT service, last_seen, status FROM heartbeats WHERE service = ?`, service).
		Scan(&hb.Service, &lastSeen, &hb.Status)
	if err != nil {
		return Heartbeat{}, err
	}
	hb.LastSeen, _ = time.Parse(time.RFC3339Nano, lastSeen)
	return hb, nil
}

func generateMCPApprovalID() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate approval id: %w", err)
	}
	return "mcpappr-" + hex.EncodeToString(random), nil
}

func generateUsageEventID() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate usage event id: %w", err)
	}
	return "usage-" + hex.EncodeToString(random), nil
}

func usageSelectSQL() string {
	return `SELECT id, key_id, workspace_id, user_id, provider_id, model, operation, input_tokens, output_tokens, total_tokens, cost_usd, latency_ms, status, error, created_at FROM usage_events`
}

func usageWhereClause(filter UsageFilter) (string, []any) {
	var clauses []string
	var args []any
	if value := strings.TrimSpace(filter.KeyID); value != "" {
		clauses = append(clauses, "key_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.WorkspaceID); value != "" {
		clauses = append(clauses, "workspace_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.UserID); value != "" {
		clauses = append(clauses, "user_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.ProviderID); value != "" {
		clauses = append(clauses, "provider_id = ?")
		args = append(args, value)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func llmProxyKeySelectSQL() string {
	return `SELECT id, key_hash, workspace_id, user_id, provider_id, models, budget_usd, rpm, tpm, max_concurrency, status, created_at, expires_at, last_used_at FROM llm_proxy_keys`
}

func llmProxyKeyWhereClause(filter LLMProxyKeyFilter) (string, []any) {
	var clauses []string
	var args []any
	if value := strings.TrimSpace(filter.WorkspaceID); value != "" {
		clauses = append(clauses, "workspace_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.UserID); value != "" {
		clauses = append(clauses, "user_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.ProviderID); value != "" {
		clauses = append(clauses, "provider_id = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(filter.Status); value != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, value)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

type usageEventScanner interface {
	Scan(dest ...any) error
}

func scanUsageEvent(row usageEventScanner) (UsageEvent, error) {
	var event UsageEvent
	var createdAt string
	err := row.Scan(
		&event.ID,
		&event.KeyID,
		&event.WorkspaceID,
		&event.UserID,
		&event.ProviderID,
		&event.Model,
		&event.Operation,
		&event.InputTokens,
		&event.OutputTokens,
		&event.TotalTokens,
		&event.CostUSD,
		&event.LatencyMS,
		&event.Status,
		&event.Error,
		&createdAt,
	)
	if err != nil {
		return UsageEvent{}, err
	}
	event.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return event, nil
}

type llmProxyKeyScanner interface {
	Scan(dest ...any) error
}

func scanLLMProxyKey(row llmProxyKeyScanner) (LLMProxyKey, error) {
	var key LLMProxyKey
	var modelsJSON string
	var createdAt string
	var expiresAt string
	var lastUsedAt string
	err := row.Scan(
		&key.ID,
		&key.KeyHash,
		&key.WorkspaceID,
		&key.UserID,
		&key.ProviderID,
		&modelsJSON,
		&key.BudgetUSD,
		&key.RPM,
		&key.TPM,
		&key.MaxConcurrency,
		&key.Status,
		&createdAt,
		&expiresAt,
		&lastUsedAt,
	)
	if err != nil {
		return LLMProxyKey{}, err
	}
	_ = json.Unmarshal([]byte(modelsJSON), &key.Models)
	key.Models = normalizeStringList(key.Models)
	key.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	key.ExpiresAt = parseOptionalTime(expiresAt)
	key.LastUsedAt = parseOptionalTime(lastUsedAt)
	return key, nil
}

func mcpApprovalSelectSQL() string {
	return `SELECT id, vps_id, server, tool, argument_digest, argument_keys, status, created_at, expires_at, used_at, reason, rule, approved_by, policy_version FROM mcp_approvals`
}

func generateLLMProxyKeyID() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate llm proxy key id: %w", err)
	}
	return "llmkey-" + hex.EncodeToString(random), nil
}

func normalizeStringList(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseOptionalTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, strings.TrimSpace(value))
	return parsed
}

type mcpApprovalScanner interface {
	Scan(dest ...any) error
}

func scanMCPApproval(row mcpApprovalScanner) (MCPApproval, error) {
	var approval MCPApproval
	var keysJSON string
	var createdAt string
	var expiresAt string
	var usedAt string
	err := row.Scan(
		&approval.ID,
		&approval.VPSID,
		&approval.Server,
		&approval.Tool,
		&approval.ArgumentDigest,
		&keysJSON,
		&approval.Status,
		&createdAt,
		&expiresAt,
		&usedAt,
		&approval.Reason,
		&approval.Rule,
		&approval.ApprovedBy,
		&approval.PolicyVersion,
	)
	if err != nil {
		return MCPApproval{}, err
	}
	keys, err := decodeMCPApprovalKeys(keysJSON)
	if err != nil {
		return MCPApproval{}, err
	}
	approval.ArgumentKeys = keys
	approval.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	approval.ExpiresAt, _ = time.Parse(time.RFC3339Nano, expiresAt)
	if usedAt != "" {
		approval.UsedAt, _ = time.Parse(time.RFC3339Nano, usedAt)
	}
	return approval, nil
}

func encodeMCPApprovalKeys(keys []string) (string, error) {
	raw, err := json.Marshal(normalizeMCPApprovalKeys(keys))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func decodeMCPApprovalKeys(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var keys []string
	if err := json.Unmarshal([]byte(raw), &keys); err != nil {
		return nil, err
	}
	return normalizeMCPApprovalKeys(keys), nil
}

func normalizeMCPApprovalKeys(keys []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
