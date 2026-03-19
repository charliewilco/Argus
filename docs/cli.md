# CLI

The CLI reads from the SQLite event store and can optionally re-deliver events to a handler module.

## Commands

Replay events:
```
argus replay --since <iso> --until <iso> [--tenant <id>] [--connection <id>] [--normalized <json>] --handler <path> --sqlite <path>
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

## Examples

Replay all events from January 2024:
```bash
argus replay \
  --since 2024-01-01T00:00:00Z \
  --until 2024-02-01T00:00:00Z \
  --handler ./handler.ts \
  --sqlite ./argus.sqlite
```

Replay events scoped to one tenant and connection:
```bash
argus replay \
  --since 2024-01-01 \
  --until 2024-02-01 \
  --tenant acme \
  --connection gh-main \
  --handler ./handler.ts \
  --sqlite ./argus.sqlite
```

Filter by normalized fields (exact key match):
```bash
argus replay \
  --since 2024-01-01 \
  --until 2024-02-01 \
  --normalized '{"repoFullName":"acme/backend"}' \
  --handler ./handler.ts \
  --sqlite ./argus.sqlite
```

List all DLQ entries:
```bash
argus dlq list --sqlite ./argus.sqlite
```

Replay a specific dead-lettered event:
```bash
argus dlq replay \
  --event evt_abc123 \
  --handler ./handler.ts \
  --sqlite ./argus.sqlite
```

Increase the delivery timeout (default is 30 s):
```bash
argus replay --since 2024-01-01 --handler ./handler.ts --sqlite ./argus.sqlite --wait-ms 60000
```

## Handler modules

The CLI loads a handler module (required for replay) that exports either:
- `default` function, or
- `handleEvent(event)`

The handler receives the stored `EventEnvelope`.

The CLI waits for delivery to finish and will timeout after 30s by default. Use
`--wait-ms` to adjust.

### Writing a handler module

```typescript
// handler.ts
import type { EventEnvelope } from "@argus/core/event";

export default async function handleEvent(event: EventEnvelope): Promise<void> {
  console.log("Replaying event:", event.type, event.id);

  // Route by event type
  if (event.type === "github.issue.created") {
    const normalized = event.data.normalized as {
      repoFullName: string;
      issueNumber: number;
      title: string;
    };
    await notifySlack(`New issue in ${normalized.repoFullName}: ${normalized.title}`);
  }
}

async function notifySlack(message: string) {
  // ... your notification logic
}
```

### Generating a handler scaffold

The `scaffold handler` command writes a typed stub:

```bash
argus scaffold handler ./my-handler.ts
```

This creates `./my-handler.ts` with the correct export signature, which you can then fill in.
