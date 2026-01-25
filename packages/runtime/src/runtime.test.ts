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

test("Runtime replays events by id", async () => {
	const eventStore = new MemoryEventStore();
	const runtime = new Runtime({
		eventStore,
		queue: new MemoryQueue(),
	});

	await eventStore.put({
		id: "event_2",
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

	let delivered = 0;
	runtime.onEvent(() => {
		delivered += 1;
	});

	const count = await runtime.replayByIds(["event_2"]);
	expect(count).toBe(1);

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

test("Runtime selects latest trigger version by default", async () => {
	class VersionedProvider extends AbstractProvider {
		name = "versioned";
		version = "0.1.0";

		constructor() {
			super();
			this.registerTrigger(versionedTrigger("1"));
			this.registerTrigger(versionedTrigger("2"));
		}

		validateConnection(connection: unknown): asserts connection is Connection {
			if (!connection || typeof connection !== "object")
				throw new Error("invalid");
		}
	}

	function versionedTrigger(version: string): TriggerDefinition {
		return {
			provider: "versioned",
			key: "event",
			version,
			mode: "webhook",
			async setup() {
				return {};
			},
			async teardown() {},
			async transform(input) {
				return [
					{
						id: "",
						type: `versioned.event.v${version}`,
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
				return `v${version}`;
			},
		};
	}

	const eventStore = new MemoryEventStore();
	const runtime = new Runtime({
		eventStore,
		queue: new MemoryQueue(),
	});
	runtime.registerProvider(new VersionedProvider());
	runtime.registerConnection({
		tenantId: "tenant",
		connectionId: "conn",
		provider: "versioned",
		auth: {},
	});

	await runtime.handleWebhook({
		provider: "versioned",
		triggerKey: "event",
		body: { hello: "world" },
		headers: {},
		tenantId: "tenant",
		connectionId: "conn",
	});

	const events = await eventStore.list();
	expect(events.length).toBe(1);
	expect(events[0]?.type).toBe("versioned.event.v2");
});

test("Runtime uses explicit trigger version when provided", async () => {
	class VersionedProvider extends AbstractProvider {
		name = "versioned";
		version = "0.1.0";

		constructor() {
			super();
			this.registerTrigger(versionedTrigger("1"));
			this.registerTrigger(versionedTrigger("2"));
		}

		validateConnection(connection: unknown): asserts connection is Connection {
			if (!connection || typeof connection !== "object")
				throw new Error("invalid");
		}
	}

	function versionedTrigger(version: string): TriggerDefinition {
		return {
			provider: "versioned",
			key: "event",
			version,
			mode: "webhook",
			async setup() {
				return {};
			},
			async teardown() {},
			async transform(input) {
				return [
					{
						id: "",
						type: `versioned.event.v${version}`,
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
				return `v${version}`;
			},
		};
	}

	const eventStore = new MemoryEventStore();
	const runtime = new Runtime({
		eventStore,
		queue: new MemoryQueue(),
	});
	runtime.registerProvider(new VersionedProvider());
	runtime.registerConnection({
		tenantId: "tenant",
		connectionId: "conn",
		provider: "versioned",
		auth: {},
	});

	await runtime.handleWebhook({
		provider: "versioned",
		triggerKey: "event",
		triggerVersion: "1",
		body: { hello: "world" },
		headers: {},
		tenantId: "tenant",
		connectionId: "conn",
	});

	const events = await eventStore.list();
	expect(events.length).toBe(1);
	expect(events[0]?.type).toBe("versioned.event.v1");
});

test("Runtime enforces tenant scope", async () => {
	const runtime = new Runtime({
		eventStore: new MemoryEventStore(),
		queue: new MemoryQueue(),
		tenantScope: "tenant_a",
	});

	runtime.registerProvider(new TestProvider());

	expect(() =>
		runtime.registerConnection({
			tenantId: "tenant_b",
			connectionId: "conn",
			provider: "test",
			auth: {},
		}),
	).toThrow();

	runtime.registerConnection({
		tenantId: "tenant_a",
		connectionId: "conn",
		provider: "test",
		auth: {},
	});

	const result = await runtime.handleWebhook({
		provider: "test",
		triggerKey: "event",
		body: { hello: "world" },
		headers: {},
		tenantId: "tenant_b",
		connectionId: "conn",
	});

	expect(result.accepted).toBe(false);
	expect(result.reason).toBe("tenant out of scope");
});

test("Runtime rejects unknown providers and connections", async () => {
	const runtime = new Runtime({
		eventStore: new MemoryEventStore(),
		queue: new MemoryQueue(),
	});

	const unknownProvider = await runtime.handleWebhook({
		provider: "missing",
		triggerKey: "event",
		body: {},
		headers: {},
		tenantId: "tenant",
		connectionId: "conn",
	});
	expect(unknownProvider.accepted).toBe(false);
	expect(unknownProvider.reason).toBe("unknown provider: missing");

	runtime.registerProvider(new TestProvider());
	const unknownConnection = await runtime.handleWebhook({
		provider: "test",
		triggerKey: "event",
		body: {},
		headers: {},
		tenantId: "tenant",
		connectionId: "missing",
	});
	expect(unknownConnection.accepted).toBe(false);
	expect(unknownConnection.reason).toBe("unknown connection");
});

test("Runtime rejects connection/provider mismatch", async () => {
	const runtime = new Runtime({
		eventStore: new MemoryEventStore(),
		queue: new MemoryQueue(),
	});
	runtime.registerProvider(new TestProvider());
	class OtherProvider extends AbstractProvider {
		name = "other";
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
	runtime.registerProvider(new OtherProvider());
	runtime.registerConnection({
		tenantId: "tenant",
		connectionId: "conn",
		provider: "test",
		auth: {},
	});

	const result = await runtime.handleWebhook({
		provider: "other",
		triggerKey: "event",
		body: {},
		headers: {},
		tenantId: "tenant",
		connectionId: "conn",
	});

	expect(result.accepted).toBe(false);
	expect(result.reason).toBe("connection/provider mismatch");
});

test("Runtime calls validateConfig and ingest hooks", async () => {
	let validateCalls = 0;
	let ingestCalls = 0;

	class HookProvider extends AbstractProvider {
		name = "hooks";
		version = "0.1.0";

		constructor() {
			super();
			this.registerTrigger({
				provider: "hooks",
				key: "event",
				version: "1",
				mode: "webhook",
				validateConfig() {
					validateCalls += 1;
				},
				async setup() {
					return {};
				},
				async teardown() {},
				async ingest(ctx) {
					if (ctx.headers["x-test"] === "1") {
						ingestCalls += 1;
					}
				},
				async transform() {
					return [
						{
							id: "",
							type: "hooks.event",
							occurredAt: new Date().toISOString(),
							receivedAt: new Date().toISOString(),
							provider: "hooks",
							triggerKey: "event",
							triggerVersion: "1",
							tenantId: "tenant",
							connectionId: "conn",
							dedupeKey: "",
							data: { raw: {} },
							meta: {},
						},
					];
				},
				dedupe() {
					return "hooks";
				},
			});
		}

		validateConnection(connection: unknown): asserts connection is Connection {
			if (!connection || typeof connection !== "object")
				throw new Error("invalid");
		}
	}

	const runtime = new Runtime({
		eventStore: new MemoryEventStore(),
		queue: new MemoryQueue(),
	});
	runtime.registerProvider(new HookProvider());
	runtime.registerConnection({
		tenantId: "tenant",
		connectionId: "conn",
		provider: "hooks",
		auth: {},
		config: { enabled: true },
	});

	await runtime.handleWebhook({
		provider: "hooks",
		triggerKey: "event",
		body: {},
		headers: { "x-test": "1" },
		tenantId: "tenant",
		connectionId: "conn",
	});

	expect(validateCalls).toBe(1);
	expect(ingestCalls).toBe(1);
});

test("Runtime rejects replay filters outside tenant scope", async () => {
	const runtime = new Runtime({
		eventStore: new MemoryEventStore(),
		queue: new MemoryQueue(),
		tenantScope: "tenant_a",
	});

	await expect(
		runtime.replay({ tenantId: "tenant_b" }),
	).rejects.toThrow("tenant out of scope");
});
