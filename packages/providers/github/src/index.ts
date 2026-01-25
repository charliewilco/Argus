import { AbstractProvider } from "@argus/core/abstractProvider"
import type { Connection } from "@argus/core/connection"
import type { EventEnvelope } from "@argus/core/event"
import type { TriggerDefinition } from "@argus/core/trigger"
import type { TransformInput } from "@argus/core/runtimeTypes"

export type GitHubConnectionAuth = {
  token?: string
  webhookSecret?: string
}

export class GitHubProvider extends AbstractProvider {
  name = "github"
  version = "0.1.0"

  constructor() {
    super()
    this.registerTrigger(issueCreatedTrigger())
  }

  validateConnection(connection: unknown): asserts connection is Connection {
    if (!connection || typeof connection !== "object") {
      throw new Error("Invalid connection")
    }
    const c = connection as Connection
    if (!c.tenantId || !c.connectionId || !c.provider) {
      throw new Error("Connection missing required fields")
    }
    if (c.provider !== this.name) {
      throw new Error("Connection provider mismatch")
    }
  }
}

function issueCreatedTrigger(): TriggerDefinition {
  return {
    provider: "github",
    key: "issue.created",
    version: "1",
    mode: "webhook",
    async setup() {
      return {}
    },
    async teardown() {},
    async transform(input: TransformInput): Promise<EventEnvelope[]> {
      const payload = input.payload as GitHubIssueWebhook
      if (!payload || payload.action !== "opened") return []

      const issue = payload.issue
      const repo = payload.repository
      if (!issue || !repo) return []

      const normalized = {
        repoFullName: repo.full_name,
        issueNumber: issue.number,
        title: issue.title,
        userLogin: issue.user?.login,
        url: issue.html_url,
      }

      const event: EventEnvelope = {
        id: "",
        type: "github.issue.created",
        occurredAt: issue.created_at ?? new Date().toISOString(),
        receivedAt: input.receivedAt,
        provider: input.provider,
        triggerKey: input.triggerKey,
        triggerVersion: input.triggerVersion,
        tenantId: input.connection.tenantId,
        connectionId: input.connection.connectionId,
        dedupeKey: "",
        data: {
          normalized,
          raw: payload,
        },
        meta: input.meta ?? {},
      }

      return [event]
    },
    dedupe(event: EventEnvelope): string {
      const headers = (event.meta?.headers ?? {}) as Record<string, string>
      const delivery = headers["x-github-delivery"]
      if (delivery) return delivery

      const raw = event.data?.raw as GitHubIssueWebhook | undefined
      const issueId = raw?.issue?.id
      const updatedAt = raw?.issue?.updated_at
      if (issueId && updatedAt) return `${issueId}:${updatedAt}`

      return `${event.connectionId}:${event.occurredAt}`
    },
  }
}

type GitHubIssueWebhook = {
  action?: string
  issue?: {
    id: number
    number: number
    title: string
    created_at?: string
    updated_at?: string
    html_url?: string
    user?: { login?: string }
  }
  repository?: {
    full_name?: string
  }
}
