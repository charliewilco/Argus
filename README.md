# Argus

Argus is automation infrastructure written in Go. It is intended to ship as a single binary that manages OAuth connections, ingests provider events, executes declarative pipelines, and dispatches actions to external services.

This repository is now on the Go rewrite path. The current foundation in place is:
- Core envelope and schema types
- Connection and pipeline domain models
- `Store` interface
- SQLite-backed store with migrations
- In-memory queue implementation
- OAuth token encryption, PKCE state management, and refresh handling
- Spec-shaped HTTP router with placeholder endpoints
- Minimal server and CLI entrypoints

## Quick Start

```bash
go test ./...
ARGUS_SECRET_KEY=development-secret go run ./cmd/argus
go run ./cmd/argus-cli --help
```

The server currently exposes `GET /healthz` and a scaffolded API surface. Provider-backed OAuth routes, webhook ingestion, and pipeline execution are not wired yet.

## Current Status

The repository has been converted away from the previous Bun/TypeScript prototype. The current foundation now includes queueing and encrypted OAuth token storage, so the next meaningful layer is provider integration and pipeline execution rather than more infrastructure churn.

## Layout

```text
cmd/
  argus/
  argus-cli/
config/
docs/
internal/
  api/
  connections/
  envelope/
  oauth/
  queue/
  pipeline/
  schema/
  store/
migrations/
providers/
  github/
```

## Verification

These commands are expected to pass before shipping changes:

```bash
gofmt -w .
go test ./...
go build ./...
go vet ./...
```
