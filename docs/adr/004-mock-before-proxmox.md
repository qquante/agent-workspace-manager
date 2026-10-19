# ADR-004: Mock Backend Before Live Proxmox Validation

Status: accepted

The v0 implementation starts with a mock provisioner backend, then adds a
Proxmox REST adapter behind the same interface before any live lab create.

Rationale:

- The repository currently has no Proxmox lab attached.
- Mock mode gives deterministic tests for config, state, audit, and daemon
  behavior before touching infrastructure.
- The first Proxmox implementation can be tested against the same provisioner
  interface with `httptest` before a real lab host is used.

Mock mode is not a product feature. It is a development and acceptance harness.
Live Proxmox validation remains a separate gate.
