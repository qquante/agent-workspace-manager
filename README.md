# Agent Workspace Manager

A Go control plane for isolated Docker workspaces, model access policies and inspectable agent operations.

## Features

- Workspace and provisioning modules separate the control plane from user containers.
- Proxy modules implement provider access and usage controls.
- Audit and MCP policy modules expose operational boundaries.

## Stack

Go 1.25 · Docker · SQLite · YAML · HTTP proxy · MCP policy controls

SQLite is referenced through modernc.org/sqlite; YAML files describe users and policies.

## Getting started

Install the Go version declared in go.mod and Docker for workspace integration. Review cmd/ and examples/ before starting any provisioning command.

## How it works

```text
User/policy configuration → control plane → isolated workspace and provider proxy → usage/audit records
```

## Current limitations

The current direction is a Docker workspace MVP. Earlier operating-system and virtualization concepts in design notes are not delivered features. Unit tests use mocked runners and local test servers; real workspace provisioning has not been validated.

## Validation

Go 1.27.3 on Windows: all 12 packages with tests passed. Tests use temporary stores, mocked Docker runners and local HTTP servers.

```console
go test -count=1 ./...
```

## License

Project-authored source code is licensed under the [MIT License](LICENSE). Third-party components and datasets retain their respective rights and licenses.
