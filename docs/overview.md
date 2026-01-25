# Overview

Argus is a Bun-native TypeScript library for monitoring external systems via provider triggers.

Key ideas:
- Providers implement triggers (webhook, poll, or hybrid).
- Triggers output `EventEnvelope` objects.
- Runtime handles dedupe, retries/backoff, DLQ, and replay.
- Storage and queue are pluggable packages.

Monorepo layout:
- `packages/core`: shared types and interfaces
- `packages/runtime`: orchestration and delivery
- `packages/storage/*`: event store implementations
- `packages/queue/*`: delivery queue implementations
- `packages/providers/*`: provider libraries
- `packages/cli`: replay and DLQ tooling
- `apps/example`: Bun server integration example
