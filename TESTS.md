# Stolas Docker Agent Workspace Test Matrix

Status: active - Docker-only agent image, lifecycle, MCP injection, provider catalog, provider check, secret injection, and approval gates passing
Last updated: 2026-06-24

## Required Gates For Docker Agent Workspace v0

Run these before claiming Docker Agent Workspace v0 is complete:

```bash
go mod tidy
gofmt -w cmd internal
go test ./...
```

## Unit Tests

- `internal/config`
  - parses valid minimal config.
  - accepts UTF-8 BOM at the start of config files created by Windows PowerShell.
  - parses Docker config.
  - parses Proxmox config with `token_file` and explicit lab TLS option.
  - rejects missing backend/state/audit config.
  - writes and reloads config.
  - rejects direct Proxmox token config keys.
- `internal/audit`
  - writes JSONL events.
  - redacts token, secret, password, API key, credential, and private key fields.
- `internal/state`
  - migrates SQLite schema.
  - creates and lists mock VPS records.
  - persists Proxmox `external_id` and `task_id`.
  - reads, updates status, and deletes VPS records.
  - writes and reads daemon heartbeats.
  - creates MCP approval intents with normalized argument keys.
  - stores approval `approved_by` and `policy_version` metadata.
  - consumes MCP approval intents exactly once.
  - rejects expired or mismatched MCP approval intents without consuming them.
  - consumes MCP rate limit counters, rejects over-limit calls, and resets after the window.
- `internal/provisioner`
  - mock-create persists a VPS record.
  - mock-create writes an audit event.
- `internal/docker`
  - reads Docker server version and security options.
  - creates per-workspace Docker network and volume.
  - creates a detached workspace container.
  - applies memory, swap, CPU, and PID limits.
  - applies `cap-drop=ALL` and `no-new-privileges`.
  - injects rendered MCP workspace files during Docker create when enabled.
  - injects rendered provider workspace files and native LiteLLM config during Docker create when enabled.
  - injects provider secrets through Docker stdin, not Docker command arguments.
  - writes provider secret values only to `/home/stola/.config/stola/secrets.env` and writes names only to `/workspace/.stola/secrets/manifest.json`.
  - executes MCP stdio servers via `docker exec -i -w /workspace`.
  - maps status/logs/exec/stop/start/delete to Docker CLI commands.
- `internal/mcp`
  - parses the default MCP manifest.
  - rejects duplicate server IDs.
  - renders the Stola allowlist JSON.
  - renders minimal OpenCode, Hermes, and OpenClaw config files.
  - omits stdio servers from OpenClaw config because current OpenClaw config accepts remote transports only.
  - builds newline-delimited MCP JSON-RPC input for `initialize`, `tools/list`, and `tools/call`.
  - parses `tools/list` and `tools/call` responses.
  - returns JSON-RPC errors from MCP servers.
  - validates per-server `tool_policy` actions.
  - resolves per-tool decisions for `allow`, `require_approval`, and `deny`.
  - summarizes arguments/results without exposing raw values in audit details.
  - computes a canonical argument digest for approval binding.
  - validates filesystem MCP path arguments before approval or execution.
  - rejects path traversal, paths outside `/workspace`, Windows host path syntax, `.stola`, agent configs, env files, key files, and secret-looking filenames.
  - denies unknown filesystem MCP tools until their argument schema is explicitly covered.
  - loads `argument_policy` from the MCP manifest.
  - applies `argument_policy` overrides by plan, VPS ID, and user ID.
  - allows manifest-defined schemas for new filesystem tools while still enforcing protected paths.
  - loads and validates root `execution_policy`.
  - evaluates execution policy for server risk, network access, command binary, scoped rules, approval action, and rate limit.
  - denies disallowed network/command before MCP execution.
  - loads a versioned policy overlay outside the base manifest.
  - applies overlay `execution_policy`, `tool_policy`, and `argument_policy` by server.
  - writes rendered bundle files to a workspace directory.
- `internal/providers`
  - parses the default provider manifest.
  - validates provider IDs, kinds, env var names, and default provider.
  - finds providers by ID.
  - renders provider catalog JSON.
  - renders `env.example` without secret values.
  - renders LiteLLM config.
  - renders OpenAI-compatible endpoint map.
  - writes rendered provider bundle files to a workspace directory.
  - checks OpenAI-compatible providers with read-only `GET /models`.
  - checks OpenRouter through `GET /api/v1/models` and optional OpenRouter headers.
  - requires the provider API key from the explicit secret bundle.
  - handles HTTP failures without exposing secret values.
- `internal/secrets`
  - parses `.env` files with comments, quotes, optional `export`, and Windows-created UTF-8 BOM.
  - filters provider secrets by allowed env var names from the provider manifest.
  - rejects secret files with no matching provider variables.
  - renders workspace manifests with secret names only.
  - renders container secret env files with actual values for runtime use.
  - writes `.stola/secrets/manifest.json` without leaking secret values.
- `internal/controlplane`
  - loads users/RBAC from YAML.
  - authenticates Bearer tokens by SHA-256 hash.
  - enforces permissions for policy reload and MCP approvals.
  - signs and verifies one-shot MCP approval tokens with HMAC.
  - creates signed approval tokens through the API and persists approval metadata.
  - signs an existing pending approval intent through `POST /v1/mcp/approvals/{id}/approve`.
- `internal/proxmox`
  - sends `Authorization: PVEAPIToken=...` from `token_file`.
  - reads Proxmox version, nodes, and storage from `/api2/json`.
  - posts LXC Nano create form to `/nodes/{node}/lxc`.
  - rejects missing VMID/template/storage before HTTP create.
- `cmd/stola`
  - `config validate` succeeds on valid config.
  - `doctor` reports mock backend.
  - `mcp validate` validates the default manifest.
  - `mcp list` lists default and optional allowed servers.
  - `mcp render --workspace` writes the generated workspace bundle.
  - `providers validate` validates the default provider catalog.
  - `providers list` lists supported providers without printing secret values.
  - `providers render --workspace` writes the generated provider bundle.
  - `providers check --secrets-file PATH --provider ID` performs a read-only health check without printing secret values.
  - `secrets validate --file PATH` validates a provider secret file without printing values.
  - `secrets list --file PATH` prints allowed secret names only.
  - `workspace create/list` works as the product-facing command surface.
  - `vps create --secrets-file PATH` validates and injects provider secrets for Docker workspaces.
  - `vps ...` remains a temporary compatibility alias.
  - `vps exec` audit details do not keep raw `argv`.
  - `mcp tools --name NAME --server SERVER` lists tools from an allowed stdio MCP server inside a Docker workspace.
  - `mcp call --name NAME --server SERVER --tool TOOL --input JSON|@file` calls an allowed tool with audit.
  - `mcp call` creates an auditable one-shot `approval_id` for tools marked `require_approval`.
  - `mcp call --approval-id ID` consumes a matching approval exactly once.
  - `mcp call --approval-token TOKEN` verifies server signature, user, request binding, and policy version before consuming approval.
  - `mcp call --approval-token @PATH` reads a signed approval token from a local file.
  - `mcp call` denies blocked filesystem paths before creating an `approval_id`.
  - `mcp call` applies `execution_policy` before `tools/list`.
  - `mcp call` persists and enforces MCP rate limits.
  - `mcp validate --policy-file PATH` validates an effective manifest with a policy overlay and prints `policy_version`.
  - `approvals list` lists pending approvals through the local API.
  - `approvals approve --id ID --write-token PATH` signs a pending approval and writes the token to a local file.
  - `POST /v1/usage`, `GET /v1/usage`, and `GET /v1/usage/summary` persist and aggregate usage/cost telemetry.
  - `POST /v1/llm/keys` creates a virtual key and returns the raw token once.
  - `GET /v1/llm/keys` lists key metadata/spend without exposing raw tokens.
  - `daemon llm-proxy` forwards OpenAI-compatible chat completions, records usage, and enforces budget/RPM.
  - `provisioner mock-create` creates a fake VPS.
  - `docker check` reports local Docker runtime details.
  - Docker lifecycle commands are covered by real smoke against Docker Desktop.
  - `proxmox check` reads mocked version/nodes/storage.
  - `vps create` calls the Proxmox LXC create endpoint when backend is Proxmox.
  - `daemon provisioner --once` writes a heartbeat.

## Manual CLI Smoke Tests

Use a temp config path on non-Linux/dev machines:

```bash
stola --config ./tmp/stola.yaml init --force --state ./tmp/stola.db --audit-log ./tmp/audit.jsonl
stola --config ./tmp/stola.yaml config validate
stola --config ./tmp/stola.yaml doctor
stola --config ./tmp/stola.yaml node status
stola --config ./tmp/stola.yaml provisioner mock-create --name nano-a --plan nano
stola --config ./tmp/stola.yaml vps create --name nano-a --plan nano
stola mcp validate
stola mcp list
stola mcp render --workspace ./tmp/mcp-render-smoke
stola providers validate
stola providers list
stola providers render --workspace ./tmp/providers-render-smoke
stola providers check --provider openai --secrets-file ./tmp/provider-secrets.env
stola --config ./tmp/stola.yaml daemon provisioner --once
stola service status
```

Expected:

- Config validates.
- Doctor reports `backend: mock`.
- Node status prints node/config/state/audit/runtime paths.
- Mock VPS is created.
- Generic `vps create` works in mock mode.
- MCP manifest validates, lists allowed servers, and renders the config bundle.
- Provider check works when `./tmp/provider-secrets.env` contains an explicit test key for the selected provider.
- Audit log contains `provisioner.mock_create` without secrets.
- Provisioner daemon writes one heartbeat and exits.
- Systemd unit files are reported as present when run from the repo.

## Control Plane Identity Smoke Tests

The automated unit tests cover the API through `httptest`. For a local daemon smoke, create a non-secret users file from `examples/control-plane-users.example.yaml`, write only the SHA-256 hash of a random token, then run:

```powershell
stola --config ./tmp/stola.yaml mcp validate --manifest ./manifests/mcp/default.yaml --policy-file ./examples/mcp-policy.example.yaml
stola --config ./tmp/stola.yaml daemon api
```

Expected:

- `mcp validate` prints a non-empty `policy_version`.
- `daemon api` requires `api.users_file` and `api.signing_key_file`.
- `GET /healthz` works without auth.
- `GET /v1/policy` requires a Bearer token with `policy.read`.
- `POST /v1/policy/reload` requires `policy.reload`.
- `GET /v1/mcp/approvals` requires `mcp.approve` and lists approval intents.
- `POST /v1/mcp/approvals` requires `mcp.approve` and returns `approval_id` plus signed `approval_token`.
- `POST /v1/mcp/approvals/{id}/approve` signs an existing pending approval generated by the CLI.
- `stola mcp call --approval-token TOKEN` accepts only matching VPS/server/tool/argument digest and current policy version.

Full local UX smoke:

```powershell
.\scripts\smoke-control-plane-ux-local.ps1
```

Expected:

- Builds local `tmp/control-plane-ux-smoke/stola.exe`.
- Generates a temporary users file containing only token SHA-256, not the raw token.
- Starts `stola daemon api` only on `127.0.0.1`.
- Validates `GET /healthz`, `GET /v1/policy`, and `POST /v1/policy/reload`.
- Creates a Docker workspace from `stola-agent-base:dev`.
- Validates Hermes, OpenCode, OpenClaw, `uvx`, workspace configs, and native agent configs.
- Validates provider secrets, injects only allowed provider vars, and confirms unrelated secrets are absent.
- Runs `opencode mcp list`, `hermes mcp list`, and `openclaw config validate && openclaw mcp status && openclaw mcp list`.
- Runs `stola mcp tools` and safe filesystem MCP calls.
- Confirms `write_file` creates a pending approval instead of executing.
- Lists the pending `approval_id` with `stola approvals list`.
- Approves that pending `approval_id` with `stola approvals approve --write-token`.
- Executes `write_file` with signed `--approval-token @PATH`.
- Rejects reuse of the same signed token.
- Denies `.stola` path mutation before approval creation.
- Confirms audit events exist.
- Confirms audit does not include provider secret values, unrelated secret names, or raw `vps exec` argv.
- Stops the local API and cleans the smoke container/network/volume.

## Docker Workspace Smoke Tests

Docker-first is the active MVP runtime.

```powershell
stola --config ./tmp/docker.yaml init --force `
  --backend docker `
  --docker-image debian:bookworm-slim `
  --state ./tmp/docker-stola.db `
  --audit-log ./tmp/docker-audit.jsonl

stola --config ./tmp/docker.yaml config validate
stola --config ./tmp/docker.yaml docker check
stola --config ./tmp/docker.yaml vps create `
  --name smoke-docker `
  --plan nano `
  --image busybox:1.36 `
  --memory 128 `
  --swap 0 `
  --cores 1 `
  --pids-limit 64
```

Expected:

- Docker server version is printed.
- Security options include at least seccomp/cgroup namespace on this Windows Docker Desktop host.
- Container is created as `stola-smoke-docker`.
- Container has `CapDrop=[ALL]` and `SecurityOpt=[no-new-privileges:true]`.
- SQLite record has `backend=docker`, `status=running`, and `external_id=<container id>`.
- Test container, network, and volume are removed after smoke validation.

## Agent Base Image Smoke Tests

Build and verify the actual Stola agent image:

```powershell
.\scripts\smoke-stola-agent-base.ps1
```

Expected:

- Docker builds `stola-agent-base:dev`.
- Smoke container starts with 1 GiB RAM, 1 CPU, PID limit, `cap-drop=ALL`, and `no-new-privileges`.
- `hermes`, `opencode`, `openclaw`, and `uvx` resolve inside the container.
- `/opt/stola/manifests/mcp/default.yaml` exists and contains `stola-default-mcp`.
- Container runs as UID `10001` in `/workspace`.
- Smoke container is removed after validation.

## Docker Agent Lifecycle Smoke Tests

Run after `stola-agent-base:dev` exists:

```powershell
stola --config ./tmp/agent-base-cli-smoke/stola.yaml init --force `
  --backend docker `
  --docker-image stola-agent-base:dev `
  --state ./tmp/agent-base-cli-smoke/stola.db `
  --audit-log ./tmp/agent-base-cli-smoke/audit.jsonl

stola --config ./tmp/agent-base-cli-smoke/stola.yaml vps create `
  --name stola-agent-smoke `
  --plan nano `
  --image stola-agent-base:dev `
  --memory 1024 `
  --swap 0 `
  --cores 1 `
  --pids-limit 256

stola --config ./tmp/agent-base-cli-smoke/stola.yaml vps list
stola --config ./tmp/agent-base-cli-smoke/stola.yaml vps status --name stola-agent-smoke
stola --config ./tmp/agent-base-cli-smoke/stola.yaml vps exec --name stola-agent-smoke -- sh -lc "command -v hermes && command -v opencode && command -v openclaw"
stola --config ./tmp/agent-base-cli-smoke/stola.yaml vps exec --name stola-agent-smoke -- sh -lc "test -f /workspace/.stola/mcp/default.yaml && test -f /workspace/.stola/mcp/allowlist.json && test -f /workspace/opencode.json && test -f /workspace/.stola/mcp/hermes.config.yaml && test -f /workspace/.stola/mcp/openclaw.json"
stola --config ./tmp/agent-base-cli-smoke/stola.yaml vps exec --name stola-agent-smoke -- sh -lc "test -f /home/stola/.config/opencode/opencode.json && test -f /home/stola/.hermes/config.yaml && test -f /home/stola/.openclaw/openclaw.json"
stola --config ./tmp/agent-base-cli-smoke/stola.yaml vps exec --name stola-agent-smoke -- sh -lc "opencode mcp list"
stola --config ./tmp/agent-base-cli-smoke/stola.yaml vps exec --name stola-agent-smoke -- sh -lc "hermes mcp list"
stola --config ./tmp/agent-base-cli-smoke/stola.yaml vps exec --name stola-agent-smoke -- sh -lc "openclaw config validate && openclaw mcp status && openclaw mcp list"
stola --config ./tmp/agent-base-cli-smoke/stola.yaml mcp tools --name stola-agent-smoke --server filesystem --timeout 90s
stola --config ./tmp/agent-base-cli-smoke/stola.yaml mcp call --name stola-agent-smoke --server filesystem --tool list_allowed_directories --input "{}" --timeout 90s
stola --config ./tmp/agent-base-cli-smoke/stola.yaml vps exec --name stola-agent-smoke -- sh -lc "printf 'stola mcp call smoke\n' > /workspace/README.md"
Set-Content -NoNewline -Path ./tmp/read-file.json -Value '{"path":"/workspace/README.md"}'
stola --config ./tmp/agent-base-cli-smoke/stola.yaml mcp call --name stola-agent-smoke --server filesystem --tool read_text_file --input @./tmp/read-file.json --timeout 90s
Set-Content -NoNewline -Path ./tmp/write-file.json -Value '{"path":"/workspace/policy-smoke.txt","content":"approved write\n"}'
$approvalOutput = stola --config ./tmp/agent-base-cli-smoke/stola.yaml mcp call --name stola-agent-smoke --server filesystem --tool write_file --input @./tmp/write-file.json --timeout 90s 2>&1
# Expected to fail and print approval_id/server/tool/argument_keys/argument_digest.
$approvalID = ($approvalOutput | Select-String 'approval_id: (.+)').Matches[0].Groups[1].Value.Trim()
stola --config ./tmp/agent-base-cli-smoke/stola.yaml mcp call --name stola-agent-smoke --server filesystem --tool write_file --input @./tmp/write-file.json --approval-id $approvalID --timeout 90s
stola --config ./tmp/agent-base-cli-smoke/stola.yaml mcp call --name stola-agent-smoke --server filesystem --tool write_file --input @./tmp/write-file.json --approval-id $approvalID --timeout 90s
# Expected to fail because the approval_id is one-shot.
stola --config ./tmp/agent-base-cli-smoke/stola.yaml vps logs --name stola-agent-smoke --tail 20
stola --config ./tmp/agent-base-cli-smoke/stola.yaml vps stop --name stola-agent-smoke
stola --config ./tmp/agent-base-cli-smoke/stola.yaml vps start --name stola-agent-smoke
stola --config ./tmp/agent-base-cli-smoke/stola.yaml vps delete --name stola-agent-smoke
```

Expected:

- `vps create` creates `stola-stola-agent-smoke` from `stola-agent-base:dev`.
- `vps exec` verifies Hermes, OpenCode, OpenClaw, and the MCP manifest inside the workspace.
- MCP bundle is injected into `/workspace` with Stola allowlist plus minimal OpenCode/Hermes/OpenClaw configs.
- Native config files are injected for OpenCode, Hermes, and OpenClaw under `/home/stola`.
- `opencode mcp list` loads the generated config. `git` connects only after `/workspace` is a Git repository.
- `hermes mcp list` loads `mcp_servers` from `/home/stola/.hermes/config.yaml`.
- `openclaw config validate`, `openclaw mcp status`, and `openclaw mcp list` pass with the generated remote-only config.
- `stola mcp tools` lists filesystem MCP tools from inside the workspace.
- `stola mcp call` can call `list_allowed_directories` and `read_text_file`.
- `write_file` without approval prints a one-shot `approval_id` with server/tool/argument keys/digest and does not execute.
- `write_file` succeeds once with the matching `--approval-id`.
- Reusing the same `--approval-id` is denied.
- Audit log contains `mcp.tools`, `mcp.call`, `mcp.approval`, and policy denies without raw argument/result values.
- `vps logs` returns Docker logs without failing.
- `vps stop` changes Docker status to `exited`; `vps start` returns it to `running`.
- `vps delete` removes the container, per-workspace network, per-workspace volume, and SQLite row.

## Stolas Proxy Docker Workspace Smoke Tests

Run after Docker Desktop is healthy and `stola-agent-base:dev` exists:

```powershell
.\scripts\smoke-stolas-proxy-docker-workspace-local.ps1
```

Expected:

- Builds `tmp/stolas-proxy-workspace-smoke/stolas.exe`.
- Starts a fake OpenAI-compatible upstream only on `127.0.0.1`.
- Starts `stola daemon llm-proxy` only on `127.0.0.1`.
- Creates a Docker workspace with `--llm-proxy-url` and without `--secrets-file`.
- Injects `OPENAI_BASE_URL`, virtual `OPENAI_API_KEY`, `STOLAS_LLM_PROXY_KEY`, and native LiteLLM config into the workspace.
- Does not inject provider secrets into the workspace: no `/home/stola/.config/stola/secrets.env`, no `OPENROUTER_API_KEY`, and no fake upstream secret under `/workspace` or `/home/stola/.config`.
- Verifies Hermes, OpenCode, OpenClaw, LiteLLM, and a direct OpenAI-compatible client call through the Stolas proxy.
- Verifies MCP config discovery for OpenCode/Hermes/OpenClaw still works with proxy defaults.
- Confirms proxy audit contains `llm_proxy.call` events without provider secret leakage.
- Cleans the workspace container, network, volume, proxy process, fake upstream process, and temporary files.

## Proxmox Lab Smoke Tests

Run only against an explicit Proxmox lab host:

Preferred PowerShell wrapper:

```powershell
Copy-Item .\examples\proxmox-lab.env.example.ps1 .\tmp\proxmox-lab.env.ps1
# Edit .\tmp\proxmox-lab.env.ps1 with non-secret lab values.
. .\tmp\proxmox-lab.env.ps1
.\scripts\proxmox-bootstrap-token.ps1 -RunCheck
```

If the token already exists and the secret was not saved, rotate it in lab:

```powershell
.\scripts\proxmox-bootstrap-token.ps1 -Rotate -RunCheck
```

Manual token mode:

```powershell
# Put the real token only in .\tmp\pve.token.
.\scripts\proxmox-lab-check.ps1
```

Equivalent raw CLI:

```bash
stola --config ./tmp/pve.yaml init --force \
  --backend proxmox \
  --proxmox-endpoint https://PVE_HOST:8006 \
  --proxmox-node pve \
  --proxmox-token-file ./tmp/pve.token \
  --proxmox-insecure-skip-tls-verify \
  --state ./tmp/pve-stola.db \
  --audit-log ./tmp/pve-audit.jsonl

stola --config ./tmp/pve.yaml proxmox check
stola --config ./tmp/pve.yaml proxmox nodes
stola --config ./tmp/pve.yaml proxmox storage
stola --config ./tmp/pve.yaml vps create \
  --name nano-a \
  --vmid 101 \
  --template local:vztmpl/debian-12-standard_12.7-1_amd64.tar.zst \
  --storage local-lvm \
  --bridge vmbr0
```

Expected:

- API authentication succeeds using the token file.
- Bootstrap script creates/updates lab ACLs through SSH without printing the token secret.
- Version, node, and storage inventory are printed.
- LXC create returns a Proxmox UPID.
- SQLite record has `backend=proxmox`, `external_id=101`, and `status=creating`.
- No token value appears in stdout/stderr/audit log.

## Deferred Tests

These are not part of Docker Agent Workspace v0:

- Real Proxmox LXC delete and cleanup.
- KVM/cloud-init lifecycle.
- Guest OS boot tests.
- LiteLLM virtual key provisioning and per-user budgets.
- Live provider API calls with explicit test keys for OpenRouter/OpenAI/Anthropic/etc.
  - `providers check` exists for OpenAI-compatible model-list checks, but live calls require explicit test keys outside the repo.
- Hermes interactive runtime/provider setup.
- MCP governance and real tool execution.
- Browser sandboxing.
- Dashboard/API browser tests.
- Billing/metering tests.
- Multi-node tests.
- GPU/vLLM/Ollama tests.

## Current Test Status

- Official Go `go1.26.4.windows-amd64.zip` was downloaded to `%TEMP%\stola-go-toolchain` and verified by SHA-256.
- `go mod tidy`: passed.
- `gofmt -w .\cmd .\internal` through `%TEMP%\stola-go-toolchain\go\bin\gofmt.exe`: passed.
- `go test ./...` through `%TEMP%\stola-go-toolchain\go\bin\go.exe`: passed.
- `cmd/stola` workspace alias test: passed; `stola workspace create/list` returns workspace-facing output.
- `internal/controlplane` workspace API test: passed; `POST /v1/workspaces`, `GET /v1/workspaces`, and `GET /v1/workspaces/{id}` enforce RBAC and return workspace views.
- `internal/controlplane` usage API test: passed; `POST /v1/usage`, `GET /v1/usage`, and `GET /v1/usage/summary` enforce RBAC and aggregate OpenRouter usage.
- `internal/state` usage ledger test: passed; records/list/summarizes token/cost/error data and rejects negative counters.
- `internal/state` LLM proxy key test: passed; stores only key hash, lists metadata, touches last-used time, summarizes spend by key, rotates keys, and revokes keys.
- `internal/controlplane` LLM key API test: passed; create/list/rotate/revoke works with RBAC, list does not expose raw token, and key views include budget/RPM/TPM/concurrency metadata.
- `internal/docker` LLM proxy injection test: passed; workspace config omits token, container secret env receives the virtual key through stdin, and Docker command args do not leak it.
- `internal/docker` LiteLLM proxy model rendering test: passed; `openai/gpt-4o-mini` stays single-prefixed and does not render `openai/openai`.
- `internal/llmproxy` tests: passed; chat completion forwarding records usage/spend, second call is denied by budget, disallowed model is denied, TPM is enforced before upstream, streaming SSE is accepted, and token cost fallback records spend when upstream omits `cost_usd`.
- `internal/secrets` unit tests: passed through `go test ./...`.
- `cmd/stola` secret CLI tests: passed through `go test ./...`.
- `stola providers validate --manifest .\manifests\providers\default.yaml`: passed; 26 providers, default `openrouter`.
- `stola providers list --manifest .\manifests\providers\default.yaml`: passed; printed provider metadata/env var names only.
- `stola providers render --manifest .\manifests\providers\default.yaml --workspace .\tmp\providers-render-smoke`: passed; wrote `default.yaml`, `catalog.json`, `env.example`, `litellm.config.yaml`, and `openai-compatible.json`.
- `stola providers check --provider openai --secrets-file PATH`: passed through CLI `httptest`; called `/v1/models`, used Bearer auth, counted models, and did not print the key.
- `providers.CheckProvider` OpenRouter path: passed through `httptest`; called `/api/v1/models`, used Bearer auth plus optional OpenRouter headers, and did not print the key.
- Live OpenRouter provider check: passed with explicit temporary test key; returned HTTP 200, 339 models, 161 ms; local secret file was removed after the run.
- OpenRouter end-to-end local flow: passed with workspace `openrouter-flow`.
- OpenRouter flow `stola providers check`: passed with HTTP 200, 339 models, 225 ms.
- OpenRouter flow Docker workspace create: passed with `stola-agent-base:dev` and `--secrets-file`.
- OpenRouter flow secret injection: passed; `OPENROUTER_API_KEY` was present in the workspace shell, `OPENAI_API_KEY` was absent, and `/workspace/.stola/secrets/manifest.json` did not contain the secret value.
- OpenRouter flow native config: passed; `/home/stola/.config/litellm/config.yaml` existed and contained OpenRouter config.
- OpenRouter flow in-container provider call: passed; `GET https://openrouter.ai/api/v1/models` from inside the workspace returned HTTP 200 and 339 models using the injected secret.
- OpenRouter flow agents: passed; `hermes`, `opencode`, `openclaw`, `uvx`, and `litellm` resolved, `opencode mcp list` connected enabled MCPs, `hermes mcp list` loaded servers, and `openclaw config validate && openclaw mcp status && openclaw mcp list` passed.
- OpenRouter user-like agent calls: passed; `opencode run -m openrouter/openai/gpt-4o-mini`, `hermes chat --query --provider openrouter --model openai/gpt-4o-mini`, `openclaw agent --local --model openrouter/openai/gpt-4o-mini`, and `litellm.completion(model="openrouter/openai/gpt-4o-mini")` all returned `STOLA_OK`.
- OpenRouter flow MCP: passed; `stola mcp tools`, `list_allowed_directories`, `read_text_file`, `write_file` with one-shot approval, approved file verification, and approval reuse denial all passed.
- OpenRouter flow audit: passed; audit contained expected `secrets.inject`, `mcp.call`, and `mcp.approval` events without secret prefix, wrong provider secret name, or raw `argv`.
- `scripts/smoke-openrouter-flow-local.ps1 -SecretsFile PATH`: passed end to end with `stola workspace create/status/exec/delete`, OpenRouter provider check, secret injection, OpenCode, Hermes, OpenClaw, LiteLLM, MCP tools/calls, one-shot approval, audit no-leak check, and cleanup.
- PowerShell parser check for `scripts/smoke-openrouter-flow-local.ps1`: passed.
- MCP filesystem package pin: passed after changing the manifest to `@modelcontextprotocol/server-filesystem@2025.8.21`; unpinned latest failed under Node 24.
- OpenRouter flow cleanup: passed; no `stola-openrouter-flow` container, `stola-net-openrouter-flow` network, or `stola-workspace-openrouter-flow` volume remained, and no secret prefix was found in workspace files.
- OpenRouter user-like flow cleanup: passed; no `stola-openrouter-user-flow` container, `stola-net-openrouter-user-flow` network, or `stola-workspace-openrouter-user-flow` volume remained, and no secret prefix was found in workspace files.
- Real Docker `stola docker check`: passed.
- Real Docker `stola vps create`: passed with `busybox:1.36` and cleanup.
- Real `stola-agent-base:dev` Docker build: passed.
- `scripts/smoke-stola-agent-base.ps1`: passed with `litellm` command and default providers manifest checks.
- Real Docker `stola vps create --image stola-agent-base:dev`: passed.
- Real Docker provider injection smoke with `stola-agent-base:dev`: passed; verified `/workspace/.stola/providers/*` and `/home/stola/.config/litellm/config.yaml`; cleanup removed container/network/volume.
- Real Docker lifecycle `list/status/logs/exec/stop/start/delete`: passed with cleanup.
- `stola mcp validate`: passed.
- `stola mcp list`: passed.
- `stola mcp render --workspace ./tmp/mcp-render-smoke`: passed.
- Real Docker MCP injection smoke with `busybox:1.36`: passed with cleanup.
- Real Docker native MCP smoke with `stola-agent-base:dev`: passed with cleanup.
- `opencode mcp list`: passed; filesystem/fetch/memory/time connected immediately; git connected after `git init` in `/workspace`; context7/github stayed disabled.
- `hermes mcp list`: passed; loaded fetch/filesystem/git/memory/time from `mcp_servers`.
- `openclaw config validate && openclaw mcp status && openclaw mcp list`: passed; generated config is valid and lists disabled Context7.
- `stola mcp tools --server filesystem`: passed; listed 14 filesystem MCP tools.
- `stola mcp call --tool list_allowed_directories`: passed; returned `/workspace`.
- `stola mcp call --tool read_text_file --input @file`: passed; returned controlled smoke file content.
- `stola mcp call --tool write_file --input @file`: created a one-shot approval intent and did not execute.
- `stola mcp call --tool write_file --input @file --approval-id ID`: passed once and wrote controlled smoke file.
- Reusing the same `--approval-id`: denied as expected with `already used`.
- `mcp.approval` audit: passed; wrote `not_required`, `required`, `approved`, and `denied` outcomes with `approval_id` and `argument_digest`.
- `stola mcp call --tool write_file --input {"path":"/etc/passwd"}`: denied by `mcp.argument_policy` before approval and did not print `approval_id`.
- `stola mcp call --tool write_file --input {"path":"/workspace/.stola/mcp/default.yaml"}`: denied by `mcp.argument_policy` before approval and did not print `approval_id`.
- Valid `/workspace/path-policy-ok.txt` write still used one-shot approval and succeeded once.
- Configurable policy smoke: custom manifest override for plan `trusted` allowed `/workspace/opencode.json` to reach one-shot approval and execute once.
- Configurable policy smoke: the same plan override still denied `/workspace/.stola/mcp/default.yaml` before approval.
- Execution policy smoke: custom manifest with `max_calls: 1` allowed the first `list_allowed_directories` call and denied the second via `mcp.rate_limit`.
- Execution policy smoke: custom manifest denying command bin `npx` denied filesystem MCP before execution via `mcp.policy`.
- Control plane unit tests: RBAC auth, approval token HMAC sign/verify, API approval creation, and persisted approval metadata passed.
- Control plane approve-existing unit test: `POST /v1/mcp/approvals/{id}/approve` signs a pending approval and persists metadata.
- Local control-plane UX smoke: `.\scripts\smoke-control-plane-ux-local.ps1 -SkipImageBuild` passed with `stola workspace ...`, API workspace list, API usage record/summary, provider files, native LiteLLM config, provider secret injection, `stola approvals list`, `stola approvals approve --write-token`, and `stola mcp call --approval-token @PATH`.
- Stolas LLM proxy smoke: `.\scripts\smoke-litellm-proxy-local.ps1` passed with fake upstream, API workspace create, API LLM key create, `stola daemon llm-proxy`, chat completion, usage ledger spend, budget exceeded, key list no raw token, key rotate/revoke, revoked-key denial, streaming SSE, fallback spend, and audit no-leak.
- PowerShell parser check for `scripts/smoke-openrouter-proxy-flow-local.ps1`: passed.
- Live OpenRouter proxy smoke: `.\scripts\smoke-openrouter-proxy-flow-local.ps1 -SecretsFile PATH` passed with authorized temporary OpenRouter key; build/config, control-plane health, llm-proxy health, API workspace/key create, live OpenRouter chat through Stolas proxy, usage ledger, and audit no-leak all passed. Temporary secret file was removed after the run.
- Stolas proxy Docker workspace smoke script: `scripts/smoke-stolas-proxy-docker-workspace-local.ps1` was created and passed PowerShell parser validation.
- Stolas proxy Docker workspace smoke live run: blocked by local Docker Desktop/WSL engine startup. `docker version --format "{{.Server.Version}}"` timed out, both `wsl -d docker-desktop` and `wsl -d Ubuntu` failed with `Wsl/Service/CreateInstance/CreateVm/HCS/0x800705aa`, and `LxssManager` could not be started from the current non-elevated shell. This is an environment blocker, not a Go/test failure.
- Control-plane UX smoke secret checks: passed; `OPENAI_API_KEY` existed inside the workspace shell, `UNRELATED_SECRET` was absent, workspace secret manifest contained names only, and audit did not include the fake secret value, unrelated secret name, or raw `argv`.
- Post-smoke cleanup check: API on `127.0.0.1:18750` stopped; no `stola-cp-ux-smoke` container/network/volume remained.
- Local smoke secret cleanup: temporary users file, control-plane signing key, approval token, and provider secret file removed after the run.
- Policy overlay unit test: effective policy applies root `execution_policy` and per-server `tool_policy`.
- `stola mcp validate --manifest .\manifests\mcp\default.yaml --policy-file .\examples\mcp-policy.example.yaml`: passed and printed `policy_version=1c7f2bb1ef23cd92`.
- `stola mcp tools --server context7`: denied as expected because Context7 is optional/disabled; `mcp.policy` audit event written.
- Audit redaction/summarization for MCP calls: passed; audit stored argument keys/bytes and result summary, not raw path/content.
- `scripts/proxmox-bootstrap-token.ps1` PowerShell parse check: passed.
- `scripts/proxmox-bootstrap-token.ps1` placeholder guard: passed.
- `scripts/proxmox-lab-check.ps1` PowerShell parse check: passed.
- `scripts/proxmox-lab-check.ps1` empty-token guard: passed.
- Manual CLI smoke test: passed in mock mode.
- Docker adapter tests: passed.
- Proxmox adapter and CLI integration: passed against `httptest`.
- Live OpenAI provider check: not run; no real OpenAI test key was provided.
- Live Proxmox lab check/create: not run; local `tmp/pve.token` exists but is empty, and no lab endpoint was provided.

Latest command output:

```text
ok  	stola-os/cmd/stola	(cached)
ok  	stola-os/internal/audit	(cached)
ok  	stola-os/internal/config	(cached)
ok  	stola-os/internal/controlplane	(cached)
ok  	stola-os/internal/docker	(cached)
ok  	stola-os/internal/mcp	(cached)
ok  	stola-os/internal/providers	(cached)
ok  	stola-os/internal/provisioner	(cached)
ok  	stola-os/internal/proxmox	(cached)
ok  	stola-os/internal/secrets	(cached)
ok  	stola-os/internal/state	(cached)
?   	stola-os/internal/version	[no test files]
```
