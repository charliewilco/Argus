# Argus — Bun + TypeScript Monorepo Spec

Provider-library based monitoring and trigger system.

## 1. Goal
Argus is a Bun-native TypeScript library for monitoring external systems via event triggers (webhook + polling). It produces a common `EventEnvelope` and delivers events with:
- dedupe
- retries / backoff
- DLQ
- replay

Providers are independent workspace libraries that conform to a shared Provider interface and can be tested in isolation.

## 2. Non-Goals
- No web framework dependency
- No UI / billing / marketplace
- No SOC2 / compliance
- No ordering guarantees
- No full payload normalization

## 3. Monorepo Structure (Bun workspaces)
```
argus/
  bun.lock
  package.json

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

  apps/
    example/
```

Rules:
- Each folder is a real workspace with its own `package.json`.
- Imports use workspace names (examples: `@argus/core`, `@argus/provider-github`).
- No TS path aliases.
- Each package is testable independently with `bun test`.

## 4. Tooling
- Bun workspaces
- ESM only
- Tests: `bun test`
- Runtime TS executed directly by Bun

## 5. Core Package (`@argus/core`)

### 5.1 `EventEnvelope`
`packages/core/src/event.ts`

```ts
export type EventEnvelope<TNormalized = unknown, TRaw = unknown> = {
  id: string
  type: string
  occurredAt: string
  receivedAt: string
  provider: string
  triggerKey: string
  triggerVersion: string
  tenantId: string
  connectionId: string
  dedupeKey: string
  data: { normalized?: TNormalized; raw: TRaw }
  meta: Record<string, unknown>
}
```

### 5.2 `TriggerDefinition`
`packages/core/src/trigger.ts`

```ts
export type TriggerMode = "webhook" | "poll" | "hybrid"

export interface TriggerDefinition<TConfig = unknown, TState = unknown> {
  provider: string
  key: string
  version: string
  mode: TriggerMode

  validateConfig?(config: unknown): asserts config is TConfig

  setup(ctx: SetupContext<TConfig>): Promise<{ state?: TState }>
  teardown(ctx: TeardownContext<TState>): Promise<void>

  ingest?(ctx: WebhookContext<TConfig, TState>): Promise<void>
  poll?(ctx: PollContext<TConfig, TState>): Promise<{ state?: TState }>

  transform(input: TransformInput): Promise<EventEnvelope[]>
  dedupe(event: EventEnvelope): string
}
```

### 5.3 Provider Interface
`packages/core/src/provider.ts`

```ts
export interface Provider {
  readonly name: string
  readonly version: string

  getTriggers(): TriggerDefinition[]
  validateConnection(connection: unknown): asserts connection is Connection

  testConnection?(connection: Connection):
    Promise<{ ok: true } | { ok: false; error: string }>
}
```

### 5.4 `AbstractProvider` (optional)
`packages/core/src/abstractProvider.ts`

```ts
export abstract class AbstractProvider implements Provider {
  abstract name: string
  abstract version: string
  protected triggers: TriggerDefinition[] = []

  getTriggers() { return this.triggers }
  protected registerTrigger(t: TriggerDefinition) { this.triggers.push(t) }

  abstract validateConnection(connection: unknown): asserts connection is Connection
}
```

### 5.5 `Connection`
`packages/core/src/connection.ts`

```ts
export type Connection = {
  tenantId: string
  connectionId: string
  provider: string
  auth: Record<string, unknown>
  config?: Record<string, unknown>
}
```

### 5.6 Event IDs
`packages/core/src/id.ts`
- `createEventId(provider, connectionId, dedupeKey)` → SHA-256
- Implemented with `Bun.crypto.subtle`

## 6. Runtime (`@argus/runtime`)

### 6.1 Runtime API
`packages/runtime/src/runtime.ts`

```ts
export class Runtime {
  constructor(opts: {
    eventStore: EventStore
    queue: Queue
    maxAttempts?: number
  })

  registerProvider(provider: Provider): void
  registerConnection(connection: Connection): void
  onEvent(handler: (e: EventEnvelope) => Promise<void> | void): void

  handleWebhook(input: {
    provider: string
    triggerKey: string
    body: unknown
    headers: Record<string, string>
    tenantId: string
    connectionId: string
  }): Promise<{ accepted: boolean; reason?: string }>

  startPolling(): void
  stopPolling(): void
}
```

Runtime does not host HTTP.

### 6.2 Pipeline
1. Validate provider + connection.
2. Resolve trigger.
3. Call `ingest()` if present.
4. Call `transform()`.
5. Compute `dedupeKey` + event id.
6. `EventStore.hasDedupe()`.
7. Store event.
8. Enqueue delivery job.
9. Retry with backoff.
10. DLQ on max attempts.

Semantics:
- At-least-once delivery
- Dedupe on (provider, connectionId, dedupeKey)
- No ordering guarantees

## 7. Storage

### 7.1 `EventStore` interface
`packages/core/src/eventStore.ts`

```ts
export interface EventStore {
  put(event: EventEnvelope): Promise<void>
  get(id: string): Promise<EventEnvelope | null>
  hasDedupe(provider: string, connectionId: string, dedupeKey: string): Promise<boolean>

  markDelivery(
    id: string,
    attempt: number,
    status: "delivered" | "failed",
    error?: string
  ): Promise<void>

  putDLQ(id: string, reason: string): Promise<void>
  listDLQ(filters?: { tenantId?: string; connectionId?: string }):
    Promise<Array<{ eventId: string; reason: string }>>

  list(filters?: {
    since?: string
    until?: string
    tenantId?: string
    connectionId?: string
  }): Promise<EventEnvelope[]>
}
```

### 7.2 Implementations
- `@argus/storage-memory` (week 1)
- `@argus/storage-sqlite` (week 2)

## 8. Queue

### 8.1 `Queue` interface
`packages/core/src/queue.ts`

```ts
export interface Queue {
  enqueue(job: DeliveryJob): Promise<void>
  lease(limit: number): Promise<DeliveryJob[]>
  ack(jobId: string): Promise<void>
  fail(jobId: string, error: string): Promise<void>
}
```

### 8.2 Implementation
- `@argus/queue-memory` (single worker loop, `setTimeout` backoff)

## 9. Providers

### 9.1 GitHub Provider
`@argus/provider-github`

Exports:
- `GitHubProvider`
- `verifyGitHubSignature(...)`

Triggers:
- Webhook: `issue.created` (week 1)
- Polling: `issues.updated` (week 2)

Webhook trigger:
- Dedupe by delivery id header.
- Fallback: `issue.id + updated_at`.
- Normalized subset:
  - `repoFullName`
  - `issueNumber`
  - `title`
  - `userLogin`
  - `url`

Polling trigger:
- Cursor: `{ since: ISO }`.
- Default lookback: 24h.
- Pagination required.

## 10. Example App (`apps/example`)
- Uses `Bun.serve()`.
- Registers runtime, `GitHubProvider`, and one connection.
- Route: `POST /webhooks/github/issue.created`.
- Verifies signature via provider helper.
- Calls `runtime.handleWebhook(...)`.
- Prints events.
- Optional forced handler failure for retry/DLQ testing.

## 11. CLI (`@argus/cli`) — Week 2+
Commands:
- `argus replay --since --until`
- `argus dlq list`
- `argus dlq replay --event <id>`

## 12. Milestones

Week 1:
- core types
- runtime pipeline
- memory store + queue
- GitHub webhook trigger
- Bun example app
- dedupe works
- basic tests

Week 2:
- SQLite store
- polling scheduler
- GitHub polling trigger
- retry + DLQ + replay
- CLI

Week 3:
- strict tenant isolation
- trigger versioning
- basic filters on `data.normalized`
- optional scaffolding helpers

## 13. Build Order
1. `@argus/core`
2. `@argus/runtime`
3. memory store + queue
4. GitHub provider
5. example app
6. tests
7. sqlite store
8. polling
9. replay + DLQ
10. week-3 extras
