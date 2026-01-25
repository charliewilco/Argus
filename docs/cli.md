# CLI

The CLI reads from the SQLite event store and can optionally re-deliver events to a handler module.

## Commands

Replay events:
```
argus replay --since <iso> --until <iso> [--tenant <id>] [--connection <id>] --handler <path> --sqlite <path>
```

List DLQ entries:
```
argus dlq list [--tenant <id>] [--connection <id>] --sqlite <path>
```

Replay a specific DLQ event:
```
argus dlq replay --event <id> --handler <path> --sqlite <path>
```

Scaffold a handler module:
```
argus scaffold handler ./handler.ts
```

## Handler modules
The CLI loads a handler module (required for replay) that exports either:
- `default` function, or
- `handleEvent(event)`

The handler receives the stored `EventEnvelope`.

The CLI waits for delivery to finish and will timeout after 30s by default. Use
`--wait-ms` to adjust.
