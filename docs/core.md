# Core Types

## EventEnvelope
`@argus/core/event`

An event carries both raw payloads and optional normalized data:
- `data.raw` is the original webhook/poll payload
- `data.normalized` is a provider-specific subset

```typescript
import type { EventEnvelope } from "@argus/core/event";

// Typed with normalized and raw shapes
type MyNormalized = { repoFullName: string; issueNumber: number };
type MyRaw = { action: string; issue: { number: number } };

function handleEvent(event: EventEnvelope<MyNormalized, MyRaw>) {
  console.log(event.type);            // "github.issue.created"
  console.log(event.tenantId);        // "acme"
  console.log(event.connectionId);    // "gh-main"
  console.log(event.occurredAt);      // ISO 8601 timestamp
  console.log(event.data.normalized); // { repoFullName, issueNumber }
  console.log(event.data.raw);        // original payload
}
```

Full shape:

| Field           | Type                    | Description                              |
|-----------------|-------------------------|------------------------------------------|
| `id`            | `string`                | Content-addressed SHA-256 identifier     |
| `type`          | `string`                | Provider-specific event type string      |
| `occurredAt`    | `string`                | ISO 8601 — when the event happened       |
| `receivedAt`    | `string`                | ISO 8601 — when Argus received it        |
| `provider`      | `string`                | Provider name (e.g. `"github"`)          |
| `triggerKey`    | `string`                | Trigger key (e.g. `"issue.created"`)     |
| `triggerVersion`| `string`                | Trigger version (e.g. `"1"`)             |
| `tenantId`      | `string`                | Tenant that owns the connection          |
| `connectionId`  | `string`                | Connection that produced the event       |
| `dedupeKey`     | `string`                | Deduplication key set by the trigger     |
| `data.raw`      | `TRaw`                  | Original unmodified payload              |
| `data.normalized` | `TNormalized?`        | Optional provider-specific normalized data |
| `meta`          | `Record<string, unknown>` | Extra context (e.g. webhook headers)   |

## TriggerDefinition
`@argus/core/trigger`

Triggers define:
- `mode`: `webhook` | `poll` | `hybrid`
- `setup` / `teardown` lifecycle hooks
- `ingest` (webhook) and/or `poll` (polling)
- `transform` into `EventEnvelope[]`
- `dedupe` key generation

```typescript
import type { TriggerDefinition } from "@argus/core/trigger";
import type { EventEnvelope } from "@argus/core/event";

type MyConfig = { repoFullName: string };
type MyState  = { cursor?: string };

const myTrigger: TriggerDefinition<MyConfig, MyState> = {
  provider: "myprovider",
  key: "item.created",
  version: "1",
  mode: "webhook",

  // Called once per connection before the trigger is first used
  async setup(ctx) {
    // ctx.connection, ctx.config available
    return {}; // return { state: initialState } to persist state
  },

  // Called when the connection is unregistered
  async teardown(ctx) {
    // ctx.state available for cleanup
  },

  // Webhook mode: called with the raw HTTP context before transform
  async ingest(ctx) {
    // Validate signature, throw to reject
  },

  // Convert raw payload to one or more EventEnvelopes
  async transform(input) {
    const payload = input.payload as { id: string; name: string };
    const event: EventEnvelope = {
      id: "",
      type: "myprovider.item.created",
      occurredAt: new Date().toISOString(),
      receivedAt: input.receivedAt,
      provider: input.provider,
      triggerKey: input.triggerKey,
      triggerVersion: input.triggerVersion,
      tenantId: input.connection.tenantId,
      connectionId: input.connection.connectionId,
      dedupeKey: "",
      data: { raw: payload, normalized: { id: payload.id, name: payload.name } },
      meta: input.meta ?? {},
    };
    return [event];
  },

  // Stable key used for deduplication — same key means same event
  dedupe(event) {
    return (event.data.raw as { id: string }).id;
  },
};
```

## Provider
`@argus/core/provider`

Providers expose:
- `name` and `version`
- `getTriggers()`
- `validateConnection()`

`AbstractProvider` is a helper base class for building providers.

```typescript
import { AbstractProvider } from "@argus/core/abstractProvider";
import type { Connection } from "@argus/core/connection";

class MyProvider extends AbstractProvider {
  name = "myprovider";
  version = "0.1.0";

  constructor() {
    super();
    this.registerTrigger(myTrigger); // defined above
  }

  validateConnection(connection: unknown): asserts connection is Connection {
    const c = connection as Connection;
    if (!c.tenantId || !c.connectionId || c.provider !== this.name) {
      throw new Error("Invalid connection");
    }
  }
}
```

## Connection
`@argus/core/connection`

Connections bind a tenant + provider to auth/config data.

```typescript
import type { Connection } from "@argus/core/connection";

const connection: Connection = {
  tenantId: "acme",          // identifies the tenant
  connectionId: "gh-main",   // unique per tenant+provider
  provider: "github",        // must match provider.name
  auth: {
    token: "ghp_...",
    webhookSecret: "s3cr3t",
  },
  config: {
    repoFullName: "acme/backend",
  },
};
```
