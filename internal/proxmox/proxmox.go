package proxmox

import (
	"context"
	"errors"

	"stola-os/internal/config"
	"stola-os/internal/state"
)

var ErrNotImplemented = errors.New("proxmox backend is defined but not implemented in Host OS core v0")

type Client struct {
	Config config.ProxmoxConfig
}

func New(cfg config.ProxmoxConfig) Client {
	return Client{Config: cfg}
}

func (c Client) Type() string {
	return "proxmox"
}

func (c Client) Create(context.Context, any) (state.VPS, error) {
	return state.VPS{}, ErrNotImplemented
}

func (c Client) Validate() error {
	if c.Config.Endpoint == "" {
		return errors.New("proxmox endpoint is required")
	}
	if c.Config.Node == "" {
		return errors.New("proxmox node is required")
	}
	if c.Config.TokenFile == "" {
		return errors.New("proxmox token_file is required")
	}
	return nil
}
