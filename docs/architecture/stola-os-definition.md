# Stola OS Definition

Stola OS is an AI-native server appliance, not a new kernel.

Current MVP direction: Docker-first agent workspaces. Proxmox/KVM remains a
future stronger-isolation backend, but it is no longer the first product gate.

It has four boundaries:

1. Stola Host OS: Linux host plus Docker/containerd now; Proxmox/KVM later.
2. Stola Control Plane: CLI, API, provisioner, state, audit, secrets, update.
3. Stola Guest OS: versioned Linux images for user VPS/container workloads.
4. AI Runtime: agents, MCP tools, model gateway keys, cron, and memory.

Host OS core v0 implements the host/control-plane foundation and now includes
an initial Docker workspace backend. It does not ship dashboard, marketplace,
GPU inference, billing, or production MCP governance.

The first useful invariant is simple:

```text
stola doctor
stola config validate
stola provisioner mock-create
stola docker check
stola daemon provisioner --once
```

These commands must work before real VPS provisioning starts.
