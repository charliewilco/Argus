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

## Usage

```typescript
import { Runtime } from "@argus/runtime/runtime";
import { MemoryEventStore } from "@argus/storage-memory";
import { MemoryQueue } from "@argus/queue-memory";
import { GitHubProvider } from "@argus/provider-github";

const runtime = new Runtime({
  eventStore: new MemoryEventStore(),
  queue: new MemoryQueue(),
});

runtime.registerProvider(new GitHubProvider());

runtime.registerConnection({
  tenantId: "acme",
  connectionId: "gh-main",
  provider: "github",
  auth: { token: process.env.GITHUB_TOKEN },
  config: { repoFullName: "acme/backend" },
});

runtime.onEvent(async (event) => {
  console.log(event.type, event.data.normalized);
});

// Poll for updated issues every 30 s
runtime.startPolling();

// Accept a webhook (call from your HTTP handler)
await runtime.handleWebhook({
  provider: "github",
  triggerKey: "issue.created",
  body: parsedBody,
  headers: requestHeaders,
  tenantId: "acme",
  connectionId: "gh-main",
});
```

## Packages
- `@argus/core`: core types, provider and trigger interfaces
- `@argus/runtime`: runtime pipeline and delivery orchestration
- `@argus/storage-*`: event store implementations
- `@argus/queue-*`: delivery queue implementations
- `@argus/provider-github`: example provider
- `@argus/cli`: replay and DLQ tooling

## Example App
See `apps/example` for a Bun server wiring `Runtime` to a webhook route.

## Documentation
- Spec and architecture details: `SPEC.md`
- Contributor guide: `AGENTS.md`
- Project docs: `docs/README.md`

## License
TBD
