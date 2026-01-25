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
