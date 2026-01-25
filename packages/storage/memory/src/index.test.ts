import { expect, test } from "bun:test";
import type { EventEnvelope } from "@argus/core/event";
import { MemoryEventStore } from "./";

function sampleEvent(id: string, dedupeKey: string): EventEnvelope {
	return {
		id,
		type: "test.event",
		occurredAt: new Date().toISOString(),
		receivedAt: new Date().toISOString(),
		provider: "test",
		triggerKey: "t",
		triggerVersion: "1",
		tenantId: "tenant",
		connectionId: "conn",
		dedupeKey,
		data: { raw: {} },
		meta: {},
	};
}

test("MemoryEventStore stores and dedupes", async () => {
	const store = new MemoryEventStore();
	const event = sampleEvent("e1", "dedupe");

	await store.put(event);

	expect(await store.get("e1")).not.toBeNull();
	expect(await store.hasDedupe("test", "conn", "dedupe")).toBe(true);
});

test("MemoryEventStore filters by normalized fields", async () => {
	const store = new MemoryEventStore();
	await store.put({
		...sampleEvent("e1", "a"),
		data: { raw: {}, normalized: { repo: "a", count: 1 } },
	});
	await store.put({
		...sampleEvent("e2", "b"),
		data: { raw: {}, normalized: { repo: "b", count: 2 } },
	});

	const filtered = await store.list({ normalized: { repo: "a" } });
	expect(filtered.length).toBe(1);
	expect(filtered[0]?.id).toBe("e1");
});

test("MemoryEventStore filters by tenant, connection, and time", async () => {
	const store = new MemoryEventStore();
	await store.put({
		...sampleEvent("e1", "a"),
		tenantId: "tenant_a",
		connectionId: "conn_a",
		occurredAt: "2024-01-01T00:00:00.000Z",
	});
	await store.put({
		...sampleEvent("e2", "b"),
		tenantId: "tenant_b",
		connectionId: "conn_b",
		occurredAt: "2024-02-01T00:00:00.000Z",
	});

	const byTenant = await store.list({ tenantId: "tenant_a" });
	expect(byTenant.map((event) => event.id)).toEqual(["e1"]);

	const byConnection = await store.list({ connectionId: "conn_b" });
	expect(byConnection.map((event) => event.id)).toEqual(["e2"]);

	const byTime = await store.list({
		since: "2024-01-15T00:00:00.000Z",
	});
	expect(byTime.map((event) => event.id)).toEqual(["e2"]);
});

test("MemoryEventStore lists DLQ entries with filters", async () => {
	const store = new MemoryEventStore();
	await store.put(sampleEvent("e1", "a"));
	await store.put({
		...sampleEvent("e2", "b"),
		tenantId: "tenant_b",
		connectionId: "conn_b",
	});
	await store.putDLQ("e1", "failed");
	await store.putDLQ("e2", "failed");

	const byTenant = await store.listDLQ({ tenantId: "tenant" });
	expect(byTenant.length).toBe(1);
	expect(byTenant[0]?.eventId).toBe("e1");

	const byConnection = await store.listDLQ({ connectionId: "conn_b" });
	expect(byConnection.length).toBe(1);
	expect(byConnection[0]?.eventId).toBe("e2");
});
