# CLI

The CLI reads from the SQLite event store and can optionally re-deliver events to a handler module.

## Commands

Replay events:
```
argus replay --since <iso> --until <iso> [--tenant <id>] [--connection <id>] [--handler <path>] --sqlite <path>
```

List DLQ entries:
```
argus dlq list [--tenant <id>] [--connection <id>] --sqlite <path>
```

Replay a specific DLQ event:
```
argus dlq replay --event <id> [--handler <path>] --sqlite <path>
```

## Handler modules
If `--handler` is provided, the CLI loads a module that exports either:
- `default` function, or
- `handleEvent(event)`

The handler receives the stored `EventEnvelope`.
