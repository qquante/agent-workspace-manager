# ADR-008: Provider Catalog And LiteLLM Config

Status: accepted, amended 2026-06-24

Stola workspaces will receive a generated provider catalog and LiteLLM/OpenAI-
compatible configuration by default.

Context:

- Users should be able to choose OpenRouter, OpenAI, Anthropic, Gemini, Bedrock,
  Groq, Mistral, local OpenAI-compatible endpoints, and other providers without
  rebuilding the workspace image.
- Provider keys must not be committed to manifests or logs.
- LiteLLM gives a common routing/config layer for many providers and can later
  become a central proxy with virtual keys, budgets, and spend tracking.
- Official LiteLLM proxy key management expects a database-backed deployment for
  virtual keys and budgets. For the local Docker MVP, Stolas needs a smaller
  localhost path that uses its own SQLite ledger and RBAC first.

Decision:

- Keep provider metadata in `manifests/providers/default.yaml`.
- Render workspace files under `.stola/providers/`:
  - `default.yaml`
  - `catalog.json`
  - `env.example`
  - `litellm.config.yaml`
  - `openai-compatible.json`
- Write native LiteLLM config to
  `/home/stola/.config/litellm/config.yaml` during Docker workspace creation.
- Store only environment variable names in manifests and generated examples.
  Real secrets stay in the runtime secret store or process environment.
- Add `stola daemon llm-proxy` as the local Stolas-controlled,
  LiteLLM-compatible/OpenAI-compatible proxy.
- Create Stolas virtual keys through the control-plane API, store only SHA-256
  hashes, enforce budget/RPM in the proxy, and write spend/tokens/latency/errors
  into the Stolas usage ledger.

Consequences:

- Provider support becomes configurable data instead of hardcoded CLI behavior.
- The first implementation validates config generation, not live calls to every
  provider.
- Workspaces can move from direct provider secrets to Stolas virtual keys without
  changing the OpenAI-compatible client shape.
- Later hosted work can still evaluate the official LiteLLM proxy with Postgres,
  but the Stolas ledger remains the product source of truth for spend and audit.
- Later work must add live provider proxy smokes, cost-map fallback when
  upstreams do not report cost, streaming accounting, TPM/concurrency limits, and
  provider cost reconciliation.
