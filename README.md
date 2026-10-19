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

The current direction is a Docker workspace MVP. Earlier operating-system and virtualization concepts in design notes are not delivered features. Go is not installed in the review environment.

## License

No open-source license has been granted. Redistribution terms remain to be defined.
