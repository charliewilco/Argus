# Runtime

`@argus/runtime` orchestrates triggers, delivery, retries/backoff, and replay.

## Lifecycle
- `registerProvider(provider)`
- `registerConnection(connection)`
- `handleWebhook(...)` for webhook triggers
- `startPolling()` / `stopPolling()` for poll triggers

When a trigger is first used for a connection, the runtime calls `trigger.setup()` and stores any returned state. On `unregisterConnection(...)`, the runtime calls `trigger.teardown()` and clears state.

## Delivery
- Dedupe happens before storage using `(provider, connectionId, dedupeKey)`.
- Events are stored, then delivery jobs are queued.
- Delivery retries with exponential backoff until `maxAttempts`.
- Failed events are written to the DLQ.

## Replay
Runtime exposes `replay(...)` and `replayDLQ(...)` helpers for re-queueing stored events.
