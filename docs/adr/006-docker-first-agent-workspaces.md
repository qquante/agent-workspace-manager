# ADR-006: Docker-First Agent Workspaces

Status: accepted

Stola will move the next implementation slice from Proxmox-first VPS creation to
Docker-first agent workspaces.

Context:

- The product goal is to give each user a safe AI workspace with Hermes,
  OpenCode, OpenClaw, MCPs, memory, cron, tools, and persistent files.
- A full Proxmox/ISO path is still useful, but it blocks product validation on
  infrastructure before the agent runtime is proven.
- Docker is available locally and gives a faster feedback loop for the core
  product: workspace lifecycle, limits, policy, audit, MCP governance, and model
  key provisioning.

Decision:

- `backend.type=docker` becomes the active MVP path.
- Proxmox remains a future/premium backend for stronger VM isolation.
- The user-facing unit is a Stola workspace or agent container, not a strong VPS.
- Docker workspaces must use explicit resource limits and conservative runtime
  security options from the start.

Initial controls:

- Per-workspace Docker network.
- Per-workspace Docker volume mounted at `/workspace`.
- Memory, CPU, and PID limits.
- `--cap-drop=ALL`.
- `--security-opt no-new-privileges:true`.
- No Docker socket mount.
- State/audit records through the existing provisioner interface.

Consequences:

- MVP difficulty drops because no Proxmox lab or ISO is required.
- Tenant isolation is weaker than KVM; this must be stated clearly.
- High-risk tenants, untrusted arbitrary workloads, Docker-in-Docker, browser
  automation, and privileged tools require gVisor/Kata/KVM or equivalent
  stronger isolation before production.
