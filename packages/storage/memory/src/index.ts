import type { EventEnvelope } from "@argus/core/event";
import type { EventStore } from "@argus/core/eventStore";

export class MemoryEventStore implements EventStore {
	private events = new Map<string, EventEnvelope>();
	private dedupe = new Set<string>();
	private deliveries = new Map<
		string,
		Array<{ attempt: number; status: string; error?: string }>
	>();
	private dlq = new Map<string, string>();

	async put(event: EventEnvelope): Promise<void> {
		this.events.set(event.id, event);
		const key = this.dedupeKey(
			event.provider,
			event.connectionId,
			event.dedupeKey,
		);
		this.dedupe.add(key);
	}

	async get(id: string): Promise<EventEnvelope | null> {
		return this.events.get(id) ?? null;
	}

	async hasDedupe(
		provider: string,
		connectionId: string,
		dedupeKey: string,
	): Promise<boolean> {
		return this.dedupe.has(this.dedupeKey(provider, connectionId, dedupeKey));
	}

	async markDelivery(
		id: string,
		attempt: number,
		status: "delivered" | "failed",
		error?: string,
	): Promise<void> {
		const list = this.deliveries.get(id) ?? [];
		list.push({ attempt, status, error });
		this.deliveries.set(id, list);
	}

	async putDLQ(id: string, reason: string): Promise<void> {
		this.dlq.set(id, reason);
	}

	async listDLQ(filters?: {
		tenantId?: string;
		connectionId?: string;
	}): Promise<Array<{ eventId: string; reason: string }>> {
		const results: Array<{ eventId: string; reason: string }> = [];
		for (const [eventId, reason] of this.dlq.entries()) {
			const event = this.events.get(eventId);
			if (!event) continue;
			if (filters?.tenantId && event.tenantId !== filters.tenantId) continue;
			if (filters?.connectionId && event.connectionId !== filters.connectionId)
				continue;
			results.push({ eventId, reason });
		}
		return results;
	}

	async list(filters?: {
		since?: string;
		until?: string;
		tenantId?: string;
		connectionId?: string;
		normalized?: Record<string, unknown>;
	}): Promise<EventEnvelope[]> {
		const results: EventEnvelope[] = [];
		const sinceMs = filters?.since ? Date.parse(filters.since) : null;
		const untilMs = filters?.until ? Date.parse(filters.until) : null;

		for (const event of this.events.values()) {
			if (filters?.tenantId && event.tenantId !== filters.tenantId) continue;
			if (filters?.connectionId && event.connectionId !== filters.connectionId)
				continue;
			if (!this.matchesNormalized(event, filters?.normalized)) continue;

			const occurredMs = Date.parse(event.occurredAt);
			if (sinceMs !== null && occurredMs < sinceMs) continue;
			if (untilMs !== null && occurredMs > untilMs) continue;

			results.push(event);
		}

		return results;
	}

	private dedupeKey(
		provider: string,
		connectionId: string,
		dedupeKey: string,
	): string {
		return `${provider}:${connectionId}:${dedupeKey}`;
	}

	private matchesNormalized(
		event: EventEnvelope,
		filters?: Record<string, unknown>,
	): boolean {
		if (!filters || Object.keys(filters).length === 0) return true;
		const normalized = event.data?.normalized;
		if (!normalized || typeof normalized !== "object") return false;
		for (const [key, value] of Object.entries(filters)) {
			if ((normalized as Record<string, unknown>)[key] !== value) {
				return false;
			}
		}
		return true;
	}
}
