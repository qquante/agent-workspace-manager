# Docker Agent Workspaces

The active Stola MVP runtime is a Docker-backed agent workspace.

```text
Linux host
  -> Docker or containerd
  -> stola CLI/API/daemon
  -> one workspace per user
     -> persistent /workspace volume
     -> isolated Docker network
     -> Hermes / OpenCode / OpenClaw
     -> approved MCP servers
     -> LiteLLM virtual key
     -> audit and policy controls
```

## Boundary

A Docker workspace is not a strong VPS. It shares the host kernel and must be
treated as a lower-isolation runtime than KVM.

The product can expose two runtime classes later:

- Container workspace: fast, dense, cheap, good for trusted or medium-risk agent
  workloads.
- VM workspace: stronger isolation for sensitive users, arbitrary code, browser
  automation, Docker-in-Docker, or higher-risk MCPs.

## MVP Requirements

- `stola docker check`.
- `stola vps create` with `backend.type=docker`.
- `stola-agent-base:dev` with Hermes, OpenCode, OpenClaw, and `uvx`.
- Default MCP manifest at `/opt/stola/manifests/mcp/default.yaml`.
- Per-workspace network and volume.
- Memory, CPU, swap, and PID limits.
- No privileged containers.
- No Docker socket mount.
- Capability drop and `no-new-privileges`.
- Audit record for every workspace create.
- `stola vps list`, `status`, `logs`, `exec`, `stop`, `start`, and `delete`.

## Current Image

`images/stola-agent-base/Dockerfile` builds the current agent workspace image.
It runs as UID `10001`, uses `/workspace` as the working directory, and copies
`manifests/mcp/default.yaml` into `/opt/stola/manifests/mcp/default.yaml`.

The first manifest enables only local workspace primitives by default:
filesystem, git, fetch, time, and memory. Context7 and GitHub are optional and
must be explicitly enabled with secrets supplied through environment variables.

## Deferred Hardening

- Rootless Docker or user namespace remap on Linux hosts.
- gVisor `runsc` runtime for untrusted user-facing containers.
- Kata Containers or KVM for strong tenant boundaries.
- Egress policy enforcement.
- Filesystem allowlist per MCP.
- Tool-level approval policy.
