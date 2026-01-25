![](.github/header.png)

# Argus

Argus is a Bun-native TypeScript monorepo for monitoring external systems via event triggers (webhooks + polling). It produces a common `EventEnvelope` and delivers events with dedupe, retries/backoff, DLQ, and replay.

## Highlights
- Provider-based architecture with a shared trigger interface
- Bun workspaces, ESM-only
- Testable packages that can run in isolation

## Monorepo Layout
```
apps/
  example/
packages/
  core/
  runtime/
  storage/
    memory/
    sqlite/
  queue/
    memory/
  providers/
    github/
  cli/
```

## Quick Start
```bash
bun install
bun test
```

Run a workspace test:
```bash
bun test packages/core
```

## Packages
- `@argus/core`: core types, provider and trigger interfaces
- `@argus/runtime`: runtime pipeline and delivery orchestration
- `@argus/storage-*`: event store implementations
- `@argus/queue-*`: delivery queue implementations
- `@argus/provider-github`: example provider
- `@argus/cli`: replay and DLQ tooling (planned)

## Example App
See `apps/example` for a Bun server wiring `Runtime` to a webhook route.

## Documentation
- Spec and architecture details: `SPEC.md`
- Contributor guide: `AGENTS.md`
- Project docs: `docs/README.md`

## License
TBD
