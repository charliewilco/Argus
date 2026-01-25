import type { EventEnvelope } from "./event";

export interface EventStore {
	put(event: EventEnvelope): Promise<void>;
	get(id: string): Promise<EventEnvelope | null>;
	hasDedupe(
		provider: string,
		tenantId: string,
		connectionId: string,
		dedupeKey: string,
	): Promise<boolean>;

	markDelivery(
		id: string,
		attempt: number,
		status: "delivered" | "failed",
		error?: string,
	): Promise<void>;

	putDLQ(id: string, reason: string): Promise<void>;
	listDLQ(filters?: {
		tenantId?: string;
		connectionId?: string;
	}): Promise<Array<{ eventId: string; reason: string }>>;

	list(filters?: {
		since?: string;
		until?: string;
		tenantId?: string;
		connectionId?: string;
		normalized?: Record<string, unknown>;
	}): Promise<EventEnvelope[]>;
}
