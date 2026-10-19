# Stolas Docker Agent Workspace Goals

Status: active - Docker-only agent image, workspace lifecycle, MCP injection, approval flow, provider catalog, local secret injection, OpenRouter smoke gate, workspace API, and Stolas-controlled LLM proxy passing
Last updated: 2026-06-24

## Current Goal

Build Stolas Agent Workspace v0 as a Docker-only local control layer: Go CLI, local config, SQLite state, JSONL audit log, Docker backend for local workspaces, `stola-agent-base` with Hermes/OpenCode/OpenClaw, MCP manager/injection, provider catalog/LiteLLM config, per-workspace provider secret injection, read-only provider checks, Docker lifecycle commands, control-plane identity/RBAC/API for workspaces and MCP approvals, and Proxmox/KVM kept only as future stronger-isolation backend work.

## Success Criteria

- `stola doctor` works without a Proxmox host and reports mock mode as healthy.
- `stola init` creates a safe default config.
- `stola config validate` accepts valid config and rejects config missing backend/state/audit paths.
- `stola node status` reports node ID, backend, config path, state path, audit path, runtime path, and version.
- `stola provisioner mock-create` creates a fake VPS record in SQLite and writes an audit event.
- `stola docker check` validates local Docker availability and reports runtime security options.
- `stola workspace create` can create a Docker-backed Stolas workspace with network, volume, resource limits, and baseline hardening.
- `stola workspace list/status/logs/exec/stop/start/delete` works for Docker-backed workspaces.
- `stola vps ...` remains available as a temporary legacy alias.
- `stola-agent-base:dev` builds with Hermes, OpenCode, OpenClaw, `uvx`, and the default MCP manifest.
- `stola mcp validate/list/render` can validate the default manifest, list allowed servers, and render workspace configs.
- `stola providers validate/list/render` can validate the default provider catalog, list supported providers without secrets, and render workspace/provider configs.
- `stola providers check --secrets-file PATH --provider ID` performs a read-only model-list health check for OpenAI-compatible providers without printing secret values.
- `stola secrets validate/list --file PATH` validates a provider secret env file, filters it against the provider catalog, and never prints secret values.
- `stola workspace create --image stola-agent-base:dev` creates a usable Docker-backed agent workspace.
- Docker-backed `stola vps create` injects `.stola/mcp/default.yaml`, `.stola/mcp/allowlist.json`, `opencode.json`, `.stola/mcp/hermes.config.yaml`, and `.stola/mcp/openclaw.json` into `/workspace`.
- Docker-backed `stola vps create` injects native config files for OpenCode, Hermes, and OpenClaw under `/home/stola`.
- Docker-backed `stola vps create` injects `.stola/providers/default.yaml`, `.stola/providers/catalog.json`, `.stola/providers/env.example`, `.stola/providers/litellm.config.yaml`, and `.stola/providers/openai-compatible.json` into `/workspace`.
- Docker-backed `stola vps create` injects native LiteLLM config under `/home/stola/.config/litellm/config.yaml`.
- Docker-backed `stola vps create --secrets-file PATH` injects provider secrets into `/home/stola/.config/stola/secrets.env`, writes only secret names to `/workspace/.stola/secrets/manifest.json`, and excludes variables not declared by the provider catalog.
- Audit logs do not contain provider secret values, unrelated secret names, raw secret file paths, or raw `vps exec` argv payloads.
- `opencode mcp list`, `hermes mcp list`, and `openclaw mcp status` can read the generated config inside `stola-agent-base:dev`.
- `stola mcp tools` can list tools from an allowed stdio MCP server inside a Docker workspace.
- `stola mcp call` can execute an allowed MCP tool inside a Docker workspace and write an audit event.
- `stola mcp call` blocks tools marked `require_approval`, creates an auditable one-shot `approval_id`, and only executes when a matching unused `--approval-id` is supplied.
- `stola mcp call` denies unsafe filesystem paths before approval creation, including paths outside `/workspace`, `.stola`, agent configs, env files, key files, and secret-looking filenames.
- Filesystem MCP argument policy is configurable in the MCP manifest and supports overrides by VPS ID, plan, and policy user ID.
- MCP execution policy is configurable in the MCP manifest and controls server risk, network class, command binary, rule-based approval, and persistent rate limits.
- MCP policy can be overlaid from a versioned policy file outside the base manifest.
- `stola daemon api` exposes authenticated control-plane endpoints for policy read/reload and signed MCP approval creation.
- `stola daemon api` exposes authenticated `GET /v1/workspaces`, `GET /v1/workspaces/{id}`, and `POST /v1/workspaces`.
- `stola daemon api` exposes authenticated usage ledger endpoints for workspace/user/provider/model tokens, cost, latency, and errors.
- `stola daemon llm-proxy` exposes a localhost OpenAI-compatible proxy controlled by Stolas virtual keys.
- Stolas virtual keys are stored as SHA-256 hashes, expose the raw token only on creation/rotation, and support provider/model scope, budget USD, RPM, TPM, max concurrency, revocation, and rotation.
- LLM proxy requests record spend/tokens/latency/errors into the Stolas usage ledger and enforce budget/RPM/TPM/concurrency before upstream calls.
- Docker workspaces can be injected with Stolas proxy defaults (`OPENAI_BASE_URL`, virtual `OPENAI_API_KEY`, LiteLLM config) instead of direct provider secrets.
- LLM proxy supports buffered streaming passthrough with usage accounting and a minimal cost-map fallback when upstream omits `cost_usd`.
- Control-plane API authenticates Bearer tokens by SHA-256 hash and enforces RBAC permissions.
- Signed `approval_token` values bind approval ID, user, VPS, server, tool, argument digest, expiration, and policy version.
- Control-plane API can sign an existing pending CLI approval intent through `POST /v1/mcp/approvals/{id}/approve`.
- `stola mcp call --approval-token TOKEN` verifies the server signature and binding before consuming a one-shot approval.
- `stola approvals list` and `stola approvals approve --write-token PATH` let a local operator approve pending MCP tool calls without manually copying approval tokens.
- `stola mcp call --approval-token @PATH` reads the signed token from a local file.
- MCP audit records argument keys/size and result summary, not raw argument or result payloads.
- `stola proxmox check` authenticates via `token_file`, reads Proxmox version, nodes, and configured-node storage.
- `stola proxmox nodes` and `stola proxmox storage` read node/storage inventory.
- `stola vps create` can use the current provisioner interface to request an LXC Nano through Proxmox, but this is no longer the active MVP gate.
- Proxmox-created VPS records persist `external_id` and Proxmox task ID in SQLite.
- `stola daemon provisioner --once` writes a heartbeat and exits cleanly.
- systemd unit files exist for `stola-api`, `stola-provisioner`, and `stola-auditd`.
- tests cover config, state, audit redaction, mock provisioning, Proxmox HTTP adapter behavior, and CLI flows.

## In Scope Now

- Docker-only Agent Workspace core only. No ISO, no installer, no new OS distribution in this MVP.
- Go implementation.
- Mock provisioner backend.
- Initial Docker backend behind `provisioner.Backend`.
- Docker workspace create with per-workspace network and volume.
- Docker resource controls for RAM, swap, CPU, and PID limits.
- Docker runtime hardening with `cap-drop=ALL` and `no-new-privileges`.
- `stola-agent-base` Docker image.
- Hermes, OpenCode, OpenClaw, and `uvx` installed in the agent base image.
- Default MCP allowlist manifest for workspace filesystem, git, fetch, time, memory, and optional Context7/GitHub.
- Default provider catalog for OpenRouter, OpenAI, Anthropic, Gemini, Azure OpenAI, Bedrock, Groq, Mistral, Cohere, DeepSeek, xAI, Together, Fireworks, Perplexity, Cerebras, SambaNova, Replicate, Hugging Face, Cloudflare, NVIDIA NIM, Vercel AI Gateway, LiteLLM Proxy, Ollama, vLLM, LM Studio, and custom OpenAI-compatible endpoints.
- Provider secret env-file validation, filtering, and Docker workspace injection for allowed provider variables.
- MCP manager for manifest validation, allowed-server listing, config rendering, and Docker workspace injection.
- Provider manager for catalog validation, provider listing, config rendering, and Docker workspace injection.
- Read-only provider health check for OpenAI-compatible providers, starting with OpenRouter/OpenAI.
- Minimal MCP host config generation for OpenCode, Hermes, and OpenClaw.
- Minimal provider config generation for LiteLLM and OpenAI-compatible clients.
- Native MCP config injection for the installed agent tools in `stola-agent-base`.
- Direct MCP stdio execution for enabled manifest servers in Docker workspaces.
- Per-tool MCP policy with `allow`, `require_approval`, and `deny`.
- One-shot CLI approval IDs for risky tool calls, persisted in SQLite and bound to VPS/server/tool/argument digest.
- Configurable filesystem MCP argument path policy before approval/execution.
- Configurable MCP execution policy for risk/network/command/rules/rate limits.
- Versioned MCP policy overlay outside the base manifest.
- Control-plane API identity with users file, role permissions, policy reload, and signed MCP approvals.
- Usage/cost ledger in SQLite with API endpoints for record, list, and summary.
- Audit events for MCP discovery, calls, and policy denies.
- Docker lifecycle commands: list, status, logs, exec, stop, start, delete.
- Initial Proxmox REST adapter remains in the tree, but is no longer an MVP gate.
- Local single-node SQLite state.
- JSONL audit logging with secret redaction.
- Minimal `.deb` and systemd packaging skeleton.
- Architecture docs and ADRs for the v0 decisions.

## Out Of Scope For This Goal

- OS/ISO/installer/host appliance delivery.
- Full Proxmox lifecycle beyond initial LXC Nano create.
- Real Proxmox lab validation; Proxmox is now future/premium backend work.
- KVM creation.
- Dashboard/frontend.
- Guest OS image build pipeline.
- Full live provider API validation across every catalog provider.
- Full live completion/chat smoke across every catalog provider. OpenRouter is the reference live provider path for now.
- Official LiteLLM proxy with Postgres-backed key management. Current MVP uses the Stolas-controlled LiteLLM-compatible/OpenAI-compatible proxy and Stolas SQLite ledger.
- Full provider-wide cost-map maintenance. Current fallback is intentionally minimal and should move to official LiteLLM cost metadata or a managed catalog before hosted billing.
- Interactive Hermes runtime configuration.
- MCP marketplace or dynamic MCP installation.
- Billing.
- GPU/vLLM/Ollama local inference.
- Multi-node cluster.

## Current Blockers

- Docker live smoke for the new Stolas proxy workspace path is blocked by the local Windows WSL/Docker Desktop engine failing to start.
- Current observed error: `Wsl/Service/CreateInstance/CreateVm/HCS/0x800705aa`; `docker version` times out and `wsl -d docker-desktop`/`wsl -d Ubuntu` fail before Docker can create containers.
- Real Docker `vps create` passed locally with both `busybox:1.36` and `stola-agent-base:dev`.
- Proxmox lab validation is parked for a future backend and is not a current blocker.
- The Windows environment still has no global `go` binary in `PATH`; validation currently uses a portable Go toolchain in `%TEMP%\stola-go-toolchain`.

## Latest Result

- `go mod tidy`: passed.
- `gofmt -w .\cmd .\internal` through `%TEMP%\stola-go-toolchain\go\bin\gofmt.exe`: passed.
- `go test ./...` through `%TEMP%\stola-go-toolchain\go\bin\go.exe`: passed.
- `cmd/stola` workspace alias test: passed; `stola workspace create/list` uses workspace-facing wording.
- Control-plane workspace API test: passed; `POST /v1/workspaces`, `GET /v1/workspaces`, and `GET /v1/workspaces/{id}` work with RBAC permissions `workspace.create`/`workspace.read`.
- Usage ledger tests: passed; `POST /v1/usage`, `GET /v1/usage`, and `GET /v1/usage/summary` store and aggregate workspace/user/provider/model cost, tokens, latency, and errors.
- LLM proxy tests: passed; virtual key auth, provider forwarding, usage recording, budget denial, disallowed model denial, TPM denial, streaming usage accounting, and cost fallback are covered.
- Docker proxy injection/model rendering tests: passed; proxy virtual key is injected through stdin, workspace config omits token, Docker args do not leak token, and LiteLLM model names do not get double-prefixed.
- `.\scripts\smoke-litellm-proxy-local.ps1`: passed with fake upstream, API workspace create, API LLM key create, proxy chat completion, usage ledger spend, budget exceeded, key list no raw token, key rotate/revoke, revoked-key denial, streaming SSE, fallback spend, and audit no-leak.
- `scripts/smoke-openrouter-proxy-flow-local.ps1`: passed live with authorized temporary OpenRouter key; API workspace/key create, live OpenRouter chat through Stolas proxy, usage ledger, audit no-leak, and temporary secret cleanup passed.
- `scripts/smoke-stolas-proxy-docker-workspace-local.ps1`: created and PowerShell parser check passed; live Docker run is blocked by the local WSL/Docker Desktop startup failure above.
- `stola secrets validate --file <provider.env>`: passed in the control-plane smoke, selecting only provider-catalog secrets.
- `stola secrets list --file <provider.env>`: covered by CLI tests; prints secret names only, no values.
- `stola providers validate --manifest .\manifests\providers\default.yaml`: passed with 26 providers and default `openrouter`.
- `stola providers list --manifest .\manifests\providers\default.yaml`: passed; printed provider IDs/env var names/base URLs only, no secret values.
- `stola providers render --manifest .\manifests\providers\default.yaml --workspace .\tmp\providers-render-smoke`: passed; wrote 5 provider config files.
- `stola providers check --secrets-file PATH --provider openai`: passed in CLI test against `httptest`, using `GET /v1/models` and Bearer token without printing the key.
- `providers.CheckProvider` OpenRouter test: passed against `httptest`, using `GET /api/v1/models` and optional OpenRouter headers without printing the key.
- Live `stola providers check --provider openrouter --secrets-file .\tmp\provider-check-openrouter.env --timeout 20s`: passed with HTTP 200, 339 models, 161 ms; temporary secret file removed after the run.
- OpenRouter end-to-end local flow: passed with Docker workspace `openrouter-flow`, provider check HTTP 200/339 models, secret injection, native LiteLLM config, OpenCode/Hermes/OpenClaw checks, OpenRouter read-only API call from inside the workspace, MCP tools, MCP read/write, one-shot approval, audit no-leak check, and Docker cleanup.
- OpenRouter user-like agent flow: passed with Docker workspace `openrouter-user-flow`; `opencode run`, `hermes chat --query`, `openclaw agent --local`, and `litellm.completion` each called OpenRouter and returned `STOLA_OK`.
- OpenRouter user-like MCP flow: passed with `stola mcp tools`, `list_allowed_directories`, `read_text_file`, `write_file` approval intent, approved write, and approval reuse denial.
- `go test ./...` after the OpenRouter user-like flow: passed.
- `.\scripts\smoke-openrouter-flow-local.ps1 -SecretsFile PATH`: passed end to end with OpenRouter, `stola workspace create/status/exec/delete`, OpenCode, Hermes, OpenClaw, LiteLLM, MCP discovery/call, one-shot approval, audit no-leak check, and cleanup.
- MCP filesystem server was pinned to `@modelcontextprotocol/server-filesystem@2025.8.21` after `latest` broke under Node 24.
- `scripts/smoke-stola-agent-base.ps1`: passed after adding `litellm` and provider manifest checks.
- Real Docker provider injection smoke with `stola-agent-base:dev`: passed; verified `/workspace/.stola/providers/*` and `/home/stola/.config/litellm/config.yaml`; cleanup removed container/network/volume.
- `go test ./internal/controlplane`: passed through `go test ./...`.
- `stola mcp validate --manifest .\manifests\mcp\default.yaml --policy-file .\examples\mcp-policy.example.yaml`: passed with `policy_version=1c7f2bb1ef23cd92`.
- `.\scripts\smoke-control-plane-ux-local.ps1 -SkipImageBuild`: passed, including workspace command surface, API workspace list, usage record/summary API, provider workspace files, native LiteLLM config, provider secret injection, `UNRELATED_SECRET` exclusion, and audit no-leak checks.
- Post-smoke cleanup check: API on `127.0.0.1:18750` stopped; no smoke container/network/volume remained; temporary users file, signing key, approval token, and provider secret file were removed.
- `stola mcp validate`: passed.
- `stola mcp list`: passed.
- `stola mcp render --workspace ./tmp/mcp-render-smoke`: passed.
- Manual CLI smoke from `TESTS.md`: passed in mock mode.
- Docker backend tests passed.
- Real local Docker smoke passed for `stola docker check`.
- Real local Docker smoke passed for `stola vps create` using `busybox:1.36`; cleanup removed the test container/network/volume.
- Real local `stola-agent-base:dev` build and smoke passed.
- Real local `stola vps create --image stola-agent-base:dev` passed.
- Real local Docker lifecycle smoke passed for `vps list/status/logs/exec/stop/start/delete`; cleanup removed container, network, volume, and state row.
- Real local Docker MCP injection smoke passed with `busybox:1.36`; `/workspace` received Stola allowlist plus OpenCode/Hermes/OpenClaw configs; cleanup removed container, network, volume, and state row.
- Real local native MCP smoke passed with `stola-agent-base:dev`.
- OpenCode loaded 7 MCP entries; filesystem/fetch/memory/time connected immediately, git connected after `git init`, and context7/github remained disabled.
- Hermes loaded the generated `mcp_servers` from `/home/stola/.hermes/config.yaml`.
- OpenClaw validated `/home/stola/.openclaw/openclaw.json` and listed disabled Context7; stdio MCPs are intentionally omitted from OpenClaw config because current OpenClaw accepts remote MCP transports only.
- Real local MCP execution smoke passed with `stola mcp tools --server filesystem`.
- Real local MCP tool calls passed for `list_allowed_directories` and `read_text_file`.
- Real local one-shot approval smoke passed: `write_file` created an approval intent without executing, succeeded once with matching `--approval-id`, and rejected reuse of the same ID.
- Real local path policy smoke passed: `/etc/passwd` and `/workspace/.stola/mcp/default.yaml` were denied before approval and did not print `approval_id`; a valid `/workspace/path-policy-ok.txt` write still used one-shot approval and succeeded.
- Real local configurable policy smoke passed: custom manifest override for plan `trusted` allowed `/workspace/opencode.json` to reach one-shot approval and execute once, while `.stola` stayed denied before approval.
- Real local execution policy smoke passed: custom rate limit denied the second call via `mcp.rate_limit`, and command policy denied `npx` via `mcp.policy`.
- Control-plane RBAC/API tests passed: Bearer token SHA-256 auth, role permissions, HMAC approval token signing/verification, API approval creation, and persisted `approved_by`/`policy_version`.
- Control-plane local UX smoke passed end to end: API localhost, policy reload, Docker workspace, Hermes/OpenCode/OpenClaw checks, MCP tools/calls, pending approval, `stola approvals list/approve`, signed token file execution, token reuse denial, blocked path denial, and audit event checks.
- MCP policy overlay tests passed: effective manifest applies overlay `execution_policy` and per-server tool policy without editing the base manifest.
- Optional disabled server execution was denied and logged through `mcp.policy`.
- Proxmox adapter/CLI tests passed against `httptest`; no live Proxmox create was run.
- Lab helper script `scripts/proxmox-lab-check.ps1` was created and passes PowerShell parse plus empty-token guard checks.
- Token bootstrap script `scripts/proxmox-bootstrap-token.ps1` was created and passes PowerShell parse plus placeholder guard checks.
