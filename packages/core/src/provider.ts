import type { Connection } from "./connection"
import type { TriggerDefinition } from "./trigger"

export interface Provider {
  readonly name: string
  readonly version: string

  getTriggers(): TriggerDefinition[]
  validateConnection(connection: unknown): asserts connection is Connection

  testConnection?(
    connection: Connection,
  ): Promise<{ ok: true } | { ok: false; error: string }>
}
