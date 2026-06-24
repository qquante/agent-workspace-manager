package provisioner

import (
	"context"
	"fmt"
	"time"

	"stola-os/internal/audit"
	"stola-os/internal/state"
)

type CreateSpec struct {
	Name string
	Plan string
}

type Backend interface {
	Type() string
	Create(ctx context.Context, spec CreateSpec) (state.VPS, error)
}

type Service struct {
	State *state.Store
	Audit *audit.Logger
	Back  Backend
}

func (s Service) MockCreate(ctx context.Context, spec CreateSpec) (state.VPS, error) {
	if s.Back == nil {
		s.Back = MockBackend{}
	}
	vps, err := s.Back.Create(ctx, spec)
	if err != nil {
		return state.VPS{}, err
	}
	if err := s.State.CreateVPS(ctx, vps); err != nil {
		return state.VPS{}, err
	}
	if s.Audit != nil {
		_ = s.Audit.Record(audit.Event{
			Actor:   "stola-cli",
			Action:  "provisioner.mock_create",
			Target:  vps.ID,
			Outcome: "success",
			Details: map[string]any{
				"name":    vps.Name,
				"plan":    vps.Plan,
				"backend": vps.Backend,
			},
		})
	}
	return vps, nil
}

func RunHeartbeat(ctx context.Context, store *state.Store, service string, interval time.Duration) error {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if err := store.SetHeartbeat(ctx, service, "running", time.Now().UTC()); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := store.SetHeartbeat(ctx, service, "running", time.Now().UTC()); err != nil {
				return err
			}
		}
	}
}

type MockBackend struct{}

func (MockBackend) Type() string {
	return "mock"
}

func (MockBackend) Create(_ context.Context, spec CreateSpec) (state.VPS, error) {
	if spec.Name == "" {
		spec.Name = "stola-nano"
	}
	if spec.Plan == "" {
		spec.Plan = "nano"
	}
	now := time.Now().UTC()
	return state.VPS{
		ID:        fmt.Sprintf("mock-%d", now.UnixNano()),
		Name:      spec.Name,
		Plan:      spec.Plan,
		Status:    "created",
		Backend:   "mock",
		CreatedAt: now,
	}, nil
}
