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

## Quick Start

Install dependencies and run all tests:

```bash
bun install
bun test
```

### Minimal runtime setup

```typescript
import { Runtime } from "@argus/runtime/runtime";
import { MemoryEventStore } from "@argus/storage-memory";
import { MemoryQueue } from "@argus/queue-memory";
import { GitHubProvider } from "@argus/provider-github";

// 1. Create storage and queue
const eventStore = new MemoryEventStore();
const queue = new MemoryQueue();

// 2. Create the runtime
const runtime = new Runtime({ eventStore, queue });

// 3. Register a provider
runtime.registerProvider(new GitHubProvider());

// 4. Register a connection (tenant ↔ provider binding)
runtime.registerConnection({
  tenantId: "acme",
  connectionId: "gh-main",
  provider: "github",
  auth: { token: process.env.GITHUB_TOKEN },
  config: { repoFullName: "acme/backend" },
});

// 5. Handle incoming events
runtime.onEvent(async (event) => {
  console.log(event.type, event.data.normalized);
});

// 6. Start polling (for poll/hybrid triggers)
runtime.startPolling();
```

### Accepting webhooks

```typescript
const result = await runtime.handleWebhook({
  provider: "github",
  triggerKey: "issue.created",
  body: parsedJsonBody,
  headers: { "x-github-delivery": "abc123", /* ... */ },
  tenantId: "acme",
  connectionId: "gh-main",
});

if (!result.accepted) {
  console.error("Rejected:", result.reason);
}
```
