# Stola Host OS Core v0 Threat Model

This threat model covers the Host OS core only.

## Actors

- Host admin: controls the Linux/Docker host and any future Proxmox host.
- Stola service: local systemd daemon running privileged host operations.
- Guest user: future user of a Stola VPS/container.
- Agent runtime: future AI process that can request tools.
- MCP server: future tool provider with filesystem, network, or API access.
- External attacker: unauthenticated network actor.

## Assets

- Host root privileges.
- Docker daemon access and any future Proxmox API token files.
- `/etc/stola/stola.yaml`.
- SQLite state in `/var/lib/stola/stola.db`.
- Audit log in `/var/log/stola/audit.jsonl`.
- Future guest filesystems, snapshots, and model gateway keys.

## Boundaries

- Host OS to Stola services.
- Stola services to Docker daemon/CLI.
- Stola services to future Proxmox API.
- Stola services to guest agent.
- Agent runtime to MCP tools.
- MCP tools to secrets and network.

## v0 Controls

- Proxmox token is referenced by file path, never embedded in YAML.
- Docker workspaces are created without privileged mode, without Docker socket
  mounts, with capability drop, process limits, memory/CPU limits, and
  `no-new-privileges`.
- Audit events redact token, secret, password, credential, and private key fields.
- Mock backend is default until real Proxmox behavior is implemented.
- systemd units restrict writable paths to Stola state/log/runtime locations.
- Frontend, marketplace, browser automation, and real MCP installation are out of v0.

## Primary Risks

- A host daemon bug can affect the whole node.
- A future MCP tool can become a supply-chain or confused-deputy path.
- A future agent with terminal access can execute unsafe operations.
- Logs can leak secrets if redaction is incomplete.
- Docker and LXC are not strong tenant boundaries against kernel vulnerabilities.

KVM and stronger sandboxing are required before high-risk tenants or untrusted
agent workloads are allowed.
