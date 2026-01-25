import { Database } from "bun:sqlite";
import type { EventEnvelope } from "@argus/core/event";
import type { EventStore } from "@argus/core/eventStore";

export class SqliteEventStore implements EventStore {
	private db: Database;

	constructor(opts?: { filename?: string }) {
		const filename = opts?.filename ?? ":memory:";
		this.db = new Database(filename);
		this.db.exec(`
			CREATE TABLE IF NOT EXISTS events (
				id TEXT PRIMARY KEY,
				type TEXT,
				occurredAt TEXT,
				receivedAt TEXT,
				provider TEXT,
				triggerKey TEXT,
				triggerVersion TEXT,
				tenantId TEXT,
				connectionId TEXT,
				dedupeKey TEXT,
				data TEXT,
				meta TEXT
			);
			CREATE INDEX IF NOT EXISTS events_dedupe
				ON events(tenantId, provider, connectionId, dedupeKey);
			CREATE INDEX IF NOT EXISTS events_dedupe_legacy
				ON events(provider, connectionId, dedupeKey);
			CREATE INDEX IF NOT EXISTS events_tenant
				ON events(tenantId, connectionId);
			CREATE TABLE IF NOT EXISTS deliveries (
				eventId TEXT,
				attempt INTEGER,
				status TEXT,
				error TEXT,
				createdAt TEXT
			);
			CREATE TABLE IF NOT EXISTS dlq (
				eventId TEXT PRIMARY KEY,
				reason TEXT
			);
		`);
	}

	async put(event: EventEnvelope): Promise<void> {
		this.db
			.query(
				`INSERT OR REPLACE INTO events (
					id, type, occurredAt, receivedAt, provider, triggerKey, triggerVersion,
					tenantId, connectionId, dedupeKey, data, meta
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			)
			.run(
				event.id,
				event.type,
				event.occurredAt,
				event.receivedAt,
				event.provider,
				event.triggerKey,
				event.triggerVersion,
				event.tenantId,
				event.connectionId,
				event.dedupeKey,
				JSON.stringify(event.data ?? {}),
				JSON.stringify(event.meta ?? {}),
			);
	}

	async get(id: string): Promise<EventEnvelope | null> {
		const row = this.db
			.query(`SELECT * FROM events WHERE id = ?`)
			.get(id) as Record<string, unknown> | null;
		return row ? this.rowToEvent(row) : null;
	}

	async hasDedupe(
		provider: string,
		tenantId: string,
		connectionId: string,
		dedupeKey: string,
	): Promise<boolean> {
		const row = this.db
			.query(
				`SELECT 1 FROM events WHERE provider = ? AND tenantId = ? AND connectionId = ? AND dedupeKey = ? LIMIT 1`,
			)
			.get(provider, tenantId, connectionId, dedupeKey);
		return Boolean(row);
	}

	async markDelivery(
		id: string,
		attempt: number,
		status: "delivered" | "failed",
		error?: string,
	): Promise<void> {
		this.db
			.query(
				`INSERT INTO deliveries (eventId, attempt, status, error, createdAt)
				 VALUES (?, ?, ?, ?, ?)`,
			)
			.run(id, attempt, status, error ?? null, new Date().toISOString());
	}

	async putDLQ(id: string, reason: string): Promise<void> {
		this.db
			.query(`INSERT OR REPLACE INTO dlq (eventId, reason) VALUES (?, ?)`)
			.run(id, reason);
	}

	async listDLQ(filters?: {
		tenantId?: string;
		connectionId?: string;
	}): Promise<Array<{ eventId: string; reason: string }>> {
		const rows = this.db
			.query(
				`SELECT d.eventId as eventId, d.reason as reason
				 FROM dlq d
				 LEFT JOIN events e ON e.id = d.eventId
				 WHERE (? IS NULL OR e.tenantId = ?)
				   AND (? IS NULL OR e.connectionId = ?)`,
			)
			.all(
				filters?.tenantId ?? null,
				filters?.tenantId ?? null,
				filters?.connectionId ?? null,
				filters?.connectionId ?? null,
			) as Array<{ eventId: string; reason: string }>;

		return rows ?? [];
	}

	async list(filters?: {
		since?: string;
		until?: string;
		tenantId?: string;
		connectionId?: string;
		normalized?: Record<string, unknown>;
	}): Promise<EventEnvelope[]> {
		const rows = this.db
			.query(
				`SELECT * FROM events
				 WHERE (? IS NULL OR occurredAt >= ?)
				   AND (? IS NULL OR occurredAt <= ?)
				   AND (? IS NULL OR tenantId = ?)
				   AND (? IS NULL OR connectionId = ?)`,
			)
			.all(
				filters?.since ?? null,
				filters?.since ?? null,
				filters?.until ?? null,
				filters?.until ?? null,
				filters?.tenantId ?? null,
				filters?.tenantId ?? null,
				filters?.connectionId ?? null,
				filters?.connectionId ?? null,
			) as Record<string, unknown>[];

		const events = rows.map((row) => this.rowToEvent(row));
		return events.filter((event) =>
			this.matchesNormalized(event, filters?.normalized),
		);
	}

	private rowToEvent(row: Record<string, unknown>): EventEnvelope {
		return {
			id: String(row.id),
			type: String(row.type ?? ""),
			occurredAt: String(row.occurredAt ?? ""),
			receivedAt: String(row.receivedAt ?? ""),
			provider: String(row.provider ?? ""),
			triggerKey: String(row.triggerKey ?? ""),
			triggerVersion: String(row.triggerVersion ?? ""),
			tenantId: String(row.tenantId ?? ""),
			connectionId: String(row.connectionId ?? ""),
			dedupeKey: String(row.dedupeKey ?? ""),
			data: JSON.parse(String(row.data ?? "{}")),
			meta: JSON.parse(String(row.meta ?? "{}")),
		};
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
