# GitHub Provider

Package: `@argus/provider-github`

Exports:
- `GitHubProvider`
- `verifyGitHubSignature(secret, body, signatureHeader)`

## Setup

```typescript
import { GitHubProvider } from "@argus/provider-github";
import { Runtime } from "@argus/runtime/runtime";
import { MemoryEventStore } from "@argus/storage-memory";
import { MemoryQueue } from "@argus/queue-memory";

const runtime = new Runtime({
  eventStore: new MemoryEventStore(),
  queue: new MemoryQueue(),
});

runtime.registerProvider(new GitHubProvider());

runtime.registerConnection({
  tenantId: "acme",
  connectionId: "gh-main",
  provider: "github",
  auth: {
    token: process.env.GITHUB_TOKEN,           // for polling
    webhookSecret: process.env.GITHUB_WEBHOOK_SECRET, // for webhooks
  },
  config: {
    repoFullName: "acme/backend",  // scope polling to a specific repo
  },
});
```

## Webhook Trigger
Key: `issue.created`

Behavior:
- Handles `issues` webhooks with `action=opened`.
- Dedupe uses `x-github-delivery` header; fallback `issue.id + updated_at`.
- Normalized fields:
  - `repoFullName`
  - `issueNumber`
  - `title`
  - `userLogin`
  - `url`

### Verifying signatures

Always verify the `x-hub-signature-256` header before passing the body to the runtime:

```typescript
import { verifyGitHubSignature } from "@argus/provider-github/verifyGitHubSignature";

// In your HTTP handler:
const rawBody = await req.text();
const signature = req.headers.get("x-hub-signature-256");
const secret = process.env.GITHUB_WEBHOOK_SECRET!;

if (!verifyGitHubSignature(secret, rawBody, signature)) {
  return new Response("invalid signature", { status: 401 });
}
```

`verifyGitHubSignature` returns a boolean — no async, no exceptions.

### Handling the webhook

```typescript
const body = JSON.parse(rawBody);
const headers: Record<string, string> = {};
for (const [key, value] of req.headers.entries()) {
  headers[key.toLowerCase()] = value;
}

const result = await runtime.handleWebhook({
  provider: "github",
  triggerKey: "issue.created",
  body,
  headers,
  tenantId: "acme",
  connectionId: "gh-main",
});

// result.accepted === false if provider/connection/trigger unknown
// or if the event is a duplicate (same delivery ID seen before)
```

### Event shape

```typescript
runtime.onEvent((event) => {
  // event.type === "github.issue.created"
  const n = event.data.normalized as {
    repoFullName: string;
    issueNumber: number;
    title: string;
    userLogin?: string;
    url?: string;
  };

  console.log(`[${n.repoFullName}] #${n.issueNumber}: ${n.title}`);
});
```

## Polling Trigger
Key: `issues.updated`

Behavior:
- Polls GitHub issues updated since cursor time.
- Default lookback is 24h on first run.
- Pagination uses the `link` header.

### Starting polling

```typescript
runtime.startPolling(); // runs every 30s by default

// Or configure interval at construction time:
const runtime = new Runtime({
  eventStore,
  queue,
  pollIntervalMs: 60_000, // 1 minute
});
runtime.startPolling();
```

The polling trigger requires a GitHub token for private repos; public repos work without one.

### Poll event shape

```typescript
runtime.onEvent((event) => {
  // event.type === "github.issue.updated"
  const n = event.data.normalized as {
    repoFullName: string | undefined;
    issueNumber: number;
    title: string;
    userLogin?: string;
    url?: string;
  };
});
```

### Scoping to a specific repository

Set `config.repoFullName` on the connection to restrict polling to one repo:

```typescript
runtime.registerConnection({
  tenantId: "acme",
  connectionId: "gh-backend",
  provider: "github",
  auth: { token: process.env.GITHUB_TOKEN },
  config: { repoFullName: "acme/backend" },
});
```

Without `repoFullName`, the trigger polls `/issues` (all repos the token has access to).

## Stopping and cleanup

```typescript
runtime.stopPolling();

// Or stop everything:
runtime.shutdown();
```
