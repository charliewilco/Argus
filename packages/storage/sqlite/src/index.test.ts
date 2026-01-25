import { expect, test } from "bun:test";
import { SqliteEventStore } from "./";

const baseEvent = {
	id: "event_1",
	type: "test.event",
	occurredAt: "2024-01-01T00:00:00.000Z",
	receivedAt: "2024-01-01T00:00:00.000Z",
	provider: "test",
	triggerKey: "event",
	triggerVersion: "1",
	tenantId: "tenant",
	connectionId: "conn",
	dedupeKey: "dedupe",
	data: { raw: { hello: "world" } },
	meta: {},
};

test("SqliteEventStore stores and dedupes", async () => {
	const store = new SqliteEventStore({ filename: ":memory:" });
	await store.put(baseEvent);

	expect(await store.hasDedupe("test", "conn", "dedupe")).toBe(true);

	const fetched = await store.get("event_1");
	expect(fetched?.id).toBe("event_1");
});

test("SqliteEventStore filters list by tenant and time", async () => {
	const store = new SqliteEventStore({ filename: ":memory:" });
	await store.put(baseEvent);
	await store.put({
		...baseEvent,
		id: "event_2",
		tenantId: "tenant_2",
		occurredAt: "2024-02-01T00:00:00.000Z",
	});

	const byTenant = await store.list({ tenantId: "tenant" });
	expect(byTenant.length).toBe(1);

	const byTime = await store.list({ since: "2024-01-15T00:00:00.000Z" });
	expect(byTime.length).toBe(1);
	expect(byTime[0]?.id).toBe("event_2");
});

test("SqliteEventStore lists DLQ entries", async () => {
	const store = new SqliteEventStore({ filename: ":memory:" });
	await store.put(baseEvent);
	await store.putDLQ("event_1", "failed");

	const entries = await store.listDLQ({ tenantId: "tenant" });
	expect(entries.length).toBe(1);
	expect(entries[0]?.eventId).toBe("event_1");
});
