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

class PollingProvider extends AbstractProvider {
	name = "poller";
	version = "0.1.0";

	constructor() {
		super();
		this.registerTrigger(pollingTrigger());
	}

	validateConnection(connection: unknown): asserts connection is Connection {
		if (!connection || typeof connection !== "object")
			throw new Error("invalid");
	}
}

function pollingTrigger(): TriggerDefinition<unknown, { done?: boolean }> {
	return {
		provider: "poller",
		key: "poll.event",
		version: "1",
		mode: "poll",
		async setup() {
			return {};
		},
		async teardown() {},
		async poll(ctx) {
			if (ctx.state?.done) return { state: ctx.state };
			return { state: { done: true }, payloads: [{ hello: "poll" }] };
		},
		async transform(input) {
			return [
				{
					id: "",
					type: "poll.event",
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
		dedupe(event) {
			const payload = event.data?.raw as { hello?: string } | undefined;
			return payload?.hello ?? "poll";
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

test("Runtime polls triggers and delivers events", async () => {
	const runtime = new Runtime({
		eventStore: new MemoryEventStore(),
		queue: new MemoryQueue(),
		pollIntervalMs: 10,
	});

	const provider = new PollingProvider();
	runtime.registerProvider(provider);
	runtime.registerConnection({
		tenantId: "tenant",
		connectionId: "conn",
		provider: "poller",
		auth: {},
	});

	let delivered = 0;
	runtime.onEvent(() => {
		delivered += 1;
	});

	runtime.startPolling();

	await new Promise((resolve) => setTimeout(resolve, 600));
	runtime.stopPolling();

	expect(delivered).toBe(1);
});

test("Runtime moves failed events to DLQ after max attempts", async () => {
	const eventStore = new MemoryEventStore();
	const runtime = new Runtime({
		eventStore,
		queue: new MemoryQueue(),
		maxAttempts: 1,
	});

	const provider = new TestProvider();
	runtime.registerProvider(provider);
	runtime.registerConnection({
		tenantId: "tenant",
		connectionId: "conn",
		provider: "test",
		auth: {},
	});

	runtime.onEvent(() => {
		throw new Error("boom");
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
	const dlq = await eventStore.listDLQ();
	expect(dlq.length).toBe(1);
});

test("Runtime replays DLQ events", async () => {
	const eventStore = new MemoryEventStore();
	const runtime = new Runtime({
		eventStore,
		queue: new MemoryQueue(),
	});

	runtime.onEvent(() => {});

	await eventStore.put({
		id: "event_1",
		type: "test.event",
		occurredAt: new Date().toISOString(),
		receivedAt: new Date().toISOString(),
		provider: "test",
		triggerKey: "event",
		triggerVersion: "1",
		tenantId: "tenant",
		connectionId: "conn",
		dedupeKey: "dedupe",
		data: { raw: { hello: "world" } },
		meta: {},
	});
	await eventStore.putDLQ("event_1", "failed");

	let delivered = 0;
	runtime.onEvent(() => {
		delivered += 1;
	});

	await runtime.replayDLQ();
	await new Promise((resolve) => setTimeout(resolve, 400));

	expect(delivered).toBe(1);
});

test("Runtime calls setup once and teardown on unregister", async () => {
	let setupCalls = 0;
	let teardownCalls = 0;

	class SetupProvider extends AbstractProvider {
		name = "setup";
		version = "0.1.0";

		constructor() {
			super();
			this.registerTrigger(setupTrigger());
		}

		validateConnection(connection: unknown): asserts connection is Connection {
			if (!connection || typeof connection !== "object")
				throw new Error("invalid");
		}
	}

	function setupTrigger(): TriggerDefinition {
		return {
			provider: "setup",
			key: "event",
			version: "1",
			mode: "webhook",
			async setup() {
				setupCalls += 1;
				return {};
			},
			async teardown() {
				teardownCalls += 1;
			},
			async transform(input) {
				return [
					{
						id: "",
						type: "setup.event",
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
				return "setup";
			},
		};
	}

	const runtime = new Runtime({
		eventStore: new MemoryEventStore(),
		queue: new MemoryQueue(),
	});

	runtime.registerProvider(new SetupProvider());
	runtime.registerConnection({
		tenantId: "tenant",
		connectionId: "conn",
		provider: "setup",
		auth: {},
	});

	await runtime.handleWebhook({
		provider: "setup",
		triggerKey: "event",
		body: { hello: "world" },
		headers: {},
		tenantId: "tenant",
		connectionId: "conn",
	});

	await runtime.handleWebhook({
		provider: "setup",
		triggerKey: "event",
		body: { hello: "world" },
		headers: {},
		tenantId: "tenant",
		connectionId: "conn",
	});

	expect(setupCalls).toBe(1);

	const removed = await runtime.unregisterConnection("tenant", "conn");
	expect(removed).toBe(true);
	expect(teardownCalls).toBe(1);
});
