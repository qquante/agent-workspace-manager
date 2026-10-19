# Agent Workspace Manager

A Go control plane for isolated Docker workspaces, model access policies and inspectable agent operations.

![review](https://img.shields.io/badge/review-private-374151) ![primary](https://img.shields.io/badge/primary-Go-2563EB) [![LinkedIn](https://img.shields.io/badge/LinkedIn-Douglas%20Silva-0A66C2?logo=linkedin&logoColor=white)](https://www.linkedin.com/in/douglas-silva-data-analyst/)

[Overview](#overview) · [Architecture](#architecture) · [Setup](#setup) · [Validation](#validation) · [Roadmap](#roadmap) · [Review notes](docs/REVIEW.md)

## Overview

- Workspace and provisioning modules separate the control plane from user containers.
- Proxy modules implement provider access and usage controls.
- Audit and MCP policy modules expose operational boundaries.

**Technology:** Go 1.25 · Docker · SQLite · YAML · HTTP proxy · MCP policy controls

**Persistence and data:** SQLite is referenced through modernc.org/sqlite; YAML files describe users and policies.

## Architecture

```text
User/policy configuration → control plane → isolated workspace and provider proxy → usage/audit records
```

| Location | Review focus |
| --- | --- |
| [`go.mod`](go.mod) | Source/configuration inspected for scope and structure |
| [`internal/audit`](internal/audit) | Source/configuration inspected for scope and structure |
| [`internal`](internal) | Source/configuration inspected for scope and structure |
| [`cmd`](cmd) | Source/configuration inspected for scope and structure |
| [`examples`](examples) | Source/configuration inspected for scope and structure |

Internal package and module names are retained where changing them would affect imports or compatibility. This descriptive repository name does not imply a completed package/API migration.

## Setup

Install the Go version declared in go.mod and Docker for workspace integration. Review cmd/ and examples/ before starting any provisioning command.

Start from an isolated environment. Use `.env.example` when present and inspect the configuration loader: an example file is not necessarily loaded automatically. Runtime databases, real account sessions and private credentials are not bundled. Live integration prerequisites are separate from local tests.

## Validation

Static parsing checked 0 Python files and 1 strict JSON files. Python syntax errors: 0; JSON errors: 0. No functional test/build result is claimed for this preparation.

Available test scripts are inventoried in [the review notes](docs/REVIEW.md). Choose offline checks before operating any integration.

These results apply to the selected checks on the preparation copy. They do not establish deployment readiness, full coverage, live provider availability or measured performance. CI definitions, when present under `docs/ci-templates/`, are inactive review templates; no remote CI badge is claimed.

## Limits and current status

The current direction is a Docker workspace MVP. Earlier operating-system and virtualization concepts in design notes are not delivered features. Go is not installed in the review environment.

This private snapshot is for code and portfolio review. Generated runtime material and unreviewed binary assets may be excluded. [Review notes](docs/REVIEW.md) record preparation scope, findings and outstanding validation.

## Roadmap

- [ ] Run go test ./... with the declared Go toolchain.
- [ ] Document the smallest local Docker example and its isolation limits.
- [ ] Review proxy authentication, persistence and MCP policy failure paths.

## History and attribution

No recoverable original Git history was verified for this snapshot. Import and preparation commits use their actual current dates; filesystem timestamps are not treated as development dates.

No open-source license is granted by this preparation. A license and attribution review is required before public redistribution.

## Maintainer and feedback

Douglas de Oliveira Silva · [GitHub](https://github.com/qquante) · [LinkedIn](https://www.linkedin.com/in/douglas-silva-data-analyst/)

For this private review, report reproducible issues with a minimal fixture and the exact check output. Keep account data, credentials and generated sensitive reports out of issues and commits. Existing contribution guidelines apply where present.

**Topics:** `go`, `docker`, `sqlite`, `mcp`, `developer-tools`, `ai-agents`, `llm-proxy`
