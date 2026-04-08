# Argus Specification

## Goal

Argus is a Go service and CLI for automation infrastructure. It provides:
- OAuth-backed provider connections
- Event ingestion via webhooks and polling
- Declarative workflow pipelines
- Provider action dispatch
- Event persistence, replay, and DLQ support

The system is designed to run as a single binary with SQLite by default and minimal external dependencies.

## Current Implementation Slice

The repository is being rebuilt in the following order:
1. `internal/envelope`
2. `internal/schema`
3. `internal/store`
4. `internal/queue`
5. `internal/oauth`
6. `internal/connections`
7. `providers/github`
8. `internal/triggers`
9. `internal/pipeline`
10. `internal/actions`
11. `internal/dlq`
12. `internal/api`
13. `cmd/argus`
14. `cmd/argus-cli`
15. Additional providers

The repository currently has steps 1 through 14 in place, with constructor-wired `cmd/argus` and `cmd/argus-cli` entrypoints plus `internal/api`.

Implemented foundation:
- `internal/queue` with an in-memory queue
- `internal/oauth` with PKCE auth state, encrypted token persistence, and token refresh support
- `internal/store/sqlite` with embedded SQL migrations
- `internal/api` with spec-shaped routes backed by constructor-injected services
- `cmd/argus` server wiring for config, store, providers, OAuth, router, and HTTP lifecycle
- `cmd/argus-cli` command wiring with a health command against `/healthz`

## Storage Model

Argus currently persists these core entity families:
- `events`
- `connections`
- `connection` secrets as encrypted token ciphertext
- `oauth_states`
- `pipelines`

SQLite is the default store. Raw SQL and migrations are the source of truth.

## Queue Model

The queue contract is now defined and backed by an in-memory implementation. Jobs support:
- enqueue
- blocking dequeue
- ack
- nack with retry metadata

Redis remains a later follow-up rather than a prerequisite for the current architecture.

## OAuth Model

`internal/oauth` now owns:
- PKCE state generation and expiry
- authorization URL generation
- token exchange
- AES-GCM encryption at rest for persisted tokens
- refresh-on-read when a token is within the configured leeway window

The store layer persists opaque ciphertext and expiring OAuth state records; it does not own encryption logic.

## Design Constraints

- No global state
- Constructor-driven dependencies
- Wrapped errors with package context
- No panics in library code
- UTC timestamps everywhere
- Provider packages remain isolated from one another

## Near-Term Follow-Up

- Add a provider registry and the first real GitHub provider
- Replace API OAuth placeholders with handlers wired to provider metadata
- Add trigger execution and pipeline replay
- Add action dispatch and DLQ handling
