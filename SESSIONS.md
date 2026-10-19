# Stola OS Sessions

## 2026-06-24 - Host OS Core v0 Start

### Intent

Start implementation of the Stola Host OS Core v0 from the approved plan, while keeping project state files current.

### Decisions

- Use Go for the core host binary and daemons.
- Keep Proxmox as the target substrate, but start with a mock provisioner backend.
- Use SQLite for single-node local state.
- Use JSONL audit logs for append-only operational/security events.
- Keep frontend, real VPS creation, LiteLLM/Hermes-in-guest, billing, marketplace, GPU, and multi-node out of v0.

### Work Completed Before This Session Entry

- Created Go module skeleton.
- Added `cmd/stola` CLI with commands:
  - `doctor`
  - `init`
  - `config validate`
  - `node status`
  - `provisioner mock-create`
  - `service status`
  - `daemon api|provisioner|auditd`
- Added config parser/validator.
- Added JSONL audit logger with redaction.
- Added SQLite state store.
- Added mock provisioner.
- Added Proxmox backend stub.
- Added systemd unit files and `.deb` skeleton.
- Added ADRs and architecture docs.
- Added Go tests for config, audit, state, provisioner, and CLI behavior.

### Current Session Work

- Added `GOALS.md`, `SESSIONS.md`, and `TESTS.md`.
- Marked the Host OS Core v0 slice inside `TASKS.md`.
- Downloaded official Go `go1.26.4.windows-amd64.zip` to `%TEMP%\stola-go-toolchain`.
- Verified the Go ZIP SHA-256 from `go.dev/dl/?mode=json`.
- Used the portable Go toolchain without installing Go globally.
- Ran `go mod tidy`.
- Ran `gofmt -w cmd internal`.
- Ran `go test ./...`.
- Ran the manual CLI smoke test from `TESTS.md`.
- Removed smoke-test artifacts from `tmp/smoke`.

### Known Issues

- The machine still does not have global Go in `PATH`; use `%TEMP%\stola-go-toolchain\go\bin\go.exe` or install Go globally.
- Superseded by the later initial Proxmox adapter entry: Proxmox API calls are implemented and tested against `httptest`, but not yet validated on a live lab host.

### Test Results

```text
go mod tidy: passed
gofmt -w cmd internal: passed
go test ./...: passed

ok  	stola-os/cmd/stola
ok  	stola-os/internal/audit
ok  	stola-os/internal/config
ok  	stola-os/internal/provisioner
ok  	stola-os/internal/state
```

Manual smoke:

```text
stola init: passed
stola config validate: passed
stola doctor: passed
stola node status: passed
stola provisioner mock-create: passed
stola daemon provisioner --once: passed
stola service status: passed
```

## 2026-06-24 - Initial Proxmox Adapter

### Intent

Implement the first real Proxmox adapter behind the current provisioner interface, without attempting live LXC creation outside an explicit lab host.

### Decisions

- Use direct Proxmox REST calls for the first adapter instead of adding a wrapper library.
- Keep authentication as `token_file`; raw tokens are not valid YAML config keys.
- Keep self-signed Proxmox TLS support explicit with `insecure_skip_tls_verify`.
- Treat LXC Nano create as a lab feature until validated on a real Proxmox node.
- Persist Proxmox VMID/CTID as `external_id` and the Proxmox UPID as `task_id`.

### Work Completed

- Added Proxmox config fields:
  - `backend.proxmox.endpoint`
  - `backend.proxmox.node`
  - `backend.proxmox.token_file`
  - `backend.proxmox.insecure_skip_tls_verify`
- Implemented Proxmox client methods:
  - `GET /api2/json/version`
  - `GET /api2/json/nodes`
  - `GET /api2/json/nodes/{node}/storage`
  - `POST /api2/json/nodes/{node}/lxc`
- Added CLI commands:
  - `stola proxmox check`
  - `stola proxmox nodes`
  - `stola proxmox storage`
  - `stola vps create`
- Extended provisioner interface with generic `Create`.
- Extended SQLite `vps` table with `external_id` and `task_id`.
- Added tests for Proxmox token auth, inventory reads, LXC Nano form generation, CLI integration, config parsing, and state persistence.

### Known Issues

- No live Proxmox endpoint was provided, so LXC creation is not yet validated on a real lab node.
- No retry/backoff/idempotency key yet.
- No Proxmox task polling yet; create returns `status=creating` with the UPID.
- No automatic template discovery/download yet; caller must pass `--template`.
- No create/delete cleanup flow yet.

### Test Results

```text
gofmt -w cmd internal: passed
go test ./...: passed

ok  	stola-os/cmd/stola	(cached)
ok  	stola-os/internal/audit	(cached)
ok  	stola-os/internal/config	0.245s
ok  	stola-os/internal/provisioner	(cached)
ok  	stola-os/internal/proxmox	(cached)
ok  	stola-os/internal/state	0.489s
?   	stola-os/internal/version	[no test files]
```

Manual smoke, local mock mode:

```text
stola init: passed
stola config validate: passed
stola vps create --name nano-a --plan nano: passed
stola node status: passed
```

## 2026-06-24 - Proxmox Lab Check Preparation

### Intent

Prepare the repo to run the first live Proxmox gate safely: `stola proxmox check` only, before any `vps create`.

### Work Completed

- Added `.gitignore` entries for `tmp/`, local DBs, audit JSONL, token files, PEM/key files, and secret files.
- Added `examples/proxmox-lab.env.example.ps1` with non-secret lab environment variables.
- Added `scripts/proxmox-lab-check.ps1`.
- Added `scripts/proxmox-bootstrap-token.ps1` to create a lab Proxmox API user/token/ACL through SSH and write `tmp/pve.token` locally.
- Created local ignored placeholder `tmp/pve.token` with zero bytes.

### Safety Notes

- The token file exists but is empty; the real token must be written locally and should not be pasted into chat.
- The token can now be generated locally by the bootstrap script if SSH admin access to Proxmox is available.
- The script only generates `tmp/pve.yaml` and runs `stola proxmox check`.
- The script does not run `stola vps create`.

### Test Results

```text
go test ./...: passed
scripts/proxmox-bootstrap-token.ps1 parse check: passed
scripts/proxmox-bootstrap-token.ps1 placeholder guard: passed
scripts/proxmox-lab-check.ps1 parse check: passed
scripts/proxmox-lab-check.ps1 empty-token guard: passed
live proxmox check: not run, missing endpoint and non-empty token
```

## 2026-06-24 - Docker-First Agent Workspace Pivot

### Intent

Move the active MVP path from Proxmox-first VPS provisioning to Docker-backed
agent workspaces, because the actual product goal is safe user environments for
Hermes, OpenCode, OpenClaw, MCPs, memory, cron, tools, and workspaces.

### Decisions

- Keep Proxmox as a future/premium stronger-isolation backend.
- Add Docker as the active MVP backend behind `provisioner.Backend`.
- Keep the existing CLI/state/audit surface and reuse `stola vps create` for
  now, even though the user-facing term should become workspace/capsule later.
- Do not claim Docker workspaces are strong VPS isolation.

### Work Completed

- Added `backend.type=docker`.
- Added `backend.docker` config fields.
- Added `internal/docker`.
- Added `stola docker check`.
- Added Docker-backed `stola vps create`.
- Added per-workspace Docker network and volume.
- Added memory, swap, CPU, and PID limits.
- Added `--cap-drop ALL` and `--security-opt no-new-privileges:true`.
- Added ADR-006 and Docker workspace architecture doc.
- Updated threat model and project state docs.

### Test Results

```text
go test ./...: passed
stola docker check against local Docker Desktop: passed
stola vps create against local Docker Desktop with busybox:1.36: passed
cleanup of smoke container/network/volume: passed
```

Smoke evidence:

```text
server_version: 29.4.3
security: ["name=seccomp,profile=builtin","name=cgroupns"]
created vps: id=docker-stola-smoke-docker name=smoke-docker plan=nano backend=docker status=running
container inspect: running ALL no-new-privileges:true
```

## 2026-06-24 - Agent Base Image And Docker Lifecycle

### Intent

Create the first runnable Stola agent workspace image and add Docker lifecycle
commands needed to operate those workspaces from the CLI.

### Work Completed

- Added `images/stola-agent-base/Dockerfile`.
- Added `manifests/mcp/default.yaml`.
- Added `.dockerignore` for Docker build hygiene.
- Added `scripts/smoke-stola-agent-base.ps1`.
- Installed Hermes, OpenCode, OpenClaw, and `uvx` in the base image.
- Copied the default MCP manifest into `/opt/stola/manifests/mcp/default.yaml`.
- Ran the image as non-root user `stola` UID `10001` in `/workspace`.
- Added SQLite helpers to get/update/delete VPS rows.
- Added Docker adapter methods for status, logs, exec, stop, start, and delete.
- Added CLI commands:
  - `stola vps list`
  - `stola vps status --id|--name`
  - `stola vps logs --id|--name`
  - `stola vps exec --id|--name -- COMMAND`
  - `stola vps stop --id|--name`
  - `stola vps start --id|--name`
  - `stola vps delete --id|--name`

### Test Results

```text
go test ./...: passed
scripts/smoke-stola-agent-base.ps1 parse check: passed
scripts/smoke-stola-agent-base.ps1: passed
stola-agent-base:dev build: passed
stola vps create --image stola-agent-base:dev: passed
stola vps list/status/logs/exec/stop/start/delete: passed
Docker cleanup of agent smoke container/network/volume: passed
SQLite state row cleanup after vps delete: passed
```

Smoke evidence:

```text
ok: hermes command
ok: opencode command
ok: openclaw command
ok: uvx command
ok: default MCP manifest
ok: non-root workspace user
ok: workspace directory
created vps: id=docker-stola-stola-agent-smoke name=stola-agent-smoke plan=nano backend=docker status=running
/usr/local/bin/hermes
/usr/local/bin/opencode
/usr/local/bin/openclaw
stola-log-ok
stopped vps: id=docker-stola-stola-agent-smoke name=stola-agent-smoke
started vps: id=docker-stola-stola-agent-smoke name=stola-agent-smoke
deleted vps: id=docker-stola-stola-agent-smoke name=stola-agent-smoke
CLI Docker agent smoke passed
```

## 2026-06-24 - MCP Manager And Workspace Injection

### Intent

Implement the first Stola MCP manager: read `manifests/mcp/default.yaml`, list
allowed servers, render minimal host configs, and inject the generated bundle
into Docker-created workspaces.

### Decisions

- Keep `manifests/mcp/default.yaml` as the source of truth.
- Generate host-specific config files from the Stola manifest instead of hand
  maintaining separate OpenCode/Hermes/OpenClaw configs.
- Keep optional MCP servers disabled by default.
- Do not write secret values into generated configs; only list required env var
  names in the Stola allowlist.
- Inject files into `/workspace` with `docker exec` and base64 writes as the
  container user, avoiding root-owned files in the workspace volume.

### Work Completed

- Added `internal/mcp`.
- Added manifest validation for:
  - required version/name/workspace root.
  - duplicate server IDs.
  - supported transports.
  - stdio command requirements.
  - HTTP URL requirements.
  - default-enabled servers requiring secrets.
- Added render outputs:
  - `.stola/mcp/default.yaml`
  - `.stola/mcp/allowlist.json`
  - `opencode.json`
  - `.stola/mcp/hermes.config.yaml`
  - `.stola/mcp/openclaw.json`
- Added CLI commands:
  - `stola mcp validate`
  - `stola mcp list`
  - `stola mcp render`
- Added Docker create flags:
  - `--mcp-manifest`
  - `--no-mcp-inject`
- Enabled automatic MCP config injection for Docker-backed `stola vps create`.
- Added audit event `mcp.inject` after successful injected create.
- Added tests for MCP parse/render/write and Docker injection.

### Test Results

```text
go mod tidy: passed
gofmt/go fmt ./cmd/... ./internal/...: passed
go test ./...: passed
stola mcp validate: passed
stola mcp list: passed
stola mcp render --workspace ./tmp/mcp-render-smoke: passed
real Docker MCP injection smoke with busybox:1.36: passed
cleanup of smoke container/network/volume/state row: passed
```

## 2026-06-24 - Control Plane Identity, RBAC, Signed Approvals, And Policy Reload

### Intent

Connect MCP execution policy to real control-plane identity instead of relying
only on local CLI context. The active goal is: authenticated user -> RBAC
permission -> server-signed approval -> CLI consumes exactly one matching MCP
approval under the current policy version.

### Decisions

- Keep the MCP base manifest as the catalog of allowed servers and generated
  agent configs.
- Move per-user/per-plan operational policy changes into a versioned policy
  overlay file.
- Use Bearer tokens in v0, but store only SHA-256 token hashes in the users
  file.
- Use role permissions for API authorization:
  - `mcp.approve`
  - `policy.read`
  - `policy.reload`
- Sign approval tokens with a local HMAC key owned by the control plane.
- Keep SQLite approval intents as the one-shot source of truth; the signed token
  proves server-issued intent and binding.
- Preserve `--approval-id` as a lab/local fallback, but make
  `--approval-token` the production-facing path.

### Work Completed

- Added API config fields:
  - `api.users_file`
  - `api.signing_key_file`
  - `api.mcp_manifest`
  - `api.policy_file`
- Added `internal/controlplane`:
  - users/RBAC loader
  - Bearer token authentication by SHA-256 hash
  - HMAC approval token signing and verification
  - HTTP API server
- Implemented API routes:
  - `GET /healthz`
  - `GET /v1/policy`
  - `POST /v1/policy/reload`
  - `POST /v1/mcp/approvals`
- Added versioned MCP policy overlays outside the base manifest.
- Added effective policy version hashing from manifest + overlay bytes.
- Added `--policy-file` to MCP CLI flows.
- Wired `api.policy_file` into `stola mcp tools` and `stola mcp call`.
- Added `--approval-token` to `stola mcp call`.
- Validated signed approval tokens against:
  - VPS ID
  - server ID
  - tool name
  - canonical argument digest
  - policy user ID
  - current policy version
  - expiration
- Persisted `approved_by` and `policy_version` on MCP approvals.
- Converted `stola daemon api` into a real HTTP API daemon.
- Added examples:
  - `examples/control-plane-users.example.yaml`
  - `examples/mcp-policy.example.yaml`
- Updated task, goals, and test docs.

### Test Results

```text
go fmt ./cmd/... ./internal/...: passed
go test ./...: passed
stola mcp validate --manifest .\manifests\mcp\default.yaml --policy-file .\examples\mcp-policy.example.yaml: passed
policy_version=1c7f2bb1ef23cd92
```

Unit coverage added:

```text
internal/controlplane: RBAC auth, HMAC approval token sign/verify, API approval create
internal/mcp: policy overlay applies execution_policy and server tool policy
internal/state: approval approved_by/policy_version metadata
```

### Known Issues

- No live long-running `stola daemon api` smoke was run with curl in this turn;
  API behavior is covered through `httptest`.
- Tokens are v0 static Bearer tokens; OIDC/session login is future work.
- Policy reload validates and updates the in-process API snapshot; running CLI
  calls still load the current policy file from disk per invocation.
- `--approval-id` remains available for local/lab fallback and should be gated
  or removed before exposing this to final users.

### Next Step

Build the user-facing approval flow on top of the API: a dashboard or control
plane client that calls `POST /v1/mcp/approvals`, stores no raw secrets, and
passes only the returned `approval_token` to the execution path.

## 2026-06-24 - Localhost Control Plane UX Smoke

### Intent

Validate the real user experience locally only, without exposing any host
service outside the machine: API on `127.0.0.1`, Docker workspace, native agent
tool configs, MCP calls, pending approval, API-signed approval, execution, audit,
and cleanup.

### Decisions

- Keep this smoke strictly local with `api.listen=127.0.0.1`.
- Generate a temporary Bearer token in memory and write only its SHA-256 hash to
  the local users file.
- Add an API endpoint to approve an existing pending CLI approval intent instead
  of forcing the API to create a separate approval.
- Keep the signed approval token out of logs and only pass it directly to the
  CLI invocation.

### Work Completed

- Added `state.ApproveMCPApproval`.
- Added `POST /v1/mcp/approvals/{id}/approve`.
- Updated CLI approval-required output with:
  - `api_approve: POST /v1/mcp/approvals/{id}/approve`
  - `rerun_with_signed: --approval-token <approval_token>`
- Added `scripts/smoke-control-plane-ux-local.ps1`.
- Made the script compatible with Windows PowerShell without UTF-8 BOM issues.
- Updated task, goal, and test docs.

### Test Results

```text
go test ./...: passed
scripts/smoke-control-plane-ux-local.ps1 parse check: passed
scripts/smoke-control-plane-ux-local.ps1: passed
```

Smoke coverage:

```text
API health on http://127.0.0.1:18750: passed
GET /v1/policy: passed
POST /v1/policy/reload: passed
Docker workspace create from stola-agent-base:dev: passed
agent commands hermes/opencode/openclaw/uvx: passed
workspace MCP config files: passed
native agent config files: passed
opencode mcp list: passed
hermes mcp list: passed
openclaw config validate/status/list: passed
stola mcp tools filesystem: passed
stola mcp call list_allowed_directories: passed
stola mcp call read_text_file: passed
write_file without approval produced pending approval_id: passed
POST /v1/mcp/approvals/{id}/approve signed pending approval: passed
stola mcp call write_file --approval-token: passed
approval token reuse denied: passed
blocked .stola path denied before approval: passed
audit events present: passed
```

Post-smoke cleanup:

```text
API on 127.0.0.1:18750 stopped: passed
no stola-cp-ux-smoke container/network/volume remained: passed
temporary users file and control-plane signing key removed: passed
```

### Known Issues

- This is still a local CLI/API smoke, not a browser dashboard.
- The API currently trusts any authenticated operator with `mcp.approve`; richer
  per-user approval scopes are still future work.
- `--approval-id` remains for local/lab fallback and should be disabled or
  gated before a real multi-user release.

## 2026-06-24 - Local Approval CLI UX

### Intent

Remove the awkward manual token-copy step from the local control-plane approval
flow while keeping the API on localhost and avoiding raw tokens in shell
history.

### Work Completed

- Added `GET /v1/mcp/approvals`.
- Added `state.ListMCPApprovals`.
- Added `stola approvals list`.
- Added `stola approvals approve --id ID --write-token PATH`.
- Added `stola mcp call --approval-token @PATH`.
- Updated `scripts/smoke-control-plane-ux-local.ps1` to use:
  - `stola approvals list`
  - `stola approvals approve --write-token`
  - `stola mcp call --approval-token @approval.token`
- Kept API auth token in `STOLA_API_TOKEN` during the smoke.
- Removed temporary users file, signing key, and approval token file after the
  smoke.

### Test Results

```text
go test ./...: passed
scripts/smoke-control-plane-ux-local.ps1 parse check: passed
scripts/smoke-control-plane-ux-local.ps1: passed
```

Smoke evidence:

```text
stola approvals list pending: passed
stola approvals approve pending id: passed
stola mcp call write_file with signed approval file: passed
signed approval token is one-shot: passed
blocked path denied before approval: passed
audit events present: passed
```

Post-smoke cleanup:

```text
API on 127.0.0.1:18750 stopped: passed
no stola-cp-ux-smoke container/network/volume remained: passed
temporary users file, control-plane signing key, and approval.token removed: passed
```

## 2026-06-24 - General MCP Execution Policy Engine

### Intent

Add a general MCP policy layer beyond filesystem path checks. The CLI should
evaluate server risk, network class, command binary, rule-based approval, and
rate limits before executing MCP tool calls.

### Decisions

- Add root `execution_policy` to the MCP manifest.
- Preserve `tool_policy` as a compatibility input into the final execution
  decision.
- Evaluate path policy and execution policy before `tools/list` so blocked
  command/network/risk configurations do not start the MCP server for preflight.
- Apply persistent rate limits after the requested tool is confirmed advertised.
- Store rate counters in SQLite because CLI calls are short-lived processes.
- Audit execution denies as `mcp.policy`.
- Audit rate limit decisions as `mcp.rate_limit`.

### Work Completed

- Added `ExecutionPolicy`, `RiskPolicy`, `NetworkPolicy`, `CommandPolicy`,
  `ExecutionRule`, `ExecutionRuleMatch`, and `RateLimitPolicy`.
- Added `EvaluateExecutionPolicy`.
- Added root `execution_policy` to `manifests/mcp/default.yaml`.
- Added SQLite table `mcp_rate_limits`.
- Added `ConsumeMCPRateLimit`.
- Wired `stola mcp call` to:
  - deny path/risk/network/command before `tools/list`
  - enforce persistent rate limits
  - create one-shot approvals from execution-policy decisions
- Rendered `execution_policy` into the MCP allowlist.
- Added unit tests for rule approval, network deny, command deny, rule rate
  limit override, and SQLite rate limit reset.
- Updated task/goals/test docs.

### Test Results

```text
gofmt via %TEMP%\stola-go-toolchain\go\bin\gofmt.exe: passed
go test ./internal/mcp ./internal/state ./cmd/stola: passed
go test ./...: passed
stola mcp validate --manifest .\manifests\mcp\default.yaml: passed
stola mcp render --manifest .\manifests\mcp\default.yaml: passed
Docker execution policy smoke with stola-agent-base:dev: passed
custom manifest max_calls=1 allowed first list_allowed_directories: passed
second list_allowed_directories denied by mcp.rate_limit: passed
custom manifest denying command bin npx denied filesystem MCP through mcp.policy: passed
cleanup of smoke container/network/volume/state row: passed
```

## 2026-06-24 - Configurable MCP Argument Policy

### Intent

Move filesystem MCP argument/path policy out of hardcoded-only behavior and into
the MCP manifest, while preserving safe defaults for old manifests.

### Decisions

- Add `argument_policy` to MCP server entries.
- Keep safe defaults in the core when `argument_policy` is omitted.
- Support scoped overrides by:
  - `vps_ids`
  - `plans`
  - `user_ids`
- Treat `user_id` as policy context only in the CLI; real security identity
  still needs control-plane auth/RBAC later.
- Allow manifest-defined path schemas for new filesystem tools.
- Keep `deny_unknown_tools=true` by default.
- Keep blocked path calls before approval creation.

### Work Completed

- Added `ArgumentPolicy`, `ArgumentPathField`, `ArgumentPolicyOverride`,
  `ArgumentPolicyScope`, and `ArgumentPolicyContext`.
- Added effective policy resolution with manifest config plus matching scoped
  overrides.
- Added `argument_policy` to `manifests/mcp/default.yaml`.
- Rendered `argument_policy` into the allowlist.
- Wired CLI path policy evaluation with real `vps_id`, `plan`, and optional
  `--policy-user-id`/`STOLA_POLICY_USER_ID`.
- Added unit tests for plan override, VPS/user scoped override, and
  manifest-defined tool schemas.
- Updated task/goals/test docs.

### Test Results

```text
gofmt via %TEMP%\stola-go-toolchain\go\bin\gofmt.exe: passed
go test ./internal/mcp ./cmd/stola: passed
go test ./...: passed
Docker configurable policy smoke with stola-agent-base:dev: passed
custom manifest override for plan trusted allowed /workspace/opencode.json to reach approval: passed
write_file /workspace/opencode.json with matching approval_id: passed once
same trusted plan still denied /workspace/.stola/mcp/default.yaml before approval: passed
cleanup of smoke container/network/volume/state row: passed
```

## 2026-06-24 - MCP Filesystem Argument Path Policy

### Intent

Add argument-level safety before approval creation. A dangerous filesystem MCP
call should not receive an `approval_id` if its path escapes `/workspace` or
targets protected metadata, agent configs, env files, keys, or secret-looking
filenames.

### Decisions

- Enforce the first path policy in `internal/mcp`, not only in the CLI.
- Apply it to the default `filesystem` MCP server.
- Validate known filesystem tool schemas:
  - `path`
  - `paths`
  - `source`
  - `destination`
- Deny unknown filesystem tools until their argument schema is explicitly
  covered.
- Audit path policy denies as `mcp.argument_policy`.
- Do not store raw paths in audit details; store only argument keys, digest,
  path fields, path count, and sanitized deny rule.

### Work Completed

- Added `ValidateArgumentPathPolicy`.
- Blocked paths outside `/workspace`, relative traversal, Windows host path
  syntax, `.stola`, `.git`, secret/config directories, env files, agent config
  files, key files, and secret/token/credential-looking filenames.
- Wired the policy into `stola mcp call` before `ToolDecision` and before
  `approval_id` creation.
- Added unit tests for valid workspace paths, traversal, protected metadata,
  key/env files, multiple path arrays, and unknown filesystem tools.
- Updated task/goals/test docs.

### Test Results

```text
gofmt via %TEMP%\stola-go-toolchain\go\bin\gofmt.exe: passed
go test ./internal/mcp ./cmd/stola: passed
go test ./...: passed
Docker path policy smoke with stola-agent-base:dev: passed
write_file path=/etc/passwd: denied by mcp.argument_policy before approval
write_file path=/workspace/.stola/mcp/default.yaml: denied by mcp.argument_policy before approval
blocked path calls did not print approval_id: passed
valid write to /workspace/path-policy-ok.txt with one-shot approval: passed
cleanup of smoke container/network/volume/state row: passed
```

Smoke evidence:

```text
mcp manifest valid: manifests/mcp/default.yaml name=stola-default-mcp servers=5 optional=2
id=filesystem enabled=true optional=false risk=medium transport=stdio command="npx -y @modelcontextprotocol/server-filesystem /workspace"
id=context7 enabled=false optional=true risk=low transport=http url=https://mcp.context7.com/mcp secrets=CONTEXT7_API_KEY
/workspace/.stola/mcp/default.yaml
/workspace/.stola/mcp/allowlist.json
/workspace/opencode.json
/workspace/.stola/mcp/hermes.config.yaml
/workspace/.stola/mcp/openclaw.json
```

## 2026-06-24 - Native MCP Smoke In stola-agent-base

### Intent

Run the real native MCP smoke inside a Docker workspace created from
`stola-agent-base:dev`: OpenCode should load the generated config, Hermes
should load `mcp_servers`, and OpenClaw should read its managed MCP config.

### Findings

- `opencode mcp list` reads `/workspace/opencode.json` and lists all 7 Stola
  entries.
- OpenCode connected immediately to filesystem, fetch, memory, and time.
- OpenCode `git` failed in an empty `/workspace`, then connected after `git
  init` and an initial commit.
- `hermes mcp list` did not read the workspace-only config, but did read the
  same generated YAML after injection into `/home/stola/.hermes/config.yaml`.
- `openclaw mcp status` did not read the workspace-only config, and current
  OpenClaw rejects stdio MCP servers in `mcp.servers`.
- OpenClaw config became valid after rendering only remote-compatible servers
  (`streamable-http`/`sse`) into `/home/stola/.openclaw/openclaw.json`.

### Work Completed

- Extended Docker MCP injection to also write:
  - `/home/stola/.config/opencode/opencode.json`
  - `/home/stola/.hermes/config.yaml`
  - `/home/stola/.openclaw/openclaw.json`
- Updated OpenClaw renderer to omit stdio servers unsupported by the current
  OpenClaw config schema.
- Updated tests for native config injection and OpenClaw remote-only rendering.

### Test Results

```text
go mod tidy: passed
go fmt ./cmd/... ./internal/...: passed
go test ./...: passed
stola vps create --image stola-agent-base:dev: passed
native config injection under /home/stola: passed
opencode mcp list: passed
hermes mcp list: passed
openclaw config validate: passed
openclaw mcp status: passed
openclaw mcp list: passed
cleanup of smoke container/network/volume/state row: passed
```

Smoke evidence:

```text
opencode mcp list:
  context7 disabled
  fetch connected
  filesystem connected
  git connected after git init
  github disabled
  memory connected
  time connected

hermes mcp list:
  fetch/filesystem/git/memory/time enabled from mcp_servers

openclaw config validate:
  Config valid: ~/.openclaw/openclaw.json

openclaw mcp status:
  context7: disabled

audit:
  vps.create success
  mcp.inject success
```

## 2026-06-24 - Audited MCP Tool Execution

### Intent

Implement the first direct MCP execution path in Stola: list tools and call
tools from allowed stdio MCP servers inside a Docker workspace, with audit
events for discovery, execution, and policy denies.

### Decisions

- Implement only MCP stdio execution in this slice.
- Use the official MCP lifecycle: `initialize`, `notifications/initialized`,
  then `tools/list` or `tools/call`.
- Execute stdio MCP servers inside the workspace with:
  `docker exec -i -w /workspace <container> <server command...>`.
- Use the Stola manifest as policy source.
- Deny optional/disabled servers and non-stdio servers for direct execution.
- Preflight every `mcp call` with `tools/list`; deny if the requested tool is
  not advertised by the server.
- Do not store raw tool arguments or raw tool results in audit logs. Store only
  argument keys, argument byte count, result content types/count, result byte
  count, duration, server, and tool.

### Work Completed

- Added MCP JSON-RPC protocol helpers for:
  - `initialize`
  - `notifications/initialized`
  - `tools/list`
  - `tools/call`
  - JSON-RPC error parsing
- Added Docker stdio execution with stdin support.
- Added CLI commands:
  - `stola mcp tools --name|--id --server SERVER`
  - `stola mcp call --name|--id --server SERVER --tool TOOL --input JSON|@file`
- Added audit events:
  - `mcp.tools`
  - `mcp.call`
  - `mcp.policy`
- Added tests for protocol payloads/parsing, Docker stdin exec, and summaries.

### Test Results

```text
go fmt ./cmd/... ./internal/...: passed
go test ./...: passed
stola vps create --image stola-agent-base:dev: passed
stola mcp tools --server filesystem: passed
stola mcp call --tool list_allowed_directories: passed
stola mcp call --tool read_text_file --input @file: passed
stola mcp tools --server context7: denied as expected
cleanup of smoke container/network/volume/state row: passed
```

Smoke evidence:

```text
stola mcp tools --server filesystem:
  listed 14 tools, including list_allowed_directories and read_text_file

stola mcp call --tool list_allowed_directories:
  Allowed directories:
  /workspace

stola mcp call --tool read_text_file:
  stola mcp call smoke

audit:
  mcp.tools success with tool_count=14
  mcp.call success with argument_keys=[] for list_allowed_directories
  mcp.call success with argument_keys=["path"] for read_text_file
  mcp.policy denied for context7 disabled by manifest
```

### Remaining Work

- Per-tool policy and CLI approval were added in the next session entry.
- Add streaming/progress handling and cancellation.
- Add HTTP/streamable MCP execution if/when needed.
- Add durable tool-call IDs and richer correlation with agent sessions.

## 2026-06-24 - Per-Tool MCP Policy And CLI Approval

### Intent

Add a per-tool policy layer before exposing MCP execution broadly. The immediate
goal is to prevent automatic execution of dangerous filesystem tools such as
`write_file`, `edit_file`, `move_file`, and `create_directory`.

### Decisions

- Store tool policy in the MCP manifest per server.
- Support three actions:
  - `allow`
  - `require_approval`
  - `deny`
- Treat this first approval model as non-interactive CLI approval through
  `--approve-risk`.
- Keep raw arguments/results out of audit logs even for approved calls.
- Filesystem read-style tools stay allowed by default.
- Filesystem write/mutate tools require approval.
- Git and Memory MCPs require approval by default because they mutate local
  state or workspace history.

### Work Completed

- Added `tool_policy` to MCP manifest server entries.
- Added policy validation for invalid actions, duplicate tool rules, and empty
  tool names.
- Added per-tool decision resolution.
- Added `--approve-risk` to `stola mcp call`.
- Added `mcp.approval` audit events:
  - `not_required`
  - `required`
  - `approved`
  - `denied`
- Updated `manifests/mcp/default.yaml`:
  - `filesystem.write_file`: `require_approval`
  - `filesystem.edit_file`: `require_approval`
  - `filesystem.move_file`: `require_approval`
  - `filesystem.create_directory`: `require_approval`
  - `git` default: `require_approval`
  - `memory` default: `require_approval`

### Test Results

```text
go fmt ./cmd/... ./internal/...: passed
go test ./...: passed
stola vps create --image stola-agent-base:dev: passed
stola mcp call --tool list_allowed_directories: passed without approval
stola mcp call --tool write_file: denied without approval
stola mcp call --tool write_file --approve-risk: passed
cleanup of smoke container/network/volume/state row: passed
```

Smoke evidence:

```text
write_file without approval:
  mcp tool "write_file" requires explicit approval: rerun with --approve-risk

write_file with approval:
  Successfully wrote to /workspace/policy-smoke.txt

audit:
  mcp.approval not_required for list_allowed_directories
  mcp.approval required for write_file
  mcp.approval approved for write_file
  mcp.call success for approved write_file
```

### Remaining Work

- Replace CLI-only approval with API/dashboard user approval.
- Add per-plan and per-user policy overrides.
- Add deny-by-default modes for high-risk workspaces.
- Add policy on argument paths, not only tool names.
- Add one-shot approval IDs instead of a raw `--approve-risk` flag.

## 2026-06-24 - One-Shot MCP Approval IDs

### Intent

Replace the raw `--approve-risk` bypass with an auditable approval intent. A
risky MCP tool call should first create an `approval_id` tied to the exact VPS,
server, tool, and argument digest; execution should happen only once with that
matching ID.

### Decisions

- Persist approvals in SQLite in `mcp_approvals`.
- Bind each approval to:
  - VPS ID
  - MCP server ID
  - tool name
  - canonical argument digest
  - argument keys
  - expiration timestamp
- Print `approval_id`, server, tool, argument keys, digest, and expiration to
  the CLI without raw argument values.
- Consume the approval before tool execution. If the MCP call later fails, the
  approval remains used.
- Reject expired, mismatched, already-used, or unnecessary approval IDs.

### Work Completed

- Added `MCPApproval` state model and SQLite migration.
- Added `CreateMCPApproval`, `GetMCPApproval`, and `ConsumeMCPApproval`.
- Added canonical MCP argument digesting.
- Replaced `--approve-risk` with `--approval-id` and `--approval-ttl`.
- Updated audit details for `mcp.approval` with `approval_id`, argument digest,
  argument keys, and expiration metadata.
- Updated task/goals/test docs for the new approval flow.

### Test Results

```text
gofmt via %TEMP%\stola-go-toolchain\go\bin\gofmt.exe: passed
go test ./... via %TEMP%\stola-go-toolchain\go\bin\go.exe: passed
Docker one-shot approval smoke with stola-agent-base:dev: passed
approval intent output includes approval_id/server/tool/argument_keys/argument_digest: passed
write_file with matching --approval-id: passed once
reuse of same --approval-id: denied with already used
audit contains required/approved/denied with approval_id and argument_digest: passed
cleanup of smoke container/network/volume/state row: passed
```
## 2026-06-24 - Docker-Only Provider Catalog And LiteLLM Injection

### Intent

Stop treating the MVP as an OS/ISO/Proxmox appliance and move the active path
to Docker-only agent workspaces with a broad model-provider catalog.

### Decisions

- No ISO, installer, custom OS, or Host appliance for the MVP.
- Keep Docker as the active runtime.
- Keep Proxmox/KVM as future/premium stronger-isolation backend work only.
- Add provider support as manifest/config generation first, not live provider
  calls with real keys.
- Store provider secrets as environment variable names only; never write real
  key values into manifests, generated config, audit, or logs.

### Work Completed

- Added `internal/providers`.
- Added `manifests/providers/default.yaml` with 26 providers:
  OpenRouter, OpenAI, Anthropic, Gemini, Azure OpenAI, Bedrock, Groq, Mistral,
  Cohere, DeepSeek, xAI, Together, Fireworks, Perplexity, Cerebras, SambaNova,
  Replicate, Hugging Face, Cloudflare, NVIDIA NIM, Vercel AI Gateway, LiteLLM
  Proxy, Ollama, vLLM, LM Studio, and custom OpenAI-compatible endpoint.
- Added `stola providers validate|list|render`.
- Added generated provider files:
  - `.stola/providers/default.yaml`
  - `.stola/providers/catalog.json`
  - `.stola/providers/env.example`
  - `.stola/providers/litellm.config.yaml`
  - `.stola/providers/openai-compatible.json`
- Added Docker provider injection during `stola vps create`.
- Added native LiteLLM config injection at
  `/home/stola/.config/litellm/config.yaml`.
- Installed `litellm` in `images/stola-agent-base/Dockerfile`.
- Copied the default provider manifest into the image at
  `/opt/stola/manifests/providers/default.yaml`.
- Updated `scripts/smoke-stola-agent-base.ps1` to verify `litellm` and the
  default providers manifest.
- Fixed Windows UTF-8 BOM parsing in `internal/config`.
- Added ADR-007 and ADR-008 for the Docker-only MVP and provider catalog.

### Test Results

```text
gofmt ./cmd ./internal: passed
go test ./...: passed
stola providers validate --manifest .\manifests\providers\default.yaml: passed
stola providers list --manifest .\manifests\providers\default.yaml: passed
stola providers render --manifest .\manifests\providers\default.yaml --workspace .\tmp\providers-render-smoke: passed
scripts/smoke-stola-agent-base.ps1: passed
Real Docker provider injection smoke with stola-agent-base:dev: passed
scripts/smoke-control-plane-ux-local.ps1 with provider checks: passed
Docker cleanup check for provider-smoke container/network/volume: passed
```

### Remaining Work

- Add real secret injection per user/workspace.
- Add live provider health checks with explicit test keys.
- Add LiteLLM proxy mode with virtual keys, budgets, rate limits, and spend
  tracking.
- Rename product-facing `vps` wording to `workspace` once CLI compatibility is
  planned.

## 2026-06-24 - Provider Secret Injection Per Workspace

### Intent

Add the first real provider-secret path for Docker workspaces without exposing
keys in generated manifests, audit logs, Docker command arguments, or stdout.

### Decisions

- Keep provider keys outside the provider manifest and workspace provider
  catalog.
- Accept local env files only as an input source for the current local/dev
  control plane.
- Filter env files against `manifests/providers/default.yaml`; variables not
  declared by a provider are ignored.
- Write actual values only inside the container runtime secret file at
  `/home/stola/.config/stola/secrets.env`.
- Write only secret names to `/workspace/.stola/secrets/manifest.json`.
- Inject secret files through Docker stdin, not command-line args.
- Audit only secret names/count for `secrets.inject`, and do not audit raw
  `vps exec` argv because shell commands can reference or contain secrets.

### Work Completed

- Added `internal/secrets`.
- Added `stola secrets validate|list --file PATH`.
- Added `examples/provider-secrets.env.example`.
- Added `.env` ignores to `.gitignore` and `.dockerignore`, with example files
  allowed.
- Added `stola vps create --secrets-file PATH` for Docker-backed workspaces.
- Added Docker secret injection for:
  - `/home/stola/.config/stola/secrets.env`
  - `/home/stola/.config/stola/secrets.json`
  - `/workspace/.stola/secrets/manifest.json`
- Updated `.profile` in the container to source the generated secrets env file.
- Added audit event `secrets.inject` with names/count only.
- Changed `vps.exec` audit details from raw `argv` to `command` plus `argc`.
- Updated `scripts/smoke-control-plane-ux-local.ps1` to validate secrets,
  inject them during workspace creation, verify runtime availability, verify
  unrelated secret exclusion, and fail if audit leaks secret values or raw
  `argv`.
- Fixed the smoke setup to clear old local state/audit artifacts before each
  run, so leak checks evaluate only the current execution.

### Test Results

```text
gofmt via %TEMP%\stola-go-toolchain\go\bin\gofmt.exe: passed
go test ./... via %TEMP%\stola-go-toolchain\go\bin\go.exe: passed
scripts/smoke-control-plane-ux-local.ps1: passed
provider secrets validate: passed
provider secrets injected: passed
OPENAI_API_KEY present inside workspace shell: passed
UNRELATED_SECRET absent inside workspace shell: passed
audit no provider secret value / unrelated secret / raw argv: passed
Docker cleanup check for stola-cp-ux-smoke container/network/volume: passed
```

### Remaining Work

- Add real provider health checks with explicit test keys.
- Add LiteLLM proxy mode with virtual keys, budgets, rate limits, and spend
  tracking.
- Rename product-facing `vps` command/surface to `workspace` while preserving a
  compatibility alias.
- Add hosted multi-tenant hardening with gVisor/Kata/KVM before accepting
  untrusted users.
- Add cost/token/provider observability per workspace and user.

## 2026-06-24 - Provider Check For OpenAI-Compatible Providers

### Intent

Add a real provider health-check command that uses explicit test keys from a
secret file, starting with OpenRouter/OpenAI-style APIs, without spending tokens
or leaking provider keys.

### References Checked

- OpenAI API reference: model listing uses `GET https://api.openai.com/v1/models`
  with `Authorization: Bearer $OPENAI_API_KEY`.
- OpenRouter API reference: model listing uses
  `GET https://openrouter.ai/api/v1/models`.
- OpenRouter authentication docs: API requests use Bearer tokens, with optional
  app/referer metadata.

### Decisions

- Use model-list endpoints only for v0 provider check. This is read-only and
  should not create completions or spend tokens.
- Require `--secrets-file` explicitly. Do not read provider keys implicitly from
  the host environment.
- Start with providers whose manifest kind is OpenAI-compatible.
- Build the check endpoint from `provider.base_url + /models`.
- For OpenRouter, include optional `HTTP-Referer` and `X-Title` headers when
  the secret file includes `OPENROUTER_HTTP_REFERER`/`OPENROUTER_APP_TITLE` or
  `OR_SITE_URL`/`OR_APP_NAME`.
- Print provider ID, HTTP status, model count, endpoint, duration, and secret
  variable names only. Never print secret values.

### Work Completed

- Added `providers.FindProvider`.
- Added `internal/providers/check.go`.
- Added `providers.CheckProvider` with timeout support and model-list JSON
  parsing.
- Added `stola providers check --provider ID --secrets-file PATH`.
- Added support for `${VAR}` expansion in provider `base_url` using values from
  the explicit secret file, enabling future custom OpenAI-compatible checks.
- Added CLI/unit coverage for:
  - OpenAI-compatible `/v1/models`
  - OpenRouter `/api/v1/models`
  - optional OpenRouter headers
  - missing provider secret
  - HTTP failure without secret leakage

### Test Results

```text
gofmt via %TEMP%\stola-go-toolchain\go\bin\gofmt.exe: passed
go test ./... via %TEMP%\stola-go-toolchain\go\bin\go.exe: passed
cmd/stola providers check httptest: passed
internal/providers OpenAI-compatible check httptest: passed
internal/providers OpenRouter check httptest: passed
secret value not printed on success/error paths: passed
```

### Remaining Work

- Run `stola providers check --provider openai --secrets-file <test-env>` with a
  real OpenAI test key.
- Add provider-specific checks for non OpenAI-compatible providers such as
  Anthropic, Gemini, Bedrock, Cohere, Replicate, and Hugging Face.
- Add provider-check audit/cost telemetry once provider checks move from local
  CLI to control-plane API.

## 2026-06-24 - Live OpenRouter Provider Check

### Intent

Validate the new provider check command against a real OpenRouter API key that
expires soon, without committing or retaining the key locally.

### Command

```powershell
go run .\cmd\stola providers check --provider openrouter --secrets-file .\tmp\provider-check-openrouter.env --timeout 20s
```

### Result

```text
provider check ok: id=openrouter kind=openai_compatible status=200 models=339 endpoint=https://openrouter.ai/api/v1/models duration_ms=161 secrets=OPENROUTER_API_KEY
```

### Cleanup

- Removed `.\tmp\provider-check-openrouter.env` after the live check.
- Did not write the key to repo docs.

### Remaining Work

- Run the same live check for OpenAI with an explicit test key.
- Add non OpenAI-compatible provider checks.

## 2026-06-24 - OpenRouter End-To-End Local Flow

### Intent

Before expanding to OpenAI/Anthropic/Gemini/etc., test the entire current
Stola workspace flow with OpenRouter as the first real provider.

### Scope

- Use an explicit temporary OpenRouter test key in `tmp/openrouter-flow`.
- Keep everything local/Docker/localhost.
- Do not write the key to docs.
- Remove the secret file and Docker workspace after validation.

### Checks Passed

```text
stola build: passed
config validate: passed
docker check: passed
stola providers check --provider openrouter: HTTP 200, 339 models, 225 ms
stola vps create --image stola-agent-base:dev --secrets-file provider.env: passed
provider workspace files: passed
native LiteLLM config with openrouter: passed
OPENROUTER_API_KEY present inside workspace shell: passed
OPENAI_API_KEY absent inside workspace shell: passed
workspace secret manifest names-only/no secret value: passed
OpenRouter /api/v1/models from inside workspace: HTTP 200, 339 models
hermes/opencode/openclaw/uvx/litellm commands: passed
opencode mcp list: passed
hermes mcp list: passed
openclaw config validate/status/list: passed
stola mcp tools filesystem: passed
stola mcp call list_allowed_directories: passed
stola mcp call read_text_file: passed
write_file approval intent: passed
write_file with one-shot approval_id: passed
approval_id reuse denied: passed
audit no-leak check: passed
Docker cleanup: passed
workspace secret prefix search: absent
```

### Cleanup

- Removed the temporary provider secret file.
- Deleted `stola-openrouter-flow`.
- Removed `stola-net-openrouter-flow`.
- Removed `stola-workspace-openrouter-flow`.
- Verified the OpenRouter secret prefix was absent from workspace files.

### Remaining Work

- Leave OpenAI live check for later.
- Leave Anthropic/Gemini/Bedrock/Cohere/Replicate/Hugging Face checks for later,
  after OpenRouter is the stable reference path.
- Consider converting this manual flow into a reusable
  `scripts/smoke-openrouter-flow-local.ps1` that accepts `-SecretsFile` and
  never stores provider keys in the repo.

## 2026-06-24 - OpenRouter User-Like Agent Flow

### Intent

Answer whether the flow works like a user would use it, not only as a provider
health check. This means creating a workspace, injecting OpenRouter, calling the
agent CLIs with real LLM requests, running MCP calls, and validating cleanup.

### Checks Passed

```text
go test ./...: passed
stola providers check --provider openrouter: HTTP 200, 339 models, 171 ms
stola vps create --image stola-agent-base:dev --secrets-file provider.env: passed
OPENROUTER_API_KEY injected in workspace shell: passed
OpenRouter /api/v1/models from inside workspace: passed
opencode run -m openrouter/openai/gpt-4o-mini Say exactly STOLA_OK: returned STOLA_OK
hermes chat --query "Say exactly STOLA_OK" --provider openrouter --model openai/gpt-4o-mini: returned STOLA_OK
openclaw agent --local --session-key agent:default:openrouter-smoke --model openrouter/openai/gpt-4o-mini: returned STOLA_OK
litellm.completion(model="openrouter/openai/gpt-4o-mini"): returned STOLA_OK
stola mcp tools filesystem: passed
stola mcp call list_allowed_directories: passed
stola mcp call read_text_file: passed
stola mcp call write_file approval intent: passed
stola mcp call write_file with one-shot approval_id: passed
approval_id reuse denied: passed
audit no-leak check: passed
Docker cleanup: passed
workspace secret prefix search: absent
```

### Real Findings

- OpenCode, Hermes, OpenClaw, and LiteLLM can all call OpenRouter from inside
  the Stola workspace with the injected secret.
- `opencode run` accepts the prompt as positional words without a `--`
  separator in this Windows/PowerShell driven path.
- For complex shell scripts with quotes, passing the script as base64 into the
  container is more reliable than `sh -lc "<quoted script>"` from Windows.
- OpenClaw requires an explicit session selector such as `--session-key` for
  `openclaw agent --local`.

### Cleanup

- Removed the temporary provider secret file.
- Deleted `stola-openrouter-user-flow`.
- Removed `stola-net-openrouter-user-flow`.
- Removed `stola-workspace-openrouter-user-flow`.
- Verified the OpenRouter secret prefix was absent from workspace files.

### Remaining Work

- Turn this manual flow into an automated smoke script.
- Keep OpenAI/Anthropic/Gemini/etc. for later; OpenRouter is now the reference
  provider path.

## 2026-06-24 - Stolas Workspace API And OpenRouter Smoke Gate

### Intent

Make the current product surface match Stolas workspaces instead of VPS wording,
and turn the OpenRouter user-like flow into a repeatable local gate before
future changes.

### Work Completed

- Added `stola workspace create|list|status|logs|exec|stop|start|delete`.
- Kept `stola vps ...` as a temporary compatibility alias.
- Added `POST /v1/workspaces`, `GET /v1/workspaces`, and
  `GET /v1/workspaces/{id}` to the local control-plane API.
- Added RBAC permissions `workspace.create` and `workspace.read`.
- Added SQLite usage ledger for workspace/user/provider/model tokens, cost,
  latency, status, and errors.
- Added `POST /v1/usage`, `GET /v1/usage`, and `GET /v1/usage/summary`.
- Added RBAC permissions `usage.write` and `usage.read`.
- Added `scripts/smoke-openrouter-flow-local.ps1` with required
  `-SecretsFile`.
- Automated the OpenRouter flow: provider check, workspace create, secret
  injection, OpenCode, Hermes, OpenClaw, LiteLLM, MCP tools/calls, one-shot
  approval, audit no-leak check, and cleanup.
- Pinned filesystem MCP to
  `@modelcontextprotocol/server-filesystem@2025.8.21` because the unpinned
  latest package failed under Node 24.

### Checks Passed

```text
gofmt -w .\cmd .\internal: passed
go test ./...: passed
PowerShell parse for scripts/smoke-openrouter-flow-local.ps1: passed
scripts/smoke-openrouter-flow-local.ps1 -SecretsFile PATH: passed
usage ledger unit/API tests: passed
scripts/smoke-control-plane-ux-local.ps1 -SkipImageBuild: passed
API workspace list in control-plane smoke: passed
API usage record/summary in control-plane smoke: passed
stola workspace create/status/exec/delete in smoke: passed
OpenRouter provider check: passed
OpenCode OpenRouter call: returned STOLA_OK
Hermes OpenRouter call: returned STOLA_OK
OpenClaw OpenRouter call: returned STOLA_OK
LiteLLM OpenRouter call: returned STOLA_OK
MCP filesystem tools/calls: passed
one-shot approval create/consume/reuse-deny: passed
audit no-leak check: passed
Docker workspace cleanup: passed
repo secret prefix search: absent
PowerShell history secret prefix search: absent
```

### Remaining Work

- Implement real LiteLLM proxy mode with virtual keys, budgets, rate limits,
  spend tracking, and persistent cost data.
- Wire LiteLLM/OpenClaw/OpenCode/Hermes runtime output into the usage ledger
  automatically instead of relying on explicit `POST /v1/usage`.
- Add secret policy per user/workspace/provider/budget.
- Add transcript-level stdout/stderr leak scan to the OpenRouter smoke.
- Keep hosted/untrusted mode blocked until gVisor/Kata/KVM isolation is
  designed and tested.

## 2026-06-24 - Stolas Controlled LLM Proxy V0

### Intent

Put a Stolas-controlled LLM proxy in front of providers so workspaces can use
virtual keys, budget checks, rate limits, and the Stolas usage ledger instead of
calling provider keys directly.

### Work Completed

- Added SQLite `llm_proxy_keys` with SHA-256 key hashes, workspace/user/provider
  scope, allowed models, budget USD, RPM, status, expiration, and last-used time.
- Added `key_id` to usage events so spend can be summarized per virtual key.
- Added API permissions `llm.key.create` and `llm.key.read`.
- Added `POST /v1/llm/keys` and `GET /v1/llm/keys`; raw token is returned only
  once at creation.
- Added `stola daemon llm-proxy`, a localhost OpenAI-compatible proxy that:
  - validates Stolas virtual keys;
  - checks provider/model scope;
  - enforces budget and RPM before upstream calls;
  - forwards to OpenAI-compatible providers;
  - records tokens/cost/latency/status into the Stolas ledger;
  - denies streaming in v0 to avoid untracked usage.
- Added `packaging/systemd/stola-llm-proxy.service`.
- Added `scripts/smoke-litellm-proxy-local.ps1`.

### Checks Passed

```text
go test ./...: passed
PowerShell parse for scripts/smoke-litellm-proxy-local.ps1: passed
scripts/smoke-litellm-proxy-local.ps1: passed
fake upstream health: passed
control plane health: passed
llm proxy health: passed
api workspace create: passed
api llm key create: passed
llm proxy chat completion: passed
Stolas usage ledger recorded proxy spend: passed
llm proxy budget denied second call: passed
llm key list shows spend without raw token: passed
audit no-leak check: passed
```

### Remaining Work

- Configure new workspaces so OpenCode/Hermes/OpenClaw/LiteLLM use the Stolas
  LLM proxy virtual key by default instead of direct provider secrets.
- Add live OpenRouter proxy smoke when a fresh test key is available.
- Add cost-map fallback for providers that return tokens but no cost.
- Add safe streaming support with usage accounting.
- Add TPM and concurrency limits per key.
- For hosted multi-user mode, evaluate official LiteLLM proxy with Postgres for
  its native virtual-key budget engine while keeping the Stolas ledger as the
  product source of truth.

## 2026-06-24 - Stolas Proxy Defaults, Streaming, And Key Lifecycle

### Intent

Make the Stolas-controlled proxy the preferred workspace path, close missing
key lifecycle controls, and make streaming/cost tracking safe enough for local
validation.

### Work Completed

- Added TPM and max-concurrency metadata to Stolas virtual keys.
- Added API permissions and endpoints for key rotation/revocation:
  - `POST /v1/llm/keys/{id}/rotate`
  - `POST /v1/llm/keys/{id}/revoke`
- Added proxy enforcement for:
  - budget USD;
  - RPM;
  - estimated TPM;
  - max concurrent requests.
- Added buffered streaming passthrough in `stola daemon llm-proxy`; the proxy
  requests `stream_options.include_usage`, parses SSE chunks, records usage, and
  falls back to token estimates if usage is absent.
- Added minimal model cost fallback for `gpt-4o-mini`/`openai/gpt-4o-mini` when
  upstream returns tokens but omits `cost_usd`.
- Added Docker workspace LLM proxy injection:
  - workspace config at `.stola/llm-proxy/config.json` without token;
  - virtual key env at `/home/stola/.config/stola/llm-proxy.env`;
  - LiteLLM config pointed at the Stolas proxy;
  - profile sourcing for the proxy env file.
- Added `stola workspace create --llm-proxy-url ...` flags to create a virtual
  key and inject proxy defaults without putting provider secrets in the
  workspace.
- Added `scripts/smoke-openrouter-proxy-flow-local.ps1` for live OpenRouter via
  Stolas proxy when a fresh `OPENROUTER_API_KEY` is supplied.
- Expanded `scripts/smoke-litellm-proxy-local.ps1` to cover rotate/revoke,
  revoked-key denial, streaming, fallback spend, and audit no-leak.

### Checks Passed

```text
go test ./...: passed
scripts/smoke-litellm-proxy-local.ps1: passed
PowerShell parser check for scripts/smoke-openrouter-proxy-flow-local.ps1: passed
scripts/smoke-openrouter-proxy-flow-local.ps1 -SecretsFile PATH: passed live
temporary OpenRouter secret file cleanup: passed
OpenRouter secret absent from proxy audit: passed
```

### Remaining Work

- Add a Docker workspace smoke that verifies OpenCode/Hermes/OpenClaw/LiteLLM use
  the injected Stolas proxy env/config instead of direct provider secrets.
- Move the fallback cost map from a minimal hardcoded table to managed metadata
  or official LiteLLM cost data before using it for hosted billing.
- Replace buffered streaming with true pass-through streaming plus final usage
  finalization if low-latency streaming UX becomes required.
- Add virtual-key rotation reinjection for already-running Docker workspaces.
- Evaluate official LiteLLM proxy plus Postgres for hosted multi-user mode.

## 2026-06-25 - Docker Workspace Proxy Smoke Gate

### Intent

Verify the real user path where OpenCode, Hermes, OpenClaw, LiteLLM, and a
plain OpenAI-compatible client use the Stolas LLM proxy injected into a Docker
workspace, without placing the provider secret in the workspace.

### Work Completed

- Added `scripts/smoke-stolas-proxy-docker-workspace-local.ps1`.
- The script builds a local `stolas.exe`, starts a fake OpenAI-compatible
  upstream on `127.0.0.1`, starts `stola daemon llm-proxy` on `127.0.0.1`,
  creates a Docker workspace with `--llm-proxy-url`, and does not pass
  `--secrets-file` to workspace creation.
- The script validates proxy env/config injection:
  - `/workspace/.stola/llm-proxy/config.json`;
  - `/home/stola/.config/stola/llm-proxy.env`;
  - `/home/stola/.config/litellm/config.yaml`;
  - no direct `/home/stola/.config/stola/secrets.env`;
  - no `OPENROUTER_API_KEY` and no fake upstream secret in workspace files.
- The script exercises direct OpenAI-compatible HTTP, LiteLLM, OpenCode, Hermes,
  OpenClaw, MCP config discovery, proxy audit events, and cleanup.
- Fixed LiteLLM proxy config rendering so `openai/gpt-4o-mini` stays
  `openai/gpt-4o-mini` instead of becoming `openai/openai/gpt-4o-mini`.

### Checks Passed

```text
go test ./internal/docker ./cmd/stola ./internal/llmproxy: passed
go test ./...: passed
PowerShell parser check for scripts/smoke-stolas-proxy-docker-workspace-local.ps1: passed
Docker host-to-container localhost bridge experiment with host.docker.internal: passed before Docker engine stopped
```

### Blocked Live Check

```text
scripts/smoke-stolas-proxy-docker-workspace-local.ps1: blocked by local Docker Desktop/WSL
docker version --format "{{.Server.Version}}": timeout
wsl -d docker-desktop -e sh -lc "echo WSL_DOCKER_OK": Wsl/Service/CreateInstance/CreateVm/HCS/0x800705aa
wsl -d Ubuntu -e sh -lc "echo UBUNTU_WSL_OK": Wsl/Service/CreateInstance/CreateVm/HCS/0x800705aa
Start-Service LxssManager: failed from current non-elevated shell
```

### Remaining Work

- Re-run `.\scripts\smoke-stolas-proxy-docker-workspace-local.ps1` after Docker
  Desktop/WSL starts successfully.
- If the smoke passes, make it the default local gate before changing proxy,
  Docker workspace injection, agent config, or MCP config paths.
- Next product step after this gate is stable: add a tiny localhost UX/API flow
  that creates a workspace with provider/proxy selected, shows health/status,
  and exposes logs without requiring manual CLI sequencing.
