import { expect, test } from "bun:test";
import { Runtime } from "./runtime";
import { MemoryEventStore } from "@argus/storage-memory";
import { MemoryQueue } from "@argus/queue-memory";
import type { TriggerDefinition } from "@argus/core/trigger";
import { AbstractProvider } from "@argus/core/abstractProvider";
import type { Connection } from "@argus/core/connection";

class TestProvider extends AbstractProvider {
	name = "test";
	version = "0.1.0";

	constructor() {
		super();
		this.registerTrigger(testTrigger());
	}

	validateConnection(connection: unknown): asserts connection is Connection {
		if (!connection || typeof connection !== "object")
			throw new Error("invalid");
	}
}

function testTrigger(): TriggerDefinition {
	return {
		provider: "test",
		key: "event",
		version: "1",
		mode: "webhook",
		async setup() {
			return {};
		},
		async teardown() {},
		async transform(input) {
			return [
				{
					id: "",
					type: "test.event",
					occurredAt: new Date().toISOString(),
					receivedAt: input.receivedAt,
					provider: input.provider,
					triggerKey: input.triggerKey,
					triggerVersion: input.triggerVersion,
					tenantId: input.connection.tenantId,
					connectionId: input.connection.connectionId,
					dedupeKey: "",
					data: { raw: input.payload },
					meta: {},
				},
			];
		},
		dedupe() {
			return "fixed";
		},
	};
}

test("Runtime dedupes events", async () => {
	const eventStore = new MemoryEventStore();
	const runtime = new Runtime({
		eventStore,
		queue: new MemoryQueue(),
	});

	const provider = new TestProvider();
	runtime.registerProvider(provider);
	runtime.registerConnection({
		tenantId: "tenant",
		connectionId: "conn",
		provider: "test",
		auth: {},
	});

	await runtime.handleWebhook({
		provider: "test",
		triggerKey: "event",
		body: { hello: "world" },
		headers: {},
		tenantId: "tenant",
		connectionId: "conn",
	});

	await runtime.handleWebhook({
		provider: "test",
		triggerKey: "event",
		body: { hello: "world" },
		headers: {},
		tenantId: "tenant",
		connectionId: "conn",
	});

	const events = await eventStore.list();
	expect(events.length).toBe(1);
});

test("Runtime delivers queued events", async () => {
	const runtime = new Runtime({
		eventStore: new MemoryEventStore(),
		queue: new MemoryQueue(),
	});

	const provider = new TestProvider();
	runtime.registerProvider(provider);
	runtime.registerConnection({
		tenantId: "tenant",
		connectionId: "conn",
		provider: "test",
		auth: {},
	});

	let delivered = 0;
	runtime.onEvent(() => {
		delivered += 1;
	});

	await runtime.handleWebhook({
		provider: "test",
		triggerKey: "event",
		body: { hello: "world" },
		headers: {},
		tenantId: "tenant",
		connectionId: "conn",
	});

	await new Promise((resolve) => setTimeout(resolve, 400));
	expect(delivered).toBe(1);
});
