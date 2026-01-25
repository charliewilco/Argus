import { expect, test } from "bun:test";
import { MemoryEventStore } from "./";
import type { EventEnvelope } from "@argus/core/event";

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
