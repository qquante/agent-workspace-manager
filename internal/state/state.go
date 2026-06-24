package state

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type VPS struct {
	ID        string
	Name      string
	Plan      string
	Status    string
	Backend   string
	CreatedAt time.Time
}

type Heartbeat struct {
	Service  string
	LastSeen time.Time
	Status   string
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
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS heartbeats (
			service TEXT PRIMARY KEY,
			last_seen TEXT NOT NULL,
			status TEXT NOT NULL
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
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
		`INSERT INTO vps (id, name, plan, status, backend, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		v.ID, v.Name, v.Plan, v.Status, v.Backend, v.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (s *Store) ListVPS(ctx context.Context) ([]VPS, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, plan, status, backend, created_at FROM vps ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []VPS
	for rows.Next() {
		var v VPS
		var created string
		if err := rows.Scan(&v.ID, &v.Name, &v.Plan, &v.Status, &v.Backend, &created); err != nil {
			return nil, err
		}
		if parsed, err := time.Parse(time.RFC3339Nano, created); err == nil {
			v.CreatedAt = parsed
		}
		out = append(out, v)
	}
	return out, rows.Err()
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
