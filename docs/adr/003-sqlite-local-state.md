# ADR-003: SQLite For Single-Node Host State

Status: accepted

The Host OS core v0 uses SQLite for local single-node state.

Rationale:

- The first target is a single Proxmox/Debian host.
- SQLite avoids running Postgres before the host control plane exists.
- State can be backed up with the host config and inspected locally.
- A later multi-node control plane can migrate to Postgres.

All operational events that matter for security remain in append-only JSONL
audit logs, not only in the SQLite database.
