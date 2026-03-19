# Providers

Providers are workspace packages under `packages/providers/*`.

Each provider:
- Implements the `Provider` interface (or `AbstractProvider` helper)
- Exposes trigger definitions
- Validates connections

See [GitHub Provider](./providers/github.md) for the current example.

## Building a custom provider

Use `AbstractProvider` to reduce boilerplate:

```typescript
// packages/providers/stripe/src/index.ts
import { AbstractProvider } from "@argus/core/abstractProvider";
import type { Connection } from "@argus/core/connection";
import type { TriggerDefinition } from "@argus/core/trigger";
import type { EventEnvelope } from "@argus/core/event";
import type { TransformInput } from "@argus/core/runtimeTypes";

type StripeAuth = { webhookSecret: string };

export class StripeProvider extends AbstractProvider {
  name = "stripe";
  version = "0.1.0";

  constructor() {
    super();
    this.registerTrigger(paymentIntentSucceededTrigger());
  }

  validateConnection(connection: unknown): asserts connection is Connection {
    const c = connection as Connection;
    if (!c.tenantId || !c.connectionId || c.provider !== this.name) {
      throw new Error("Invalid Stripe connection");
    }
    const auth = c.auth as StripeAuth;
    if (!auth?.webhookSecret) {
      throw new Error("Stripe connection missing webhookSecret");
    }
  }
}

function paymentIntentSucceededTrigger(): TriggerDefinition {
  return {
    provider: "stripe",
    key: "payment_intent.succeeded",
    version: "1",
    mode: "webhook",

    async setup() { return {}; },
    async teardown() {},

    async ingest(ctx) {
      // Verify Stripe-Signature header here; throw to reject
    },

    async transform(input: TransformInput): Promise<EventEnvelope[]> {
      const payload = input.payload as { type: string; data: { object: Record<string, unknown> } };
      if (payload.type !== "payment_intent.succeeded") return [];

      const pi = payload.data.object;
      return [{
        id: "",
        type: "stripe.payment_intent.succeeded",
        occurredAt: new Date().toISOString(),
        receivedAt: input.receivedAt,
        provider: input.provider,
        triggerKey: input.triggerKey,
        triggerVersion: input.triggerVersion,
        tenantId: input.connection.tenantId,
        connectionId: input.connection.connectionId,
        dedupeKey: "",
        data: {
          raw: payload,
          normalized: { id: pi.id, amount: pi.amount, currency: pi.currency },
        },
        meta: input.meta ?? {},
      }];
    },

    dedupe(event: EventEnvelope): string {
      return (event.data.raw as { id?: string }).id ?? event.occurredAt;
    },
  };
}
```

Register it like any other provider:

```typescript
import { StripeProvider } from "@argus/provider-stripe";

runtime.registerProvider(new StripeProvider());
runtime.registerConnection({
  tenantId: "acme",
  connectionId: "stripe-live",
  provider: "stripe",
  auth: { webhookSecret: process.env.STRIPE_WEBHOOK_SECRET },
});
```

## Provider interface reference

```typescript
interface Provider {
  readonly name: string;     // unique identifier, e.g. "github"
  readonly version: string;  // semver, e.g. "0.1.0"

  getTriggers(): TriggerDefinition[];
  validateConnection(connection: unknown): asserts connection is Connection;

  // Optional: test auth credentials before registering
  testConnection?(connection: Connection): Promise<{ ok: true } | { ok: false; error: string }>;
}
```
