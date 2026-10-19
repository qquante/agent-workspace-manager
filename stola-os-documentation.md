# Stola OS — Documentação Fundacional v0.1
> AI-Native Operating System · Maio 2026

---

## 1. O que é o Stola OS

Stola OS não é um Linux com AI colada por cima. É um **sistema operacional onde a AI domina cada camada** — do boot ao terminal, da VPS ao cron job, do inference local ao provider na nuvem.

A premissa central é simples: o usuário chega, escolhe suas especificações, inicia a VPS, e o ambiente já está vivo — com agents rodando, MCPs conectados, modelos disponíveis, e cron funcionando. Sem configurar nada manualmente.

**Atualização de escopo — 2026-06-24:** o MVP deve começar como **Docker-only Agent Workspace**, não como ISO, installer, OS novo ou appliance Proxmox/Debian. Proxmox/KVM continua importante para planos de isolamento forte, mas o primeiro produto deve validar Hermes, OpenCode, OpenClaw, MCPs, memória, cron, LiteLLM, catálogo de providers e políticas dentro de containers por usuário.

**Analogia correta:** é para AI o que o Proxmox é para virtualização — você instala, e já tem um datacenter. O Stola OS instala, e já tem um sistema nervoso de AI.

---

## 2. Princípios de design

| Princípio | Significado prático |
|-----------|-------------------|
| **AI-first** | A AI não é um app. É o shell, o scheduler, o operador |
| **Workspace isolado por usuário** | Cada usuário vive primeiro num container com recursos próprios; VM/KVM entra para isolamento forte |
| **Opt-in, não opt-out** | O usuário escolhe o que instalar — nada é forçado |
| **MCP como protocolo nativo** | Tudo que o sistema expõe é um MCP server |
| **Self-improving** | O agent aprende, cria skills, e melhora com o uso |
| **Zero lock-in de modelo** | Troca de modelo com um comando, sem mudar código |

---

## 3. Arquitetura em camadas

```
┌──────────────────────────────────────────────────────────────┐
│  CAMADA 5 — INTERFACE DO USUÁRIO                             │
│  Painel Web · TUI (hermes --tui) · Telegram · Discord · CLI  │
├──────────────────────────────────────────────────────────────┤
│  CAMADA 4 — AGENTS (OS AI Layer)                             │
│  Hermes Agent v0.13 · Claude Code · Cron Engine · Kanban     │
├──────────────────────────────────────────────────────────────┤
│  CAMADA 3 — MCP BUS (protocolo universal)                    │
│  MCPs padrão pré-instalados · MCPs opcionais via painel      │
├──────────────────────────────────────────────────────────────┤
│  CAMADA 2 — INFERENCE + GATEWAY                              │
│  vLLM (produção) · Ollama (dev) · LiteLLM proxy              │
├──────────────────────────────────────────────────────────────┤
│  CAMADA 1 — VPS ENGINE                                       │
│  Docker/containerd primeiro · Proxmox/KVM depois             │
├──────────────────────────────────────────────────────────────┤
│  CAMADA 0 — HARDWARE                                         │
│  Bare-metal · GPU(s) · 256 GB RAM · NVMe storage             │
└──────────────────────────────────────────────────────────────┘
```

---

## 4. O sistema de VPS

### 4.1 Filosofia

Cada usuário recebe um **workspace isolado** — primeiro um container Docker com CPU, RAM, disco e rede próprios; depois LXC/KVM para planos que precisam de isolamento mais forte. Nenhum workspace deve montar recursos de outro. O Stola OS gerencia o que roda dentro de cada workspace.

### 4.2 Fluxo de criação de VPS

```
[1] Usuário acessa painel web
        ↓
[2] Wizard de especificações
    - RAM: 2 GB / 4 GB / 8 GB / 16 GB / 32 GB / custom
    - CPU: 1 / 2 / 4 / 8 vCPUs
    - Disco: 20 GB / 50 GB / 100 GB / 500 GB
    - GPU: nenhuma / compartilhada / passthrough dedicado
        ↓
[3] Wizard de software (opt-in por categoria)
    - Inference local? → escolhe: vLLM / Ollama / ambos / nenhum
    - Modelos locais? → lista interativa (Llama 4, Qwen3, DeepSeek...)
    - Providers externos? → OpenRouter / Anthropic / OpenAI / Groq
    - Agent principal? → Hermes Agent / Claude Code / ambos
    - MCPs padrão? → quais ativar (lista abaixo)
    - MCPs extras? → marketplace de MCPs
    - Cron jobs? → templates prontos ou custom
        ↓
[4] Geração do cloud-init / preseed automático
        ↓
[5] Boot da VPS (< 3 minutos)
        ↓
[6] Hermes Agent inicia automaticamente
    - hermes setup roda, detecta config do wizard
    - Providers configurados via .env
    - MCPs conectados
    - Skills carregadas
    - Crons registrados
        ↓
[7] Usuário recebe URL do painel web da sua VPS
    ou acessa via SSH / TUI / Telegram / Discord
```

### 4.3 Divisão de recursos num servidor de 256 GB

Exemplo de divisão padrão num host com 256 GB RAM e 2× GPUs:

```
Host Proxmox (reserva): 8 GB RAM, sem GPU
─────────────────────────────────────────
VPS de infraestrutura (interna):
  - LiteLLM gateway:    4 GB RAM
  - Postgres + Redis:   4 GB RAM
  - Langfuse (traces):  4 GB RAM
  - Open WebUI:         2 GB RAM
─────────────────────────────────────────
VPS de inference local (GPU):
  - vLLM servidor:     32 GB RAM + GPU #1 completa
  - Ollama dev:        16 GB RAM + GPU #2 (compartilhada)
─────────────────────────────────────────
VPS de usuários (pool restante ~186 GB):
  Cada usuário recebe entre 2 GB e 32 GB conforme escolha
  Ex: 20 usuários × 8 GB = 160 GB
  Ex: 10 usuários × 16 GB = 160 GB
```

---

## 5. Hermes Agent — O coração do OS

### 5.1 O que é de fato (v0.13, maio 2026)

Hermes Agent (NousResearch) é o agent que "cresce com você". Não é um chatbot — é um **runtime de AI autônomo** que vive na sua VPS, aprende com o uso, cria skills próprias, e executa tarefas de forma persistente.

**Repositório oficial:** `github.com/NousResearch/hermes-agent`
**Versão atual:** v2026.5.7 ("The Tenacity Release")
**Stars:** 23k+

### 5.2 Componentes internos do Hermes

```
hermes-agent/
├── agent/
│   ├── run_agent.py          ← Loop principal do agent
│   ├── prompt_builder.py     ← Monta o system prompt dinamicamente
│   └── transports/           ← Camada de comunicação com providers
├── tools/
│   ├── terminal_tool.py      ← Executa comandos shell
│   ├── file_tools.py         ← Lê, escreve, edita arquivos
│   ├── web_tools.py          ← web_search, web_extract
│   ├── cronjob_tools.py      ← Agenda e gerencia cron jobs
│   ├── skill_tools.py        ← Carrega e gerencia skills
│   ├── delegate_tool.py      ← Spawna subagents paralelos
│   ├── mcp_tool.py           ← Cliente MCP (1.900+ linhas Python)
│   ├── memory_tool.py        ← MEMORY.md, USER.md persistentes
│   ├── kanban_tools.py       ← Board multi-agent durável
│   └── environments/
│       ├── local.py          ← Execução local
│       ├── docker.py         ← Execução em container
│       ├── ssh.py            ← Execução remota via SSH
│       └── daytona.py        ← Sandbox cloud
├── gateway/
│   ├── run.py                ← GatewayRunner (lifecycle + cron)
│   └── platforms/
│       ├── telegram.py
│       ├── discord_adapter.py
│       ├── slack.py
│       ├── whatsapp.py
│       └── signal.py
└── ~/.hermes/                ← Dados do usuário
    ├── config.yaml           ← Config principal
    ├── .env                  ← API keys (nunca no git)
    ├── sessions/             ← Histórico de conversas (SQLite)
    ├── skills/               ← Skills criadas e instaladas
    ├── memories/             ← MEMORY.md, USER.md
    └── cron/                 ← Definições de cron jobs
```

### 5.3 Sistema de Tools (~70 tools nativas)

| Categoria | Tools |
|-----------|-------|
| **Terminal** | `terminal`, `process` |
| **Arquivos** | `read_file`, `write_file`, `patch`, `list_files` |
| **Web** | `web_search`, `web_extract` |
| **Browser** | 10 tools browser + 2 CDP-gated |
| **Cron** | `cronjob` (create/list/update/pause/resume/run/remove) |
| **Memória** | `memory` (MEMORY.md), `session_search` (FTS5) |
| **Skills** | `skill_view`, `skill_manage`, `skills_list` |
| **Delegação** | `delegate_task` (spawna subagents isolados) |
| **Código** | `execute_code` (Python sandboxado com acesso a tools) |
| **Multi-agent** | `kanban` (7 tools de board durável) |
| **IA auxiliar** | `vision_analyze`, `image_generate`, `video_analyze` |
| **Mensagens** | `send_message` (qualquer plataforma) |
| **MCP dinâmico** | qualquer tool de MCP server conectado |

### 5.4 Sistema de Cron

Hermes tem um scheduler de cron nativo, não depende do cron do OS.

```yaml
# Exemplo em ~/.hermes/config.yaml
cron:
  jobs:
    - name: backup-codigo
      schedule: "0 3 * * *"          # todo dia às 3h
      prompt: "Faça backup do projeto /home/user/app no S3"
      skills: [aws-s3, git-summary]
      model: claude/claude-haiku-4-5  # modelo barato para tarefas simples

    - name: monitor-uptime
      schedule: "*/5 * * * *"        # a cada 5 minutos
      no_agent: true                  # roda script puro, zero tokens LLM
      script: "curl -sf https://meusite.com || echo DOWN"
      
    - name: relatorio-semanal
      schedule: "0 9 * * 1"          # segunda às 9h
      prompt: "Gere relatório de uso da semana e envia no Telegram"
      delivery: telegram              # entrega na plataforma escolhida
```

**Dois modos de cron:**
- **Full agent**: usa LLM para executar a tarefa (tem custo de tokens)
- **no_agent watchdog**: executa script puro sem chamar LLM (custo zero)

### 5.5 Sistema de Skills (memória procedural)

Skills são documentos Markdown em `~/.hermes/skills/` que ensinam o agent a executar tarefas específicas. Hermes cria skills automaticamente após tarefas complexas (5+ tool calls) e as melhora com o uso.

```
~/.hermes/skills/
├── code/
│   ├── claude-code/          ← Como usar Claude Code via Hermes
│   ├── tdd/                  ← Test-driven development
│   ├── github-pr/            ← Lifecycle completo de PR
│   └── debug-python/         ← Debug com pdb/debugpy
├── infra/
│   ├── docker-deploy/        ← Deploy com Docker
│   ├── aws-s3/               ← Operações S3
│   └── vps-setup/            ← Configurar nova VPS
└── custom/                   ← Skills criadas pelo usuário
```

**Curator automático:** a cada 7 dias, o Hermes Curator roda em background, analisa quais skills estão sendo usadas, consolida as redundantes, arquiva as obsoletas, e escreve um relatório.

### 5.6 Sistema de Memória

Hermes mantém três tipos de memória persistente:

```
MEMORY.md      ← Fatos sobre projetos, preferências técnicas,
                  contexto de longo prazo. Agent atualiza sozinho.

USER.md        ← Perfil do usuário: estilo de trabalho,
                  linguagens preferidas, timezone, objetivos.

sessions/      ← Histórico completo de conversas em SQLite
                  com FTS5 para busca full-text + sumarização LLM.
```

### 5.7 Multi-agent: Kanban + Delegação

**Delegate task (síncrono):**
```
Usuário → Hermes (orchestrator) → spawn subagent-1 (worker)
                                → spawn subagent-2 (worker)
                                → spawn subagent-3 (worker)
                  ← coleta resultados ←
```

**Kanban board (durável, assíncrono):**
```bash
hermes kanban init
hermes kanban create "Refatorar módulo de auth" --workspace /app
hermes kanban dispatch --max 3   # spawna 3 workers paralelos
```

Cada worker tem: heartbeat, zombie detection, retry automático em falha, rollback via checkpoints, e bloqueio se sair sem completar.

### 5.8 Providers de modelo suportados

Hermes conecta a qualquer provider via config.yaml — sem mudar código:

```yaml
inference:
  provider: openrouter          # provider padrão
  model: anthropic/claude-sonnet-4-6
  fallback_providers:
    - provider: anthropic
      model: claude-sonnet-4-6
    - provider: local-litellm
      model: llama4-scout       # fallback para modelo local
```

**Providers suportados nativamente:**
- Nous Portal (modelos próprios)
- OpenRouter (200+ modelos)
- Anthropic direto
- OpenAI direto
- NVIDIA NIM (Nemotron)
- Google (via OpenAI compat)
- Groq
- Ollama (local)
- qualquer endpoint OpenAI-compatível

---

## 6. Claude Code no Stola OS

### 6.1 Papel no sistema

Claude Code é o especialista em código dentro do Stola OS. Enquanto Hermes é o orquestrador 24/7 e operador do OS, Claude Code é o worker de engenharia — lê repositórios inteiros, entende arquitetura, faz mudanças coordenadas em múltiplos arquivos, roda testes, e itera até passar.

### 6.2 Integração Hermes ↔ Claude Code

```
Hermes (orchestrator)
    ↓ delega via MCP bridge
Claude Code (worker especializado em código)
    ↓ executa, testa, commita
Hermes (recebe resultado, decide próximo passo)
```

Na prática, o Hermes agenda tarefas de código para Claude Code via skill:

```
# skill: claude-code/SKILL.md
# Hermes executa este skill quando precisa de trabalho de código

terminal(command="claude -p 'Adicione tratamento de erro em todos 
  os endpoints da API' --allowedTools 'Read,Edit' --max-turns 10",
  workdir="/home/user/app", timeout=300)
```

### 6.3 MCPs compartilhados

Tanto Hermes quanto Claude Code usam os mesmos MCP servers. A infraestrutura de MCP é configurada uma vez no Stola OS e fica disponível para todos os agents.

---

## 7. MCPs do Stola OS

### 7.1 MCPs pré-instalados (padrão — ativos por default)

| MCP Server | O que faz | Package |
|-----------|-----------|---------|
| `filesystem` | Lê/escreve arquivos com permissões configuráveis | `@modelcontextprotocol/server-filesystem` |
| `terminal` | Executa comandos shell no sistema | nativo Hermes |
| `git` | Operações git: clone, commit, push, diff | `@modelcontextprotocol/server-git` |
| `github` | Issues, PRs, repos, Actions via API | `@modelcontextprotocol/server-github` |
| `postgres` | Queries SQL no banco da VPS | `@anthropic-ai/server-postgres` |
| `sqlite` | Queries no banco local Hermes | nativo |
| `web-search` | Busca na web (Parallel/Firecrawl) | nativo Hermes |
| `memory` | Ler/escrever MEMORY.md e USER.md | nativo Hermes |
| `cron` | Gerenciar jobs agendados | nativo Hermes |

### 7.2 MCPs opcionais (usuário instala via painel)

| Categoria | MCPs disponíveis |
|-----------|-----------------|
| **Comunicação** | Slack, Discord, Telegram, WhatsApp, Signal, Email (Himalaya) |
| **Dev tools** | Puppeteer/browser, Docker, Kubernetes, Linear, Jira |
| **Cloud** | AWS (S3, EC2, Lambda), GCP, Azure, Vercel |
| **Dados** | MongoDB, Redis, MySQL, Supabase, PlanetScale |
| **AI/ML** | Hugging Face, Replicate, FAL.ai (imagens/vídeo) |
| **Produtividade** | Google Drive, Notion, Obsidian, Airtable |
| **Monitoramento** | Grafana, Datadog, Sentry, Uptime Robot |

### 7.3 Como MCP funciona no Stola OS

```
Agent (Hermes / Claude Code)
    ↓ chama tool: mcp__github__create_issue
MCP Client (hermes tools/mcp_tool.py)
    ↓ JSON-RPC via stdio ou HTTP/SSE
MCP Server (@modelcontextprotocol/server-github)
    ↓ API call autenticada
GitHub API
    ↑ resposta
MCP Server
    ↑ resultado estruturado
Agent
```

Cada MCP server tem um nome prefixado nas tools: `mcp__<servidor>__<tool>`. Isso evita colisões e deixa visível nos logs de onde veio cada chamada.

---

## 8. Gateway de modelos (LiteLLM)

### 8.1 Papel no Stola OS

O LiteLLM roda numa VPS interna e é o único ponto de acesso a modelos — tanto locais quanto externos. Todo agent, todo usuário, toda VPS fala com o LiteLLM. Ele decide para onde rotear.

### 8.2 Routing logic

```
Request de qualquer VPS
    ↓
LiteLLM proxy (porta 4000)
    ↓
[verifica budget da VPS] → se excedido: 429
    ↓
[verifica provider preferido]
    ├─ modelo local disponível? → vLLM ou Ollama interno
    ├─ provider primário online? → chama diretamente
    └─ fallback automático → próximo provider da lista
```

### 8.3 Configuração multi-tenant

Cada VPS de usuário recebe uma **virtual key** do LiteLLM com:
- Budget mensal em USD
- Modelos permitidos (whitelist)
- Rate limit por minuto
- Provider preferido

```yaml
# config LiteLLM para VPS do usuário #42
model_list:
  - model_name: "llama4-local"
    litellm_params:
      model: "hosted_vllm/meta-llama/Llama-4-Scout"
      api_base: "http://vps-inference:8000"
  
  - model_name: "claude-sonnet"
    litellm_params:
      model: "anthropic/claude-sonnet-4-6"
      api_key: "${ANTHROPIC_KEY}"

router_settings:
  num_retries: 3
  fallbacks: [{"llama4-local": ["claude-sonnet"]}]
```

---

## 9. Inference engines

### 9.1 vLLM (produção)

- **Quando usar:** VPS com múltiplos usuários simultâneos ou workloads pesados
- **Ponto forte:** PagedAttention — processa 180+ requests simultâneos num H100 antes de OOM
- **Throughput:** ~187 tok/s no A100 com Llama 3.1 8B (vs ~80 tok/s do Ollama)
- **Setup no Stola OS:**

```bash
# Iniciado automaticamente se GPU disponível
vllm serve meta-llama/Llama-4-Scout-17B-16E-Instruct \
  --quantization fp8 \
  --max-model-len 32768 \
  --port 8000 \
  --host 0.0.0.0
```

### 9.2 Ollama (dev/testes)

- **Quando usar:** VPS individual, dev, modelos menores, setup rápido
- **Ponto forte:** 1 comando instala e roda qualquer modelo; suporta cloud offload
- **Limite:** queues requests em vez de processar em paralelo — cai com 5+ usuários simultâneos

### 9.3 SGLang (agentes de longa duração)

- **Quando usar:** Hermes Agent rodando tasks de múltiplos turnos, workflows longos
- **Ponto forte:** otimizado para multi-turn e workloads agentic, melhor que vLLM para sessions longas

---

## 10. Painel de controle (Stola Dashboard)

### 10.1 Seções do painel

```
/dashboard
├── /vps              ← gerenciar suas VPS (criar, pausar, deletar)
├── /vps/:id
│   ├── overview      ← CPU, RAM, GPU, custo em tempo real
│   ├── terminal      ← terminal web direto na VPS
│   ├── agents        ← status do Hermes, Claude Code
│   ├── models        ← modelos instalados, trocar modelo padrão
│   ├── mcps          ← MCPs ativos + marketplace
│   ├── crons         ← jobs agendados, histórico de runs
│   ├── skills        ← biblioteca de skills, instalar novos
│   ├── memory        ← visualizar MEMORY.md e USER.md
│   └── logs          ← traces LLM via Langfuse, logs do sistema
├── /marketplace      ← MCPs, skills, e templates de VPS
└── /billing          ← uso de tokens, custo por provider
```

### 10.2 Acesso ao agent

O usuário pode interagir com o Hermes da sua VPS por qualquer canal:

| Canal | Como acessa |
|-------|------------|
| **Painel web** | Chat embed no dashboard |
| **TUI** | SSH na VPS + `hermes --tui` |
| **Telegram** | Adiciona o bot do Hermes no Telegram |
| **Discord** | Bot no server do usuário |
| **Slack** | App workspace |
| **WhatsApp** | Número configurado |
| **API REST** | `POST /v1/chat/completions` (OpenAI compat) |

---

## 11. Segurança

### 11.1 Isolamento de VPS

- Cada VPS em LXC unprivileged ou VM KVM — sem acesso ao host
- Rede interna isolada por VLAN por usuário
- Filesystem separado — nenhuma VPS monta path de outra
- GPU: passthrough exclusivo ou time-slice via nvidia MIG

### 11.2 Segurança do Hermes Agent

O Hermes v0.13 fechou 8 vulnerabilidades P0:

- **Redação ativa por padrão:** dados sensíveis são redactados em logs antes de qualquer upload
- **WhatsApp rejeita estranhos por padrão:** só aceita mensagens de contatos autorizados
- **Discord role-allowlists por guild:** CVSS 8.1 de bypass cross-guild fechado
- **TOCTOU fechado:** race conditions em auth.json e MCP OAuth eliminados
- **Browser bloqueia SSRF:** cloud metadata endpoints bloqueados no browser tool
- **Cron escaneia prompt injection:** conteúdo de skills no cron é escaneado antes de rodar
- **`hermes debug share` redacta no upload:** nunca vaza chaves em debug shares

### 11.3 Sandboxing de código

Código gerado por AI roda em ambiente isolado:

```
execute_code tool → Python sandboxado com:
  - sem acesso a filesystem fora do workdir
  - sem network por padrão (opt-in)
  - timeout configurável
  - acesso a tools Hermes via RPC (não acesso direto)
```

Para Claude Code: Bubblewrap no Linux (sandbox nativo), com opção de rodar dentro de container Docker na VPS.

---

## 12. Stack de observabilidade

| Componente | Papel | Acesso |
|-----------|-------|--------|
| **Langfuse** | Traces de LLM: tokens, custo, latência por request | Dashboard Stola |
| **Grafana** | Métricas de GPU, CPU, RAM, rede das VPS | Dashboard Stola |
| **Postgres** | Spend tracking por VPS, histórico de usage | Interno |
| **Redis** | Cache de respostas LLM, rate limiting | Interno |
| **Hermes logs** | `~/.hermes/logs/` — log de cada turn do agent | SSH ou dashboard |

---

## 13. Roadmap de implementação

### Fase 1 — Base (MVP)
- [ ] Host Linux + Docker/containerd
- [ ] Workspace engine: criar/pausar/deletar containers Docker com limites e auditoria
- [ ] LiteLLM gateway + Postgres + Redis numa VPS interna
- [ ] Integração com Ollama como inference engine padrão
- [ ] Hermes Agent instalado automaticamente em cada workspace
- [ ] OpenCode e OpenClaw instalados automaticamente em cada workspace
- [ ] Painel web básico: criar workspace, abrir terminal, ver status

### Fase 2 — Inference + MCPs
- [ ] vLLM com GPU para hosts/workspaces de produção
- [ ] Catálogo de modelos: pull via painel (Llama 4, Qwen3, DeepSeek...)
- [ ] MCPs padrão pré-configurados (filesystem, git, github, postgres, web)
- [ ] Marketplace de MCPs opcionais com 1-click install
- [ ] Integração Claude Code como agent de código

### Fase 3 — AI OS completo
- [ ] Wizard de criação de VPS com seleção de specs + software
- [ ] Cloud-init gerado automaticamente pelo wizard
- [ ] Cron engine visual no dashboard
- [ ] Skills marketplace (agentskills.io compatible)
- [ ] Multi-agent Kanban boards por VPS
- [ ] Perfis de VPS pre-configurados: "Dev", "Produção", "Data Science"

### Fase 4 — Self-hosting + escala
- [ ] Multi-node: cluster de hosts Docker primeiro; Proxmox/KVM para plano Pro
- [ ] Billing por token, por VPS, por modelo
- [ ] Self-evolution: Hermes Curator gerencia skills automaticamente
- [ ] Hermes como API pública da VPS (OpenAI compat endpoint por VPS)

---

## 14. Comandos fundamentais do Stola OS

```bash
# Criar VPS via CLI (alternativa ao painel web)
stola vps create --ram 8g --cpu 4 --disk 50g --gpu shared

# Ver VPS ativas
stola vps list

# Entrar na VPS
stola vps ssh <vps-id>

# Instalar MCP na VPS atual
stola mcp install github
stola mcp install postgres --connection-string postgresql://...

# Gerenciar modelos
stola models list          # modelos disponíveis
stola models pull llama4   # baixar modelo
stola models use claude-sonnet  # trocar modelo padrão do Hermes

# Dentro da VPS — comandos Hermes
hermes setup               # wizard inicial
hermes --tui               # interface React/Ink
hermes -z "faz isso"       # one-shot sem interação
hermes model               # trocar modelo
hermes cron list           # ver jobs agendados
hermes kanban init         # iniciar board multi-agent
hermes skills list         # ver skills disponíveis
hermes mcp serve           # expor Hermes como MCP server
hermes profile create dev  # criar perfil isolado
```

---

## 15. Glossário

| Termo | Definição |
|-------|-----------|
| **VPS** | Container/VM isolado dentro do Stola OS com recursos dedicados |
| **Agent** | Processo de AI autônomo rodando dentro de uma VPS |
| **Hermes Agent** | Runtime de AI da NousResearch — o coração operacional do Stola OS |
| **Claude Code** | Agent especializado em engenharia de software |
| **MCP** | Model Context Protocol — protocolo USB-C que conecta agents a ferramentas |
| **MCP Server** | Processo que expõe ferramentas via MCP (ex: servidor GitHub) |
| **Skill** | Documento Markdown que ensina o agent a executar um tipo de tarefa |
| **Curator** | Background process que melhora automaticamente a biblioteca de skills |
| **Cron** | Scheduler nativo do Hermes para tarefas recorrentes |
| **Kanban** | Board multi-agent durável para coordenar trabalho paralelo |
| **LiteLLM** | Proxy/gateway que unifica acesso a 100+ providers de LLM |
| **vLLM** | Engine de inference de alta performance para produção |
| **Virtual key** | Chave de API por VPS com budget e permissões isolados |
| **no_agent mode** | Cron job que roda script puro sem custo de tokens LLM |

---

*Stola OS v0.1 — Documentação fundacional*
*Baseado em: Hermes Agent v2026.5.7, LiteLLM v1.x, vLLM v0.8.x, Proxmox VE 9.1*
*Maio 2026*
