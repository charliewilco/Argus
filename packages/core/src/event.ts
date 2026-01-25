export type EventEnvelope<TNormalized = unknown, TRaw = unknown> = {
	id: string;
	type: string;
	occurredAt: string;
	receivedAt: string;
	provider: string;
	triggerKey: string;
	triggerVersion: string;
	tenantId: string;
	connectionId: string;
	dedupeKey: string;
	data: { normalized?: TNormalized; raw: TRaw };
	meta: Record<string, unknown>;
};
