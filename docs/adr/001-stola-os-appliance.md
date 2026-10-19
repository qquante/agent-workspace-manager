# ADR-001: Stola OS Is A Linux Appliance

Status: accepted, amended by ADR-006

Stola OS will not implement a custom kernel or Linux distribution from scratch.
The host OS starts as a Linux appliance with Stola services installed as
systemd-managed packages.

This keeps the project focused on its differentiator: AI-native VPS operation,
agent runtime governance, local state, auditability, and safe automation.

Consequences:

- Docker/containerd is the Docker-first MVP substrate per ADR-006.
- Proxmox VE/KVM remains a later stronger-isolation substrate.
- Stola owns the control plane, policy, audit, guest images, and CLI/API.
- A custom ISO can come after the bootstrap path is stable.
