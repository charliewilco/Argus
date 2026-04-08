# Repository Guidelines

## Project Structure
- Go module rooted at `github.com/charliewilco/argus`
- `cmd/argus` contains the server entrypoint
- `cmd/argus-cli` contains the CLI entrypoint
- `internal/` contains application packages
- `internal/oauth` owns PKCE state, token encryption, and refresh behavior
- `internal/queue` owns execution job primitives and in-memory queue behavior
- `providers/` contains provider implementations
- `migrations/` contains SQL migrations

## Build And Test
- `go test ./...`
- `go build ./...`
- `go vet ./...`
- `gofmt -w .`

Run all relevant checks before handing off changes unless the task is documentation-only.

## Coding Conventions
- Prefer interfaces before implementations
- Use raw SQL; do not introduce an ORM
- Wrap errors with package context
- Keep time values in UTC
- Avoid package cycles; provider packages must not import each other

## Architecture Notes
- `internal/store` owns persistence contracts
- `internal/oauth` owns token refresh and encryption
- `internal/store` persists opaque encrypted token bytes; it should not know how to encrypt or refresh them
- `internal/queue` should stay interface-first so Redis can slot in later without changing callers
- `internal/pipeline` executes workflows through provider interfaces rather than direct provider imports

## Agent Instructions
- Follow the implementation order in `SPEC.md`
- Keep changes small and reversible
- When in doubt, preserve long-term leverage over short-term convenience
- Do not reintroduce plaintext token storage through connection persistence
