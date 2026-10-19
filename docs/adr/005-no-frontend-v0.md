# ADR-005: No Frontend In Host OS Core v0

Status: accepted

The Host OS core v0 excludes dashboard/frontend work.

The first deliverable is the OS control layer:

- CLI
- config
- local state
- audit log
- daemons
- systemd units
- provisioner interface

The dashboard should consume stable APIs after the host layer proves itself.
