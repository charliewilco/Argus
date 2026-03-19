# Example App

`apps/example` shows how to wire the runtime into a Bun server.

Highlights:
- Uses `Bun.serve()` to expose a webhook endpoint.
- Verifies GitHub signatures with `verifyGitHubSignature`.
- Calls `runtime.handleWebhook(...)` and logs events.
- Optional forced handler failure via `ARGUS_FAIL_HANDLER=1`.

## Running the example

```bash
GITHUB_TOKEN=ghp_... \
GITHUB_WEBHOOK_SECRET=s3cr3t \
bun run apps/example/src/server.ts
```

The server listens on port 3000 by default (override with `PORT`).

## Walkthrough

### `server.ts` — runtime setup

```typescript
import { GitHubProvider } from "@argus/provider-github";
import { MemoryQueue } from "@argus/queue-memory";
import { Runtime } from "@argus/runtime/runtime";
import { MemoryEventStore } from "@argus/storage-memory";
import { createGitHubWebhookHandler } from "./handler";

// Create in-memory storage and queue (swap for SQLite in production)
const eventStore = new MemoryEventStore();
const queue = new MemoryQueue();
const runtime = new Runtime({ eventStore, queue });

// Register the GitHub provider
const provider = new GitHubProvider();
runtime.registerProvider(provider);

// Bind a tenant to a GitHub connection
runtime.registerConnection({
  tenantId: "tenant_1",
  connectionId: "github_default",
  provider: "github",
  auth: {
    token: process.env.GITHUB_TOKEN,
    webhookSecret: process.env.GITHUB_WEBHOOK_SECRET,
  },
  config: {},
});

// Handle delivered events
runtime.onEvent(async (event) => {
  console.log("EVENT", JSON.stringify(event, null, 2));
  // Set ARGUS_FAIL_HANDLER=1 to test retry/DLQ behavior
  if (process.env.ARGUS_FAIL_HANDLER === "1") {
    throw new Error("forced handler failure");
  }
});

// Start the HTTP server
const server = Bun.serve({
  port: Number(process.env.PORT ?? 3000),
  fetch: createGitHubWebhookHandler({
    runtime,
    webhookSecret: process.env.GITHUB_WEBHOOK_SECRET,
  }),
});

console.log(`Argus example listening on http://localhost:${server.port}`);
```

### `handler.ts` — webhook route

```typescript
import { verifyGitHubSignature } from "@argus/provider-github/verifyGitHubSignature";
import type { Runtime } from "@argus/runtime/runtime";

export function createGitHubWebhookHandler(opts: {
  runtime: Pick<Runtime, "handleWebhook">;
  webhookSecret?: string;
}) {
  return async function handleRequest(req: Request): Promise<Response> {
    const url = new URL(req.url);

    if (req.method === "POST" && url.pathname === "/webhooks/github/issue.created") {
      const rawBody = await req.text();

      // Verify HMAC signature before processing
      if (opts.webhookSecret) {
        const signature = req.headers.get("x-hub-signature-256");
        if (!verifyGitHubSignature(opts.webhookSecret, rawBody, signature)) {
          return new Response("invalid signature", { status: 401 });
        }
      }

      const body = rawBody ? JSON.parse(rawBody) : {};
      const headers: Record<string, string> = {};
      for (const [key, value] of req.headers.entries()) {
        headers[key.toLowerCase()] = value;
      }

      const result = await opts.runtime.handleWebhook({
        provider: "github",
        triggerKey: "issue.created",
        body,
        headers,
        tenantId: "tenant_1",
        connectionId: "github_default",
      });

      return new Response(JSON.stringify(result), {
        status: result.accepted ? 202 : 400,
        headers: { "content-type": "application/json" },
      });
    }

    return new Response("not found", { status: 404 });
  };
}
```

## Testing the webhook locally

Use a tool like [ngrok](https://ngrok.com) or [smee.io](https://smee.io) to forward GitHub webhook events to `localhost:3000`.

Or send a test event with `curl`:

```bash
# Compute HMAC signature
SECRET=s3cr3t
BODY='{"action":"opened","issue":{"id":1,"number":42,"title":"Bug","created_at":"2024-01-15T10:00:00Z","updated_at":"2024-01-15T10:00:00Z","user":{"login":"alice"}},"repository":{"full_name":"acme/backend"}}'
SIG="sha256=$(echo -n "$BODY" | openssl dgst -sha256 -hmac "$SECRET" | awk '{print $2}')"

curl -X POST http://localhost:3000/webhooks/github/issue.created \
  -H "Content-Type: application/json" \
  -H "x-hub-signature-256: $SIG" \
  -H "x-github-delivery: test-$(date +%s)" \
  -d "$BODY"
```

Expected response: `{"accepted":true}`

## Testing retry and DLQ behavior

```bash
# Force the handler to throw on every delivery attempt
ARGUS_FAIL_HANDLER=1 bun run apps/example/src/server.ts
```

After `maxAttempts` (default 5) failures, the event is written to the DLQ. You can then replay it via the CLI once the handler is fixed.
