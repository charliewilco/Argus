import type { EventEnvelope } from "@argus/core/event"
import type { EventStore } from "@argus/core/eventStore"

export class SqliteEventStore implements EventStore {
  constructor() {
    throw new Error("SqliteEventStore not implemented yet")
  }

  async put(_event: EventEnvelope): Promise<void> {
    throw new Error("SqliteEventStore not implemented yet")
  }

  async get(_id: string): Promise<EventEnvelope | null> {
    throw new Error("SqliteEventStore not implemented yet")
  }

  async hasDedupe(_provider: string, _connectionId: string, _dedupeKey: string): Promise<boolean> {
    throw new Error("SqliteEventStore not implemented yet")
  }

  async markDelivery(
    _id: string,
    _attempt: number,
    _status: "delivered" | "failed",
    _error?: string,
  ): Promise<void> {
    throw new Error("SqliteEventStore not implemented yet")
  }

  async putDLQ(_id: string, _reason: string): Promise<void> {
    throw new Error("SqliteEventStore not implemented yet")
  }

  async listDLQ(
    _filters?: { tenantId?: string; connectionId?: string },
  ): Promise<Array<{ eventId: string; reason: string }>> {
    throw new Error("SqliteEventStore not implemented yet")
  }

  async list(_filters?: {
    since?: string
    until?: string
    tenantId?: string
    connectionId?: string
  }): Promise<EventEnvelope[]> {
    throw new Error("SqliteEventStore not implemented yet")
  }
}
