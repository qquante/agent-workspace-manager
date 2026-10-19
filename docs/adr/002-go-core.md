# ADR-002: Go For Host OS Core

Status: accepted

The Stola Host OS core will be implemented in Go.

Rationale:

- Single static-ish binary distribution is practical for Debian appliances.
- Go is suitable for CLI tools, long-running daemons, and systemd integration.
- The runtime footprint is predictable for host services.
- Development velocity is better than Rust for this early OS-control layer.

Python may still be used for tooling or image build scripts, but not for the
core host daemons.
