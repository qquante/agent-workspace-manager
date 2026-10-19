# ADR-007: Docker-Only MVP, No OS Or ISO

Status: accepted

Stola will not build an ISO, installer, custom OS, or Proxmox/Debian host
appliance for the MVP.

Context:

- The real product value is the AI workspace: Hermes, OpenCode, OpenClaw, MCPs,
  approvals, audit, model providers, and persistent files per user.
- Building an OS/ISO first slows feedback and forces infrastructure decisions
  before the agent runtime is proven.
- The current local path already works with Docker Desktop and isolated Docker
  workspaces.

Decision:

- The MVP runtime is Docker-only.
- The product unit is a Stola agent workspace backed by a container and a
  persistent `/workspace` volume.
- Proxmox/KVM is retained only as future stronger-isolation work for premium or
  high-risk tenants.
- Existing `vps` command names can remain temporarily as product shorthand, but
  implementation and docs must describe the active unit as a Docker workspace.

Consequences:

- No bare-metal server is required to validate the MVP.
- No public service is exposed by default; local control-plane UX stays on
  `127.0.0.1` during development.
- Docker is not a strong tenant isolation boundary. Production multi-tenant
  workloads still need gVisor/Kata/KVM/Firecracker-style isolation before
  untrusted workloads are allowed.
- Proxmox docs/tasks stay in the backlog as future backend research, not as an
  MVP gate.

