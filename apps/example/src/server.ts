import { Runtime } from "@argus/runtime/runtime"
import { MemoryEventStore } from "@argus/storage-memory/index"
import { MemoryQueue } from "@argus/queue-memory/index"
import { GitHubProvider } from "@argus/provider-github/index"
import { verifyGitHubSignature } from "@argus/provider-github/verifyGitHubSignature"

const eventStore = new MemoryEventStore()
const queue = new MemoryQueue()
const runtime = new Runtime({ eventStore, queue })

const provider = new GitHubProvider()
runtime.registerProvider(provider)

runtime.registerConnection({
  tenantId: "tenant_1",
  connectionId: "github_default",
  provider: "github",
  auth: {
    token: process.env.GITHUB_TOKEN,
    webhookSecret: process.env.GITHUB_WEBHOOK_SECRET,
  },
  config: {},
})

runtime.onEvent(async (event) => {
  console.log("EVENT", JSON.stringify(event, null, 2))
  if (process.env.ARGUS_FAIL_HANDLER === "1") {
    throw new Error("forced handler failure")
  }
})

const server = Bun.serve({
  port: Number(process.env.PORT ?? 3000),
  fetch: async (req) => {
    const url = new URL(req.url)

    if (req.method === "POST" && url.pathname === "/webhooks/github/issue.created") {
      const rawBody = await req.text()
      const signature = req.headers.get("x-hub-signature-256")
      const secret = process.env.GITHUB_WEBHOOK_SECRET

      if (secret && !verifyGitHubSignature(secret, rawBody, signature)) {
        return new Response("invalid signature", { status: 401 })
      }

      const body = rawBody ? JSON.parse(rawBody) : {}
      const headers: Record<string, string> = {}
      for (const [key, value] of req.headers.entries()) {
        headers[key.toLowerCase()] = value
      }

      const result = await runtime.handleWebhook({
        provider: "github",
        triggerKey: "issue.created",
        body,
        headers,
        tenantId: "tenant_1",
        connectionId: "github_default",
      })

      return new Response(JSON.stringify(result), {
        status: result.accepted ? 202 : 400,
        headers: { "content-type": "application/json" },
      })
    }

    return new Response("not found", { status: 404 })
  },
})

console.log(`Argus example listening on http://localhost:${server.port}`)
