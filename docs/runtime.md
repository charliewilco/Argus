# Runtime

`@argus/runtime` orchestrates triggers, delivery, retries/backoff, and replay.

## Setup

```typescript
import { Runtime } from "@argus/runtime/runtime";
import { MemoryEventStore } from "@argus/storage-memory";
import { MemoryQueue } from "@argus/queue-memory";

const runtime = new Runtime({
  eventStore: new MemoryEventStore(),
  queue: new MemoryQueue(),
  maxAttempts: 5,          // default: 5 — retry attempts before DLQ
  pollIntervalMs: 30_000,  // default: 30 000ms — polling cycle interval
  tenantScope: "acme",     // optional: lock runtime to one tenant
});
```

## Lifecycle

- `registerProvider(provider)`
- `registerConnection(connection)`
- `handleWebhook(...)` for webhook triggers
- `startPolling()` / `stopPolling()` for poll triggers

When a trigger is first used for a connection, the runtime calls `trigger.setup()` and stores any returned state. On `unregisterConnection(...)`, the runtime calls `trigger.teardown()` and clears state.

```typescript
import { GitHubProvider } from "@argus/provider-github";

// Register provider
runtime.registerProvider(new GitHubProvider());

// Register connection
runtime.registerConnection({
  tenantId: "acme",
  connectionId: "gh-main",
  provider: "github",
  auth: { token: process.env.GITHUB_TOKEN, webhookSecret: process.env.GITHUB_WEBHOOK_SECRET },
  config: { repoFullName: "acme/backend" },
});

// Subscribe to delivered events
runtime.onEvent(async (event) => {
  console.log("Received:", event.type, event.data.normalized);
});

// Start polling for poll/hybrid triggers
runtime.startPolling();

// Handle a webhook (e.g. from an HTTP route)
const result = await runtime.handleWebhook({
  provider: "github",
  triggerKey: "issue.created",
  body: parsedBody,
  headers: requestHeaders,
  tenantId: "acme",
  connectionId: "gh-main",
});
// result: { accepted: true } or { accepted: false, reason: "..." }

// Remove a connection (calls trigger teardown)
await runtime.unregisterConnection("acme", "gh-main");

// Clean up timers when shutting down
runtime.shutdown();
```

## Tenant scope
Runtime can be constructed with a `tenantScope` option to enforce strict tenant isolation. When set, connections and webhook inputs for other tenants are rejected, and replay operations are scoped to the tenant.

```typescript
// Single-tenant runtime — rejects anything not belonging to "acme"
const runtime = new Runtime({
  eventStore,
  queue,
  tenantScope: "acme",
});
```

## Trigger versioning
If multiple trigger versions share the same key, the runtime selects the latest version by default. Webhook callers can pass an explicit `triggerVersion` to target a specific version.

```typescript
// Explicitly target trigger version "2"
await runtime.handleWebhook({
  provider: "github",
  triggerKey: "issue.created",
  triggerVersion: "2",
  body,
  headers,
  tenantId: "acme",
  connectionId: "gh-main",
});
```

## Delivery
- Dedupe happens before storage using `(provider, tenantId, connectionId, dedupeKey)`.
- Events are stored, then delivery jobs are queued.
- Delivery retries with exponential backoff until `maxAttempts`.
- Failed events are written to the DLQ.

Backoff schedule (attempt → delay):
| Attempt | Delay    |
|---------|----------|
| 2       | 1 s      |
| 3       | 2 s      |
| 4       | 4 s      |
| 5       | 8 s      |
| 6+      | 60 s max |

## Replay

Runtime exposes `replay(...)` and `replayDLQ(...)` helpers for re-queueing stored events.

```typescript
// Replay all events from a time range
const count = await runtime.replay({
  since: "2024-01-01T00:00:00Z",
  until: "2024-02-01T00:00:00Z",
  tenantId: "acme",
  connectionId: "gh-main",
  normalized: { repoFullName: "acme/backend" }, // exact match filter
});
console.log(`Re-queued ${count} events`);

// Replay specific events by ID
await runtime.replayByIds(["evt_abc123", "evt_def456"]);

// Replay all DLQ entries for a tenant
const dlqCount = await runtime.replayDLQ({ tenantId: "acme" });
```
