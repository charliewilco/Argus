# Storage

Event stores implement `EventStore` from `@argus/core/eventStore`.

## Implementations
- `@argus/storage-memory`: in-memory store for tests/examples
- `@argus/storage-sqlite`: SQLite-backed store

Event stores support:
- dedupe lookup
- delivery attempt tracking
- DLQ listing
- replay queries by time, connection, and normalized field filters

## MemoryEventStore

Use this for tests and local development. State is lost when the process exits.

```typescript
import { MemoryEventStore } from "@argus/storage-memory";
import { Runtime } from "@argus/runtime/runtime";
import { MemoryQueue } from "@argus/queue-memory";

const runtime = new Runtime({
  eventStore: new MemoryEventStore(),
  queue: new MemoryQueue(),
});
```

## SQLiteEventStore

Use this for production or any scenario where events must survive process restarts.

```typescript
import { SQLiteEventStore } from "@argus/storage-sqlite";
import { Runtime } from "@argus/runtime/runtime";
import { MemoryQueue } from "@argus/queue-memory";

const eventStore = new SQLiteEventStore("./argus.sqlite");

const runtime = new Runtime({
  eventStore,
  queue: new MemoryQueue(),
});
```

The SQLite store creates its schema automatically on first use. The database file path is relative to the working directory, or use an absolute path.

## Choosing a store

| Use case                     | Store           |
|------------------------------|-----------------|
| Unit / integration tests     | `MemoryEventStore` |
| Local development            | `MemoryEventStore` or `SQLiteEventStore` |
| Production / persistent data | `SQLiteEventStore` |
| CLI replay and DLQ commands  | `SQLiteEventStore` (required) |

## Direct store access (advanced)

The event store is normally used by the runtime, but you can query it directly if needed:

```typescript
// List events with filters
const events = await eventStore.list({
  since: "2024-01-01T00:00:00Z",
  until: "2024-02-01T00:00:00Z",
  tenantId: "acme",
  connectionId: "gh-main",
  normalized: { repoFullName: "acme/backend" },
});

// Fetch a single event by ID
const event = await eventStore.get("evt_abc123");

// List dead-lettered events
const dlqEntries = await eventStore.listDLQ({ tenantId: "acme" });
```
