# Stolas Docker Agent Workspace - Technical Task Backlog

Status: Stolas Docker Agent Workspace v0 com MCP manager, approval flow, provider catalog, secret injection, OpenRouter gate e API inicial passando
Escopo: nao fazer ISO/installer/OS neste MVP. O produto ativo e o Stolas control plane local para workspaces Docker com agentes, MCP, policy, audit e providers LLM. Proxmox/KVM fica como backend futuro/premium de isolamento forte.
Data-base da pesquisa: 2026-06-24.

## Current Implementation Slice - Docker Agent Workspace v0

Este e o corte ativo. O restante do backlog continua valido, mas fica em espera ate este slice passar nos gates de `TESTS.md`.

- [x] Criar base Go do binario `stola`.
- [x] Criar config local em `/etc/stola/stola.yaml` com backend `mock` por padrao.
- [x] Criar estado local SQLite em `/var/lib/stola/stola.db`.
- [x] Criar audit log JSONL em `/var/log/stola/audit.jsonl`.
- [x] Criar redaction de campos sensiveis em audit events.
- [x] Criar comandos `stola doctor`, `stola init`, `stola config validate`, `stola node status`.
- [x] Criar `stola provisioner mock-create`.
- [x] Criar `backend.type=docker`.
- [x] Criar config `backend.docker`.
- [x] Criar `stola docker check`.
- [x] Criar adapter Docker inicial atras da interface `provisioner.Backend`.
- [x] Criar `stola vps create` com backend Docker.
- [x] Criar `stola workspace create|list|status|logs|exec|stop|start|delete` como nome correto do produto.
- [x] Manter `stola vps ...` como alias legado temporario.
- [x] Criar network Docker por workspace.
- [x] Criar volume Docker por workspace em `/workspace`.
- [x] Aplicar limites de memoria/swap/CPU/PIDs no container.
- [x] Aplicar `--cap-drop ALL`.
- [x] Aplicar `--security-opt no-new-privileges:true`.
- [x] Persistir container ID em `external_id`.
- [x] Testar `stola docker check` contra Docker local real.
- [x] Testar `stola vps create` contra Docker local real com cleanup.
- [x] Criar imagem `stola-agent-base`.
- [x] Instalar Hermes na imagem base.
- [x] Instalar OpenCode na imagem base.
- [x] Instalar OpenClaw na imagem base.
- [x] Criar manifesto de MCPs permitidos para workspace.
- [x] Criar gerenciador MCP para ler/validar/renderizar `manifests/mcp/default.yaml`.
- [x] Criar `stola mcp validate`.
- [x] Criar `stola mcp list`.
- [x] Criar `stola mcp render`.
- [x] Gerar allowlist JSON por workspace em `.stola/mcp/allowlist.json`.
- [x] Gerar config minima OpenCode por workspace em `opencode.json`.
- [x] Gerar config minima Hermes em `.stola/mcp/hermes.config.yaml`.
- [x] Gerar config minima OpenClaw em `.stola/mcp/openclaw.json`.
- [x] Injetar bundle MCP automaticamente no `/workspace` durante `stola vps create` com backend Docker.
- [x] Injetar config nativa OpenCode em `/home/stola/.config/opencode/opencode.json`.
- [x] Injetar config nativa Hermes em `/home/stola/.hermes/config.yaml`.
- [x] Injetar config nativa OpenClaw em `/home/stola/.openclaw/openclaw.json`.
- [x] Criar manifesto `manifests/providers/default.yaml` com OpenRouter, OpenAI, Anthropic, Gemini, Azure OpenAI, Bedrock, Groq, Mistral, Cohere, DeepSeek, xAI, Together, Fireworks, Perplexity, Cerebras, SambaNova, Replicate, Hugging Face, Cloudflare, NVIDIA NIM, Vercel AI Gateway, LiteLLM Proxy, Ollama, vLLM, LM Studio e endpoint OpenAI-compatible custom.
- [x] Criar gerenciador de providers para validar/listar/renderizar catalogo.
- [x] Criar comandos `stola providers validate|list|render`.
- [x] Renderizar `.stola/providers/default.yaml`, `catalog.json`, `env.example`, `litellm.config.yaml` e `openai-compatible.json`.
- [x] Injetar provider bundle automaticamente no `/workspace` durante `stola vps create` com backend Docker.
- [x] Injetar config nativa LiteLLM em `/home/stola/.config/litellm/config.yaml`.
- [x] Instalar `litellm` na imagem `stola-agent-base`.
- [x] Copiar manifesto default de providers para `/opt/stola/manifests/providers/default.yaml` na imagem.
- [x] Criar `stola secrets validate|list` para validar/listar secrets permitidos sem imprimir valores.
- [x] Criar `stola providers check --secrets-file PATH --provider ID`.
- [x] Implementar health check read-only para providers OpenAI-compatible via `GET /models`.
- [x] Cobrir OpenRouter/OpenAI usando `base_url + /models` e Bearer token do secret file.
- [x] Suportar headers opcionais do OpenRouter a partir de `OR_SITE_URL`/`OR_APP_NAME` ou `OPENROUTER_HTTP_REFERER`/`OPENROUTER_APP_TITLE`.
- [x] Fazer `providers check` exigir `--secrets-file`, sem fallback implicito para env do host.
- [x] Garantir que `providers check` nao imprime secret values em stdout/stderr nos testes.
- [x] Criar `examples/provider-secrets.env.example`.
- [x] Ignorar arquivos `.env` reais em `.gitignore` e `.dockerignore`, mantendo exemplos.
- [x] Suportar `stola vps create --secrets-file PATH` no backend Docker.
- [x] Filtrar secrets pelo manifesto de providers; variaveis fora do catalogo nao entram no workspace.
- [x] Gerar `/workspace/.stola/secrets/manifest.json` apenas com nomes de secrets, sem valores.
- [x] Injetar `/home/stola/.config/stola/secrets.env` e `secrets.json` no container por stdin, sem colocar valores em argumentos Docker.
- [x] Fazer `.profile` carregar `/home/stola/.config/stola/secrets.env` para shells do usuario `stola`.
- [x] Auditar `secrets.inject` com nomes/count apenas, sem path do arquivo e sem valores.
- [x] Remover `argv` bruto do audit de `vps.exec`; registrar apenas comando base e argc.
- [x] Renderizar OpenClaw apenas com transports aceitos pela versao atual (`streamable-http`/`sse`), sem servidores stdio invalidos.
- [x] Rodar smoke nativo com `stola-agent-base:dev`, `opencode mcp list`, `hermes mcp list` e `openclaw mcp status`.
- [x] Implementar cliente MCP stdio minimo com `initialize`, `notifications/initialized`, `tools/list` e `tools/call`.
- [x] Criar `stola mcp tools --name/--id --server SERVER`.
- [x] Criar `stola mcp call --name/--id --server SERVER --tool TOOL --input JSON|@file`.
- [x] Executar MCP stdio dentro do container com `docker exec -i -w /workspace`.
- [x] Bloquear execucao direta de servidores MCP opcionais/desabilitados.
- [x] Bloquear execucao direta de servidores MCP nao-stdio no slice atual.
- [x] Fazer preflight `tools/list` antes de `tools/call` para negar ferramenta nao anunciada.
- [x] Adicionar `tool_policy` por servidor no manifesto MCP.
- [x] Suportar actions `allow`, `require_approval` e `deny`.
- [x] Marcar `write_file`, `edit_file`, `move_file` e `create_directory` como `require_approval` no filesystem MCP.
- [x] Marcar Git MCP e Memory MCP como `require_approval` por default.
- [x] Substituir `--approve-risk` por approval IDs one-shot para tools `require_approval`.
- [x] Persistir approval intent no SQLite com `approval_id`, VPS, server, tool, `argument_keys`, `argument_digest`, status e expiracao.
- [x] Exigir `--approval-id` valido e nao usado para executar tool `require_approval`.
- [x] Bloquear reuso de approval ID ja consumido.
- [x] Implementar policy de argumentos/path para MCP filesystem antes de gerar approval.
- [x] Bloquear path fora de `/workspace`, traversal, sintaxe de host Windows, `.stola`, configs de agente, env files, chaves e paths de secrets.
- [x] Mover policy de argumentos/path para `argument_policy` configuravel no manifesto MCP.
- [x] Suportar overrides de `argument_policy` por `vps_ids`, `plans` e `user_ids`.
- [x] Suportar schema configuravel de campos por tool (`path`, `paths`, `source`, `destination`, etc.).
- [x] Implementar `execution_policy` geral no manifesto MCP para risco, rede, comando, regras, approval e rate limit.
- [x] Avaliar policy de risco/rede/comando antes de executar `tools/list`.
- [x] Suportar approval por regra em `execution_policy.rules`.
- [x] Persistir rate limit MCP no SQLite por chave de VPS/plano/usuario/server/tool/regra.
- [x] Auditar denies de execution policy em `mcp.policy`.
- [x] Auditar rate limit em `mcp.rate_limit`.
- [x] Auditar denies de path policy em `mcp.argument_policy` sem gravar path bruto.
- [x] Auditar `mcp.approval` com outcomes `not_required`, `required`, `approved` e `denied`.
- [x] Rodar smoke real de deny sem aprovacao, execucao com `--approval-id` e rejeicao de reuso do mesmo ID.
- [x] Rodar smoke real de path policy: `/etc/passwd` e `.stola` negados sem `approval_id`; path valido segue approval one-shot.
- [x] Rodar smoke real de override por plano: plan `trusted` permite `opencode.json`, mas `.stola` continua bloqueado.
- [x] Rodar smoke real de `execution_policy`: rate limit bloqueia segunda chamada e command policy nega `npx`.
- [x] Criar policy overlay MCP versionado fora do manifesto base.
- [x] Adicionar `api.users_file`, `api.signing_key_file`, `api.mcp_manifest` e `api.policy_file` na config.
- [x] Criar RBAC inicial do control plane com Bearer token hash SHA-256, roles e permissoes.
- [x] Transformar `stola daemon api` em servidor HTTP real.
- [x] Criar endpoint `GET /healthz`.
- [x] Criar endpoint autenticado `GET /v1/policy`.
- [x] Criar endpoint autenticado `POST /v1/policy/reload`.
- [x] Criar endpoint autenticado `POST /v1/mcp/approvals`.
- [x] Criar endpoint autenticado `POST /v1/mcp/approvals/{id}/approve` para assinar approval pendente.
- [x] Gerar `approval_token` HMAC assinado pelo servidor.
- [x] Validar `--approval-token` no CLI contra VPS/server/tool/argument digest/policy version.
- [x] Criar `stola approvals list` para listar approval intents via API local.
- [x] Criar `stola approvals approve --id ID --write-token PATH` para assinar approval pendente sem copiar token manualmente.
- [x] Suportar `stola mcp call --approval-token @PATH`.
- [x] Persistir `approved_by` e `policy_version` nos approval intents.
- [x] Criar exemplos `examples/control-plane-users.example.yaml` e `examples/mcp-policy.example.yaml`.
- [x] Criar `scripts/smoke-control-plane-ux-local.ps1` para testar API local + workspace + MCP + agents.
- [x] Rodar smoke de `stola mcp validate --policy-file`.
- [x] Rodar smoke local completo de UX: API localhost, `stola approvals list/approve`, approval assinado em arquivo, Hermes, OpenCode, OpenClaw e MCP filesystem.
- [x] Rodar smoke local completo de UX com `--secrets-file`, validando secret no container, `UNRELATED_SECRET` ausente, audit sem valor secreto e sem `argv` bruto.
- [x] Auditar `mcp.tools`.
- [x] Auditar `mcp.call`.
- [x] Auditar `mcp.policy` em denies.
- [x] Evitar gravar argumentos/resultados brutos no audit; registrar apenas keys, tamanho e resumo.
- [x] Rodar smoke real de `stola mcp tools` e `stola mcp call` com filesystem MCP.
- [x] Manter servidores opcionais desabilitados por padrao.
- [x] Evitar escrita de valores de secrets nos configs gerados.
- [x] Registrar evento `mcp.inject` no audit log quando aplicavel.
- [x] Criar smoke script para build/run/verificacao da imagem base.
- [x] Criar comandos `stola vps list|status|logs|exec|stop|start|delete` para Docker.
- [x] Criar adapter Proxmox inicial atras da interface `provisioner.Backend`.
- [x] Ler credencial Proxmox somente por `backend.proxmox.token_file`.
- [x] Criar `stola proxmox check` para validar API, versao, nodes e storage.
- [x] Criar `stola proxmox nodes` e `stola proxmox storage`.
- [x] Criar `stola vps create` com parametros Nano para LXC em backend Proxmox.
- [x] Persistir `external_id`/`task_id` de criacao Proxmox no SQLite.
- [x] Criar `.gitignore` para bloquear `tmp/`, tokens, DBs e logs locais.
- [x] Criar script `scripts/proxmox-lab-check.ps1` para gerar config lab e rodar somente `proxmox check`.
- [x] Criar script `scripts/proxmox-bootstrap-token.ps1` para criar usuario/token/ACL de lab via SSH e `pveum`.
- [x] Criar exemplo `examples/proxmox-lab.env.example.ps1`.
- [x] Criar placeholder local vazio `tmp/pve.token` ignorado pelo Git.
- [x] Mover validacao Proxmox live para backend futuro/premium.
- [x] Criar `stola daemon api|provisioner|auditd`.
- [x] Criar unit files systemd para `stola-api`, `stola-provisioner`, `stola-auditd`.
- [x] Criar skeleton `.deb`.
- [x] Criar ADRs 001-005.
- [x] Criar `docs/architecture/stola-os-definition.md`.
- [x] Criar `docs/architecture/threat-model.md`.
- [x] Criar testes Go iniciais para config, audit, state, provisioner e CLI.
- [x] Gerar `go.sum` com `go mod tidy`.
- [x] Rodar `gofmt -w cmd internal`.
- [x] Rodar `go test ./...`.
- [x] Corrigir falhas de build/teste.
- [x] Registrar resultado em `SESSIONS.md`.
- [x] Registrar adapter Proxmox inicial em `SESSIONS.md`.
- [x] Registrar pivot Docker-first em `SESSIONS.md`.
- [x] Registrar MCP manager em `SESSIONS.md`.
- [x] Registrar pivot Docker-only/no-OS e provider catalog em `SESSIONS.md`.
- [x] Registrar secret injection por workspace em `SESSIONS.md`.

## Proximo Corte Real - Provider Runtime E Hardening

- [x] Rodar `stola providers check` contra OpenRouter live com chave de teste explicita fornecida fora do repo.
- [x] Rodar fluxo OpenRouter end-to-end: provider check live, workspace Docker, secret injection, LiteLLM config, agentes, MCP tools, MCP call e approval one-shot.
- [x] Rodar `go test ./...` apos o fluxo OpenRouter.
- [x] Rodar fluxo OpenRouter como usuario real com chamadas LLM: OpenCode `run`, Hermes `chat --query`, OpenClaw `agent --local` e LiteLLM `completion`.
- [x] Criar `scripts/smoke-openrouter-flow-local.ps1`.
- [x] Fazer `scripts/smoke-openrouter-flow-local.ps1` receber `-SecretsFile`.
- [x] Automatizar no smoke OpenRouter: provider check, workspace create, secret injection, OpenCode, Hermes, OpenClaw, LiteLLM, MCP tools/calls, approval one-shot, audit no-leak e cleanup.
- [x] Pinar `@modelcontextprotocol/server-filesystem@2025.8.21` para evitar quebra de `npx latest` com Node 24.
- [x] Criar API control-plane inicial `GET /v1/workspaces`, `GET /v1/workspaces/{id}` e `POST /v1/workspaces`.
- [x] Criar permissoes RBAC `workspace.create` e `workspace.read`.
- [x] Testar API de workspaces por `httptest`.
- [x] Criar ledger SQLite de uso/custo por workspace, usuario, provider e modelo.
- [x] Criar API control-plane `POST /v1/usage`, `GET /v1/usage` e `GET /v1/usage/summary`.
- [x] Criar permissoes RBAC `usage.write` e `usage.read`.
- [x] Testar ledger/API de usage por `go test`.
- [x] Cobrir API usage record/summary no smoke control-plane local.
- [ ] Rodar `stola providers check` contra OpenAI live com chave de teste explicita fornecida fora do repo.
- [ ] Expandir `providers check` para providers nao OpenAI-compatible com endpoints proprios.
- [x] Criar proxy local Stolas LiteLLM-compatible/OpenAI-compatible via `stola daemon llm-proxy`.
- [x] Criar virtual keys Stolas por API `POST /v1/llm/keys`, armazenando somente hash no SQLite.
- [x] Criar listagem `GET /v1/llm/keys` sem expor token bruto.
- [x] Adicionar budget USD e rate limit RPM por virtual key no proxy.
- [x] Registrar spend/tokens/latencia/erros do proxy no ledger SQLite.
- [x] Criar `scripts/smoke-litellm-proxy-local.ps1` com upstream fake, virtual key, budget exceeded e audit no-leak.
- [x] Criar unit file `stola-llm-proxy.service`.
- [x] Trocar clientes/agents do workspace para poder usar o Stolas LLM proxy por default via `workspace create --llm-proxy-url`, sem provider secret direto no workspace.
- [x] Integrar cost-map fallback minimo quando upstream retorna tokens mas nao retorna `cost_usd`.
- [x] Adicionar suporte seguro a streaming com contabilizacao de usage.
- [x] Adicionar TPM/concurrency limit por key.
- [x] Adicionar rotação/revogação de virtual keys por API.
- [x] Criar smoke live OpenRouter via proxy (`scripts/smoke-openrouter-proxy-flow-local.ps1`).
- [x] Rodar smoke live OpenRouter via proxy com chave temporaria explicita.
- [x] Criar smoke Docker de workspace usando Stolas proxy por default (`scripts/smoke-stolas-proxy-docker-workspace-local.ps1`).
- [x] Cobrir em teste Go que a config LiteLLM injetada nao duplica prefixo de provider (`openai/openai`).
- [ ] Rodar `scripts/smoke-stolas-proxy-docker-workspace-local.ps1` end to end quando Docker Desktop/WSL voltar a iniciar o engine.
- [ ] Avaliar proxy oficial LiteLLM com Postgres para hosted multiusuario.
- [x] Renomear produto/CLI de `vps` para `workspace`, mantendo alias de compatibilidade.
- [ ] Definir hardening multi-tenant real: gVisor/Kata/KVM antes de aceitar usuario nao confiavel.
- [ ] Separar explicitamente modo local trusted dev de modo hosted/untrusted.
- [x] Cobrir no smoke OpenRouter que audit/workspace manifest/workspace files nao vazam secret e que `vps.exec` nao grava `argv` bruto.
- [ ] Capturar transcript completo stdout/stderr do smoke e varrer contra o secret literal antes do exit.

## Veredito Tecnico

Stola nao deve nascer agora como OS/ISO/installer. O MVP correto e um control plane de workspaces AI-native em Docker. Proxmox/KVM entra depois para isolamento forte, nao como requisito para validar Hermes/OpenCode/OpenClaw/MCP/providers.

Arquitetura-alvo:

```text
Host Linux/Windows dev com Docker
  -> stola CLI/control plane local
  -> container por usuario/workspace
     /workspace persistente
     Hermes, OpenCode, OpenClaw
     MCP allowlist + policy + approvals
     provider catalog + LiteLLM config
  -> providers externos por env/secret
     OpenRouter, OpenAI, Anthropic, Gemini, Bedrock, Groq, etc.
```

Dificuldade real:

| Entrega | Dificuldade |
| --- | ---: |
| Workspace Docker local | 4/10 |
| Provider catalog + LiteLLM config | 4.5/10 |
| Host OS single-node instalavel | fora do MVP |
| Imagem agent workspace padronizada | 5.5/10 |
| Provisionamento Docker confiavel | 5/10 |
| Provisionamento LXC/KVM confiavel futuro | 6.5/10 |
| Seguranca multiusuario decente | 8.5/10 |
| Agent runtime seguro com terminal/MCP | 9/10 |
| Updates/rollback/recovery bons | 8/10 |
| Multi-node/cluster/billing forte | 9/10 |
| Kernel/distro do zero | 10/10, nao fazer |

## Decisoes De Arquitetura

- [x] ADR-001: Definir Stola OS como appliance, nao kernel/distro from scratch. Superseded para o MVP por ADR-007.
- [ ] ADR-002: Definir Proxmox VE como hypervisor/controlador local inicial.
- [ ] ADR-003: Definir Debian como base do host e das imagens guest iniciais.
- [ ] ADR-004: Definir LXC unprivileged para planos Nano/Basic.
- [ ] ADR-005: Definir KVM para usuarios nao confiaveis, planos Pro e workloads com maior risco.
- [x] ADR-006: Definir Docker-first Agent Workspaces.
- [x] ADR-007: Definir Docker-only/no-OS para o MVP.
- [x] ADR-008: Definir provider catalog + LiteLLM/OpenAI-compatible config como default do MVP.
- [ ] ADR-009: Adiar GPU local/vLLM para fase posterior ao MVP.
- [ ] ADR-010: Adiar marketplace MCP publico ate existir policy engine e assinatura.
- [ ] ADR-011: Definir que browser automation nao fica ativo por default em planos pequenos.
- [ ] ADR-012: Definir que toda tool call do agent precisa de audit log.
- [ ] ADR-013: Definir que secrets nunca entram em prompt/log bruto.
- [ ] ADR-014: Definir rollback obrigatorio antes de updates de host/control plane.

## Fontes Base Investigadas

Fontes primarias e repos relevantes:

- Proxmox VE Administration Guide: https://pve.proxmox.com/pve-docs/pve-admin-guide.html
- Proxmox automated installation: https://pve.proxmox.com/wiki/Automated_Installation
- Proxmox API: https://pve.proxmox.com/wiki/Proxmox_VE_API
- Proxmox Container Toolkit: https://pve.proxmox.com/pve-docs/chapter-pct.html
- Proxmox Cloud-Init: https://pve.proxmox.com/wiki/Cloud-Init_Support
- Proxmox Firewall: https://pve.proxmox.com/pve-docs/chapter-pve-firewall.html
- Proxmox SDN: https://pve.proxmox.com/pve-docs/chapter-pvesdn.html
- Proxmox Storage: https://pve.proxmox.com/pve-docs/chapter-pvesm.html
- Proxmox Backup/Restore: https://pve.proxmox.com/pve-docs/chapter-vzdump.html
- Debian preseed: https://www.debian.org/releases/forky/amd64/apb.en.html
- Debian Policy Manual: https://www.debian.org/doc/debian-policy/
- Debian package signing: https://www.debian.org/doc/manuals/securing-debian-manual/deb-pack-sign.en.html
- Debian Live Manual: https://live-team.pages.debian.net/live-manual/html/live-manual/index.en.html
- cloud-init docs: https://docs.cloud-init.io/
- Linux cgroup v2 docs: https://docs.kernel.org/admin-guide/cgroup-v2.html
- systemd resource control: https://www.freedesktop.org/software/systemd/man/systemd.resource-control.html
- systemd offline updates: https://www.freedesktop.org/software/systemd/man/systemd.offline-updates.html
- AppArmor docs: https://apparmor.net/
- NIST SP 800-190: https://csrc.nist.gov/pubs/sp/800/190/final
- CIS Debian Benchmark: https://www.cisecurity.org/benchmark/debian_linux
- LiteLLM virtual keys: https://docs.litellm.ai/docs/proxy/virtual_keys
- LiteLLM budgets/rate limits: https://docs.litellm.ai/docs/proxy/users
- MCP security best practices: https://modelcontextprotocol.io/docs/tutorials/security/security_best_practices
- MCP servers repo: https://github.com/modelcontextprotocol/servers
- NSA MCP security considerations: https://media.defense.gov/2026/Jun/02/2003943289/-1/-1/0/CSI_MCP_SECURITY.PDF
- Proxmoxer: https://github.com/proxmoxer/proxmoxer
- BPG Terraform Proxmox provider: https://github.com/bpg/terraform-provider-proxmox
- Packer Proxmox plugin: https://github.com/hashicorp/packer-plugin-proxmox
- Ansible community.proxmox: https://docs.ansible.com/projects/ansible/latest/collections/community/proxmox/index.html
- bubblewrap: https://github.com/containers/bubblewrap
- gVisor: https://gvisor.dev/docs/
- Firecracker: https://github.com/firecracker-microvm/firecracker
- Kata Containers: https://katacontainers.io/
- Prometheus node exporter: https://prometheus.io/docs/guides/node-exporter/
- OpenTelemetry docs: https://opentelemetry.io/docs/
- Langfuse self-hosting: https://langfuse.com/self-hosting

## Backlog Legado/Futuro

As fases abaixo vieram do plano OS/Proxmox original. Elas ficam preservadas como pesquisa e backlog futuro, mas **nao sao gates do MVP atual**. O corte ativo e o bloco "Current Implementation Slice - Docker Agent Workspace v0".

## Fase 0 - Definicao Do Produto OS (legado/futuro)

Objetivo: fechar o que e OS, o que e control plane, e o que fica fora do MVP.

- [ ] Escrever `docs/architecture/stola-os-definition.md`.
  - [ ] Definir Stola Host OS.
  - [ ] Definir Stola Guest OS.
  - [ ] Definir Stola Control Plane.
  - [ ] Definir AI Runtime.
  - [ ] Explicar por que nao criar kernel novo.
  - [ ] Explicar por que Proxmox/Debian e a base inicial.
- [ ] Escrever `docs/architecture/threat-model.md`.
  - [ ] Listar atores: admin host, usuario VPS, agent, MCP server, provider LLM, atacante externo.
  - [ ] Listar assets: host root, API keys, VPS FS, LiteLLM keys, logs, templates, snapshots.
  - [ ] Listar fronteiras: host/guest, guest/agent, agent/MCP, MCP/secrets, API/dashboard.
  - [ ] Listar riscos: container escape, prompt injection, MCP supply chain, token leakage, SSRF, egress abuse.
- [ ] Escrever `docs/architecture/non-goals.md`.
  - [ ] Excluir kernel proprio.
  - [ ] Excluir OpenStack no MVP.
  - [ ] Excluir marketplace publico no MVP.
  - [ ] Excluir GPU compartilhada como requisito inicial.
  - [ ] Excluir HA multi-node no MVP.
- [ ] Escrever matriz de planos.
  - [ ] Plano Nano: 1 GB RAM, LXC, sem Docker/browser.
  - [ ] Plano Basic: 2 GB RAM, LXC, MCP basico.
  - [ ] Plano Dev: 4 GB RAM, Claude Code leve.
  - [ ] Plano Pro: 8 GB+ RAM, KVM opcional, browser/sandbox.

## Fase 1 - Host OS Bootstrap (legado/futuro)

Objetivo: transformar um Proxmox/Debian limpo em Stola Host OS single-node.

- [ ] Criar repositorio `stola-os`.
- [ ] Criar layout de monorepo.
  - [ ] `host/installer`
  - [ ] `host/packages`
  - [ ] `host/systemd`
  - [ ] `guest/images`
  - [ ] `control-plane/api`
  - [ ] `control-plane/dashboard`
  - [ ] `control-plane/provisioner`
  - [ ] `security/policies`
  - [ ] `docs/architecture`
  - [ ] `docs/runbooks`
  - [ ] `tests/e2e`
- [ ] Criar instalador bootstrap `install.stola.sh`.
  - [ ] Verificar se roda em Debian/Proxmox suportado.
  - [ ] Validar root/admin.
  - [ ] Validar CPU virtualization flags.
  - [ ] Validar systemd.
  - [ ] Validar storage disponivel.
  - [ ] Validar rede publica e interna.
  - [ ] Instalar dependencias base.
  - [ ] Configurar repo APT Stola.
  - [ ] Instalar pacotes Stola.
  - [ ] Rodar healthcheck final.
- [ ] Criar modo dry-run do instalador.
- [ ] Criar modo uninstall/desregistro seguro.
- [ ] Criar arquivo `/etc/stola/stola.yaml`.
- [ ] Criar comando `stola doctor`.
- [ ] Criar comando `stola version`.
- [ ] Criar comando `stola node status`.
- [ ] Criar suite de testes em VM local/nested Proxmox.

## Fase 2 - Installer/ISO Do Host (legado/futuro)

Objetivo: virar OS instalavel, nao apenas script.

- [ ] Criar estrategia v1: Proxmox automated installation com `answer.toml`.
  - [ ] Gerar answer file minimo.
  - [ ] Testar install unattended.
  - [ ] Configurar hostname.
  - [ ] Configurar root/admin inicial.
  - [ ] Configurar rede.
  - [ ] Configurar storage.
  - [ ] Injetar bootstrap Stola no primeiro boot.
- [ ] Criar estrategia v2: ISO custom Stola.
  - [ ] Avaliar wrapper sobre ISO Proxmox.
  - [ ] Avaliar Debian live-build apenas para rescue/installer.
  - [ ] Definir assinatura/checksum da ISO.
  - [ ] Criar pipeline de build da ISO.
  - [ ] Criar teste de boot em QEMU.
  - [ ] Criar teste de install em bare-metal lab.
- [ ] Criar wizard de primeiro boot.
  - [ ] Configurar admin.
  - [ ] Configurar dominio/TLS.
  - [ ] Configurar storage pool.
  - [ ] Configurar bridge publica.
  - [ ] Configurar rede interna.
  - [ ] Configurar LiteLLM providers.
  - [ ] Configurar email/alertas.
- [ ] Criar recovery mode.
  - [ ] Resetar senha admin.
  - [ ] Desativar control plane com seguranca.
  - [ ] Subir shell de emergencia.
  - [ ] Exportar logs de diagnostico.

## Fase 3 - Pacotes E Servicos Systemd

Objetivo: empacotar Stola como OS, com servicos gerenciados e atualizaveis.

- [ ] Criar pacote `stola-cli`.
  - [ ] `stola vps list`
  - [ ] `stola vps create`
  - [ ] `stola vps delete`
  - [ ] `stola vps start`
  - [ ] `stola vps stop`
  - [ ] `stola vps exec`
  - [ ] `stola logs`
  - [ ] `stola doctor`
- [ ] Criar pacote `stola-api`.
  - [ ] Systemd unit.
  - [ ] Health endpoint.
  - [ ] Config reload.
  - [ ] Structured logs.
  - [ ] API auth local.
- [ ] Criar pacote `stola-provisioner`.
  - [ ] Worker systemd.
  - [ ] Queue interna.
  - [ ] Idempotencia por job.
  - [ ] Lock por VMID/CTID.
  - [ ] Retry com backoff.
- [ ] Criar pacote `stola-networkd`.
  - [ ] Gerenciar bridge/VNet.
  - [ ] Gerenciar firewall por VPS.
  - [ ] Gerenciar reverse proxy.
  - [ ] Gerenciar DNS interno.
- [ ] Criar pacote `stola-storaged`.
  - [ ] Gerenciar storage pools.
  - [ ] Gerenciar volumes.
  - [ ] Gerenciar snapshots.
  - [ ] Gerenciar quotas.
- [ ] Criar pacote `stola-secrets`.
  - [ ] Guardar provider keys.
  - [ ] Guardar LiteLLM virtual keys.
  - [ ] Guardar tokens MCP.
  - [ ] Redactar logs.
- [ ] Criar pacote `stola-metering`.
  - [ ] Medir CPU/RAM/disk/rede.
  - [ ] Medir tokens por virtual key.
  - [ ] Medir custo por provider/modelo.
  - [ ] Emitir alertas de budget.
- [ ] Criar pacote `stola-auditd`.
  - [ ] Audit log de API calls.
  - [ ] Audit log de provisionamento.
  - [ ] Audit log de tool calls.
  - [ ] Audit log de secrets access.
- [ ] Criar pacote `stola-updater`.
  - [ ] Verificar canal stable/beta/edge.
  - [ ] Baixar updates.
  - [ ] Criar snapshot pre-update.
  - [ ] Rodar healthcheck post-update.
  - [ ] Rollback se falhar.
- [ ] Criar pacote `stola-guest-agent`.
  - [ ] Registrar guest no host.
  - [ ] Reportar health.
  - [ ] Reportar agent runtime status.
  - [ ] Aplicar policy local.

## Fase 4 - Repositorio APT E Supply Chain

Objetivo: permitir instalacao/update confiavel.

- [ ] Criar repo APT `apt.stola.ai`.
- [ ] Criar chave GPG/Ed25519 de assinatura.
- [ ] Definir politica de rotacao de chaves.
- [ ] Criar pipeline de build `.deb`.
- [ ] Criar pipeline de assinatura.
- [ ] Criar ambientes `stable`, `beta`, `edge`.
- [ ] Criar SBOM por pacote.
- [ ] Criar scan de dependencias.
- [ ] Criar reproducibility check basico.
- [ ] Criar changelog por release.
- [ ] Criar rollback de pacote.
- [ ] Criar pinning de versoes criticas.
- [ ] Criar mirror privado para lab.

## Fase 5 - VPS Engine

Objetivo: criar/deletar/gerenciar LXC e KVM de forma segura.

- [ ] Criar adaptador Proxmox API.
  - [ ] Avaliar Proxmoxer.
  - [x] Avaliar chamada direta REST.
  - [x] Definir autenticacao API token.
  - [x] Implementar timeout.
  - [ ] Implementar retry.
  - [ ] Implementar idempotency keys.
- [ ] Implementar LXC create.
  - [x] Selecionar template.
  - [x] Definir CTID.
  - [x] Definir hostname.
  - [x] Definir RAM/swap.
  - [x] Definir CPU cores.
  - [x] Definir disk quota inicial via `rootfs`.
  - [x] Definir network DHCP via bridge.
  - [x] Definir unprivileged.
  - [ ] Definir nesting off por default.
  - [ ] Definir AppArmor profile.
  - [ ] Validar create/delete em Proxmox lab real.
- [ ] Implementar KVM create.
  - [ ] Clonar template cloud-init.
  - [ ] Definir VMID.
  - [ ] Definir CPU/RAM/disk.
  - [ ] Definir cloud-init user-data.
  - [ ] Instalar/ativar qemu-guest-agent.
  - [ ] Reportar IP.
  - [ ] Validar boot health.
- [ ] Implementar start/stop/reboot.
- [ ] Implementar delete com grace period.
- [ ] Implementar suspend/pause quando aplicavel.
- [ ] Implementar resize CPU/RAM/disk.
- [ ] Implementar snapshot.
- [ ] Implementar rollback snapshot.
- [ ] Implementar backup.
- [ ] Implementar restore.
- [ ] Implementar console/terminal web bridge.
- [ ] Implementar job event stream.

## Fase 6 - Guest OS Images

Objetivo: criar imagens oficiais Stola para cada plano.

- [ ] Criar `stola-guest-nano`.
  - [ ] Debian minimal.
  - [ ] SSH.
  - [ ] stola-guest-agent.
  - [ ] Hermes light.
  - [ ] MCP filesystem restrito.
  - [ ] Sem Docker.
  - [ ] Sem browser.
  - [ ] 1 GB RAM alvo.
- [ ] Criar `stola-guest-basic`.
  - [ ] Debian minimal.
  - [ ] Git.
  - [ ] Node/Python runtime minimo.
  - [ ] Hermes.
  - [ ] MCP git/github opcional.
  - [ ] 2 GB RAM alvo.
- [ ] Criar `stola-guest-dev`.
  - [ ] Git, Node, Python, build tools.
  - [ ] Claude Code opcional.
  - [ ] Test runners comuns.
  - [ ] 4 GB RAM alvo.
- [ ] Criar `stola-guest-pro`.
  - [ ] KVM-first opcional.
  - [ ] Docker opcional.
  - [ ] Browser automation isolado.
  - [ ] 8 GB+ RAM alvo.
- [ ] Criar pipeline Packer para VM templates.
- [ ] Criar pipeline para LXC templates.
- [ ] Criar assinatura das imagens.
- [ ] Criar scan CVE das imagens.
- [ ] Criar teste de boot das imagens.
- [ ] Criar teste de memoria minima.
- [ ] Criar teste de agent startup.
- [ ] Criar teste de LiteLLM key.
- [ ] Criar teste de MCP allowlist.

## Fase 7 - Rede E Multitenancy

Objetivo: isolar tenants e expor servicos com seguranca.

- [ ] Definir topologia single-node.
  - [ ] Bridge publica.
  - [ ] Bridge interna control plane.
  - [ ] Network por tenant ou por VPS.
  - [ ] NAT/reverse proxy quando nao houver IP dedicado.
- [ ] Definir topologia multi-node futura.
  - [ ] SDN zones.
  - [ ] VNets.
  - [ ] Subnets.
  - [ ] IPAM.
  - [ ] VXLAN/EVPN se necessario.
- [ ] Implementar firewall host default deny.
- [ ] Implementar firewall por VPS.
- [ ] Implementar egress policy.
- [ ] Implementar bloqueio metadata endpoints.
- [ ] Implementar rate limit por VPS.
- [ ] Implementar portas publicas declarativas.
- [ ] Implementar TLS automatico.
- [ ] Implementar DNS interno.
- [ ] Implementar audit log de mudancas de rede.
- [ ] Criar testes de isolamento entre VPS.
- [ ] Criar testes de egress abuse.
- [ ] Criar testes de SSRF basico.

## Fase 8 - Storage, Snapshot E Backup

Objetivo: storage previsivel, restauravel e com quotas reais.

- [ ] Definir storage default single-node.
  - [ ] Avaliar ZFS mirror.
  - [ ] Avaliar LVM-thin.
  - [ ] Definir tradeoff de snapshots/clones.
- [ ] Definir storage multi-node futuro.
  - [ ] Avaliar Ceph apenas em cluster real.
  - [ ] Avaliar Proxmox Backup Server.
- [ ] Implementar quota por VPS.
- [ ] Implementar alerta de pool cheio.
- [ ] Implementar snapshot pre-update.
- [ ] Implementar snapshot pre-agent-risk-action.
- [ ] Implementar backup agendado.
- [ ] Implementar restore testado.
- [ ] Implementar retention policy.
- [ ] Implementar export de usuario.
- [ ] Implementar limpeza de caches/logs.
- [ ] Criar runbook de disaster recovery.
- [ ] Criar teste mensal de restore.

## Fase 9 - Security Hardening Do Host

Objetivo: deixar o host defensavel antes de vender multiusuario.

- [ ] Aplicar baseline CIS Debian adaptado.
- [ ] Desabilitar login root por senha.
- [ ] Exigir SSH key.
- [ ] Exigir 2FA para dashboard/admin.
- [ ] Restringir Proxmox UI/API por rede/admin.
- [ ] Configurar firewall host.
- [ ] Configurar unattended security updates ou update orchestrado.
- [ ] Configurar auditd/journald persistente.
- [ ] Configurar AppArmor profiles.
- [ ] Configurar seccomp quando aplicavel.
- [ ] Configurar cgroups v2 resource boundaries.
- [ ] Restringir unprivileged user namespaces conforme perfil de sandbox.
- [ ] Remover pacotes desnecessarios.
- [ ] Validar permissao de arquivos sensiveis.
- [ ] Redactar secrets em logs.
- [ ] Criar scanner de config insegura.
- [ ] Criar `stola security check`.
- [ ] Criar teste de hardening em CI/lab.

## Fase 10 - Agent Runtime Seguro

Objetivo: integrar agentes sem entregar root remoto para qualquer prompt.

- [ ] Definir usuario Linux do agent.
- [ ] Definir diretorios permitidos.
- [ ] Definir modelo de permissoes por tool.
- [ ] Definir policy file por VPS.
- [ ] Implementar allowlist de MCPs.
- [ ] Implementar allowlist/denylist de comandos.
- [ ] Implementar modos: read-only, dry-run, execute.
- [ ] Implementar approval gates para acoes perigosas.
- [ ] Implementar sandbox de comandos com bubblewrap para planos suportados.
- [ ] Avaliar gVisor/Firecracker/Kata para execucao nao confiavel futura.
- [ ] Bloquear acesso automatico a secrets.
- [ ] Redactar secrets antes de enviar para LLM.
- [ ] Logar tool input/output com redaction.
- [ ] Definir limite de tokens por tarefa.
- [ ] Definir limite de tempo por tarefa.
- [ ] Definir limite de processos.
- [ ] Definir limite de rede por tarefa.
- [ ] Criar testes de prompt injection.
- [ ] Criar testes de command injection.
- [x] Criar testes de path traversal para MCP filesystem path policy.
- [ ] Criar testes de secret leakage.

## Fase 11 - MCP Governance

Objetivo: MCP como parte nativa da OS, mas governado por policy.

- [ ] Criar registry interno de MCPs aprovados.
- [ ] Criar manifesto MCP Stola.
  - [ ] Nome.
  - [ ] Versao.
  - [ ] Origem.
  - [ ] Hash.
  - [ ] Permissoes.
  - [ ] Secrets exigidos.
  - [ ] Network access.
  - [ ] Filesystem access.
- [ ] Implementar instalacao MCP por usuario.
- [ ] Implementar MCP por VPS.
- [ ] Implementar MCP por plano.
- [ ] Implementar tool naming deterministico.
- [ ] Implementar tool permission prompts.
- [ ] Implementar isolamento por processo.
- [ ] Implementar audit log por tool call.
- [ ] Implementar scan de MCP de terceiros.
- [ ] Implementar rollback/uninstall MCP.
- [ ] Bloquear marketplace publico ate passar security gate.
- [ ] Criar lista inicial: filesystem, git, github, sqlite, web-search restrito.
- [ ] Criar testes de tool name collision.
- [ ] Criar testes de malicious tool description.
- [ ] Criar testes de OAuth/token leakage.

## Fase 12 - LiteLLM E Modelo Gateway

Objetivo: custo e acesso a modelos como recurso nativo da OS.

- [ ] Subir LiteLLM central.
- [ ] Conectar Postgres.
- [ ] Criar virtual key por VPS.
- [ ] Criar budget por plano.
- [ ] Criar whitelist de modelos por plano.
- [ ] Criar RPM/TPM por plano.
- [ ] Criar fallback providers.
- [ ] Criar spend tracking.
- [ ] Criar alertas de budget.
- [ ] Criar revogacao de key.
- [ ] Criar rotacao de key.
- [ ] Criar logs de chamadas LLM.
- [ ] Criar redaction pre-log.
- [ ] Integrar Langfuse ou equivalente.
- [ ] Criar testes de budget exceeded.
- [ ] Criar testes de fallback.
- [ ] Criar testes de provider outage.

## Fase 13 - Dashboard E API Da OS

Objetivo: operacao visual sem esconder os limites de seguranca.

- [ ] Criar API auth.
- [ ] Criar RBAC admin/user.
- [ ] Criar tela de VPS list.
- [ ] Criar tela de VPS create.
- [ ] Criar tela de VPS detail.
- [ ] Criar terminal web.
- [ ] Criar logs viewer.
- [ ] Criar metrics viewer.
- [ ] Criar budgets viewer.
- [ ] Criar MCP manager.
- [ ] Criar cron manager.
- [ ] Criar secrets manager.
- [ ] Criar snapshots/backups UI.
- [ ] Criar audit log UI.
- [ ] Criar update UI.
- [ ] Criar warnings para acoes perigosas.
- [ ] Criar modo read-only para suporte.
- [ ] Criar API OpenAPI spec.
- [ ] Criar client SDK minimo.

## Fase 14 - Observabilidade

Objetivo: saber o que aconteceu no host, na VPS e no agent.

- [ ] Instalar node_exporter ou equivalente.
- [ ] Coletar CPU/RAM/disk/rede host.
- [ ] Coletar metricas por VPS.
- [ ] Coletar logs systemd host.
- [ ] Coletar logs stola services.
- [ ] Coletar logs guest-agent.
- [ ] Coletar audit log agent/tool.
- [ ] Coletar LiteLLM spend.
- [ ] Coletar traces LLM.
- [ ] Definir labels: node, tenant, vps, service, job_id.
- [ ] Criar dashboards Grafana.
- [ ] Criar alertas OOM.
- [ ] Criar alertas disk full.
- [ ] Criar alertas provider cost spike.
- [ ] Criar alertas security deny spike.
- [ ] Criar runbook de incidente.

## Fase 15 - Update, Rollback E Recovery

Objetivo: update de OS sem quebrar hosts de usuarios.

- [ ] Definir canais de release.
- [ ] Implementar update dry-run.
- [ ] Implementar preflight.
- [ ] Implementar snapshot pre-update.
- [ ] Implementar backup config pre-update.
- [ ] Implementar update de pacotes Stola.
- [ ] Implementar update de guest templates.
- [ ] Implementar update de MCP catalog.
- [ ] Implementar post-update healthcheck.
- [ ] Implementar rollback automatico.
- [ ] Implementar rollback manual.
- [ ] Implementar maintenance mode.
- [ ] Implementar draining de jobs.
- [ ] Implementar release notes no dashboard.
- [ ] Criar teste de update falho.
- [ ] Criar teste de rollback.

## Fase 16 - Billing E Metering

Objetivo: cobrar ou limitar uso sem depender de estimativa frouxa.

- [ ] Definir unidades: VPS time, RAM, disk, egress, tokens, provider cost.
- [ ] Medir runtime de VPS.
- [ ] Medir consumo LiteLLM por key.
- [ ] Medir egress por VPS.
- [ ] Medir storage real.
- [ ] Definir quotas por plano.
- [ ] Definir grace period.
- [ ] Definir suspensao por budget.
- [ ] Criar invoice interno.
- [ ] Criar export CSV/JSON.
- [ ] Criar simulador de custo.
- [ ] Criar alerta antes de limite.
- [ ] Criar hard stop por limite.
- [ ] Criar testes de quota.

## Fase 17 - Testes E Gating

Objetivo: impedir que Stola vire uma caixa preta fragil.

- [ ] Criar teste de install bootstrap.
- [ ] Criar teste de ISO install.
- [ ] Criar teste de create LXC.
- [ ] Criar teste de create KVM.
- [ ] Criar teste de cloud-init.
- [ ] Criar teste de guest-agent registration.
- [ ] Criar teste de LiteLLM key.
- [ ] Criar teste de MCP filesystem policy.
- [ ] Criar teste de terminal permission.
- [ ] Criar teste de snapshot.
- [ ] Criar teste de restore.
- [ ] Criar teste de update.
- [ ] Criar teste de rollback.
- [ ] Criar teste de network isolation.
- [ ] Criar teste de resource exhaustion.
- [ ] Criar teste de OOM containment.
- [ ] Criar teste de secret redaction.
- [ ] Criar teste de prompt injection.
- [ ] Criar teste de API auth.
- [ ] Criar teste de audit completeness.
- [ ] Criar teste de backup restore mensal.

## Fase 18 - Multi-node Futuro

Objetivo: escalar depois de single-node provar valor.

- [ ] Definir cluster Proxmox.
- [ ] Definir node registration.
- [ ] Definir scheduler de placement.
- [ ] Definir capacity model.
- [ ] Definir anti-affinity.
- [ ] Definir migration policy.
- [ ] Definir shared storage.
- [ ] Avaliar Ceph apenas com equipe operacional pronta.
- [ ] Definir HA para control plane.
- [ ] Definir backup cross-node.
- [ ] Definir node drain.
- [ ] Definir node maintenance.
- [ ] Definir billing multi-node.
- [ ] Definir disaster recovery multi-node.

## Fase 19 - GPU E Inference Local Futuro

Objetivo: adicionar GPU sem comprometer o MVP.

- [ ] Definir quando GPU entra no roadmap.
- [ ] Definir GPU node separado.
- [ ] Definir vLLM central.
- [ ] Definir Ollama dev.
- [ ] Definir SGLang se workflows longos exigirem.
- [ ] Definir LiteLLM routing para modelos locais.
- [ ] Definir VRAM quota.
- [ ] Definir fila por plano.
- [ ] Definir fallback para provider externo.
- [ ] Avaliar passthrough KVM.
- [ ] Avaliar MIG apenas em hardware suportado.
- [ ] Criar teste de provider local outage.
- [ ] Criar metricas GPU.

## Fase 20 - Documentacao E Runbooks

Objetivo: operar como OS, nao como script artesanal.

- [ ] `docs/install.md`
- [ ] `docs/admin-guide.md`
- [ ] `docs/user-guide.md`
- [ ] `docs/security-model.md`
- [ ] `docs/networking.md`
- [ ] `docs/storage.md`
- [ ] `docs/backup-restore.md`
- [ ] `docs/update-rollback.md`
- [ ] `docs/mcp-governance.md`
- [ ] `docs/agent-runtime.md`
- [ ] `docs/billing-metering.md`
- [ ] `docs/troubleshooting.md`
- [ ] `docs/disaster-recovery.md`
- [ ] `docs/release-process.md`

## Ordem Pragmatica De Execucao

1. Fechar ADR Docker-first e threat model.
2. Criar backend Docker completo.
3. Criar imagem `stola-agent-base`.
4. Instalar Hermes, OpenCode e OpenClaw na imagem.
5. Criar MCP manifest/allowlist por workspace.
6. Criar LiteLLM central com virtual keys.
7. Criar `stola-api`, `stola-cli` e `stola-provisioner`.
8. Implementar audit log de tool calls.
9. Implementar comandos lifecycle Docker.
10. Implementar dashboard minimo.
11. Depois validar Proxmox/KVM para plano Pro/sensivel.
12. So depois abrir MCP marketplace publico, browser e GPU.

## Definition Of Done Do MVP

- [ ] Host Linux com Docker vira Stola Host OS via bootstrap.
- [ ] `stola doctor` passa.
- [ ] Dashboard abre localmente com admin.
- [ ] Usuario cria um workspace Nano.
- [ ] Workspace sobe com agent runtime.
- [ ] Hermes responde dentro do workspace.
- [ ] LiteLLM virtual key funciona e tem budget.
- [ ] MCP filesystem restrito funciona.
- [ ] Terminal web funciona com auditoria.
- [ ] Snapshot e rollback funcionam.
- [ ] Logs e metricas aparecem.
- [ ] Secret redaction test passa.
- [ ] Network isolation test passa.
- [ ] Restore test passa.
- [ ] Update dry-run funciona.

## Riscos Principais

- Risco 1: Docker/LXC multiusuario nao e fronteira forte contra kernel bugs. Mitigacao: gVisor/Kata/KVM para tenants de maior risco.
- Risco 2: MCP amplia superficie de ataque. Mitigacao: registry interno, allowlist, manifestos e audit log.
- Risco 3: Agente com terminal pode executar dano real. Mitigacao: policy engine, dry-run, approvals e sandbox.
- Risco 4: Billing por token/provider pode divergir. Mitigacao: LiteLLM virtual keys, spend tracking e reconciliation.
- Risco 5: Update quebrar host. Mitigacao: snapshots, healthchecks, rollback e canais.
- Risco 6: Storage pool encher. Mitigacao: quotas, alertas, retention e hard stop.
- Risco 7: Secrets vazarem em logs/prompts. Mitigacao: vault, redaction e testes especificos.
- Risco 8: Marketplace virar supply-chain attack. Mitigacao: nao abrir no MVP.

## Estimativa Honesta

- Prototipo local: 4 a 8 semanas, 1 a 2 devs fortes.
- MVP single-node vendavel: 3 a 6 meses, 2 a 4 devs.
- Beta multiusuario confiavel: 6 a 12 meses.
- OS/appliance state-of-the-art com multi-node, marketplace e GPU: 12 a 24 meses.
