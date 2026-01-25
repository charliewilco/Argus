import type { EventEnvelope } from "@argus/core/event";
import type { Provider } from "@argus/core/provider";
import type { TriggerDefinition } from "@argus/core/trigger";
import type { TransformInput } from "@argus/core/runtimeTypes";
import { createEventId } from "@argus/core/id";
import type { Connection as CoreConnection } from "@argus/core/connection";
import type { EventStore as CoreEventStore } from "@argus/core/eventStore";
import type { DeliveryJob, Queue as CoreQueue } from "@argus/core/queue";

// Re-export for convenience if consumers import from runtime
export type RuntimeConnection = CoreConnection;
export type RuntimeEventStore = CoreEventStore;
export type RuntimeQueue = CoreQueue;

const DEFAULT_MAX_ATTEMPTS = 5;
const DELIVERY_LEASE_LIMIT = 10;
const DELIVERY_TICK_MS = 250;
const DEFAULT_POLL_INTERVAL_MS = 30_000;

export class Runtime {
	private eventStore: CoreEventStore;
	private queue: CoreQueue;
	private maxAttempts: number;
	private pollIntervalMs: number;
	private providers = new Map<string, Provider>();
	private connections = new Map<string, CoreConnection>();
	private triggerStates = new Map<string, unknown>();
	private triggerSetupDone = new Set<string>();
	private handlers: Array<(e: EventEnvelope) => Promise<void> | void> = [];
	private deliveryTimer: number | null = null;
	private pollingTimer: number | null = null;

	constructor(opts: {
		eventStore: CoreEventStore;
		queue: CoreQueue;
		maxAttempts?: number;
		pollIntervalMs?: number;
	}) {
		this.eventStore = opts.eventStore;
		this.queue = opts.queue;
		this.maxAttempts = opts.maxAttempts ?? DEFAULT_MAX_ATTEMPTS;
		this.pollIntervalMs = opts.pollIntervalMs ?? DEFAULT_POLL_INTERVAL_MS;
	}

	registerProvider(provider: Provider): void {
		this.providers.set(provider.name, provider);
	}

	registerConnection(connection: CoreConnection): void {
		const provider = this.providers.get(connection.provider);
		if (!provider) {
			throw new Error(`Provider not registered: ${connection.provider}`);
		}
		provider.validateConnection(connection);
		this.connections.set(
			this.connectionKey(connection.tenantId, connection.connectionId),
			connection,
		);
	}

	async unregisterConnection(
		tenantId: string,
		connectionId: string,
	): Promise<boolean> {
		const key = this.connectionKey(tenantId, connectionId);
		const connection = this.connections.get(key);
		if (!connection) return false;

		const provider = this.providers.get(connection.provider);
		if (provider) {
			for (const trigger of provider.getTriggers()) {
				await this.teardownTrigger(connection, trigger);
			}
		}

		this.connections.delete(key);
		return true;
	}

	onEvent(handler: (e: EventEnvelope) => Promise<void> | void): void {
		this.handlers.push(handler);
	}

	async handleWebhook(input: {
		provider: string;
		triggerKey: string;
		body: unknown;
		headers: Record<string, string>;
		tenantId: string;
		connectionId: string;
	}): Promise<{ accepted: boolean; reason?: string }> {
		const provider = this.providers.get(input.provider);
		if (!provider) {
			return { accepted: false, reason: `unknown provider: ${input.provider}` };
		}

		const connection = this.connections.get(
			this.connectionKey(input.tenantId, input.connectionId),
		);
		if (!connection) {
			return { accepted: false, reason: "unknown connection" };
		}

		if (connection.provider !== provider.name) {
			return { accepted: false, reason: "connection/provider mismatch" };
		}

		const trigger = this.resolveTrigger(provider, input.triggerKey);
		if (!trigger) {
			return {
				accepted: false,
				reason: `unknown trigger: ${input.triggerKey}`,
			};
		}

		const config = (connection.config ?? {}) as unknown;
		if (trigger.validateConfig) {
			trigger.validateConfig(config);
		}

		const state = await this.ensureTriggerSetup(connection, trigger, config);

		if (trigger.ingest) {
			await trigger.ingest({
				connection,
				config,
				state,
				body: input.body,
				headers: input.headers,
				tenantId: input.tenantId,
				triggerKey: input.triggerKey,
				provider: input.provider,
			});
		}

		const events = await this.transformAndPrepareEvents(
			{
				provider: input.provider,
				triggerKey: input.triggerKey,
				triggerVersion: trigger.version,
				connection,
				receivedAt: new Date().toISOString(),
				payload: input.body,
				meta: { headers: input.headers },
			},
			trigger,
		);

		await this.persistAndQueue(events, trigger);

		return { accepted: true };
	}

	startPolling(): void {
		if (this.pollingTimer !== null) return;
		this.runPollCycle().catch(() => {});
		this.pollingTimer = setInterval(() => {
			this.runPollCycle().catch(() => {});
		}, this.pollIntervalMs) as unknown as number;
	}

	stopPolling(): void {
		if (this.pollingTimer !== null) {
			clearInterval(this.pollingTimer);
			this.pollingTimer = null;
		}
	}

	async replay(filters?: {
		since?: string;
		until?: string;
		tenantId?: string;
		connectionId?: string;
	}): Promise<number> {
		const events = await this.eventStore.list(filters);
		if (events.length === 0) return 0;

		this.startDeliveryLoop();
		for (const event of events) {
			const job: DeliveryJob = {
				id: crypto.randomUUID(),
				eventId: event.id,
				attempt: 1,
				nextRunAt: Date.now(),
			};
			await this.queue.enqueue(job);
		}

		return events.length;
	}

	async replayDLQ(filters?: {
		tenantId?: string;
		connectionId?: string;
	}): Promise<number> {
		const entries = await this.eventStore.listDLQ(filters);
		if (entries.length === 0) return 0;

		this.startDeliveryLoop();
		for (const entry of entries) {
			const job: DeliveryJob = {
				id: crypto.randomUUID(),
				eventId: entry.eventId,
				attempt: 1,
				nextRunAt: Date.now(),
			};
			await this.queue.enqueue(job);
		}

		return entries.length;
	}

	private async runPollCycle(): Promise<void> {
		for (const provider of this.providers.values()) {
			for (const trigger of provider.getTriggers()) {
				if (!trigger.poll) continue;
				if (trigger.mode !== "poll" && trigger.mode !== "hybrid") continue;

				for (const connection of this.connections.values()) {
					if (connection.provider !== provider.name) continue;

					const config = (connection.config ?? {}) as unknown;
					if (trigger.validateConfig) {
						trigger.validateConfig(config);
					}

					const state = await this.ensureTriggerSetup(connection, trigger, config);
					const result = await trigger.poll({
						connection,
						config,
						state,
						provider: provider.name,
						triggerKey: trigger.key,
					});

					if (result?.state !== undefined) {
						const stateKey = this.stateKey(connection, trigger);
						this.triggerStates.set(stateKey, result.state);
					}

					const payloads = result?.payloads ?? [];
					if (payloads.length === 0) continue;

					for (const payload of payloads) {
						const events = await this.transformAndPrepareEvents(
							{
								provider: provider.name,
								triggerKey: trigger.key,
								triggerVersion: trigger.version,
								connection,
								receivedAt: new Date().toISOString(),
								payload,
								meta: result?.meta,
							},
							trigger,
						);

						await this.persistAndQueue(events, trigger);
					}
				}
			}
		}
	}

	private startDeliveryLoop(): void {
		if (this.deliveryTimer !== null) return;
		this.deliveryTimer = setInterval(() => {
			this.runDeliveryCycle().catch(() => {});
		}, DELIVERY_TICK_MS) as unknown as number;
	}

	private stopDeliveryLoop(): void {
		if (this.deliveryTimer !== null) {
			clearInterval(this.deliveryTimer);
			this.deliveryTimer = null;
		}
	}

	private async runDeliveryCycle(): Promise<void> {
		const jobs = await this.queue.lease(DELIVERY_LEASE_LIMIT);
		if (jobs.length === 0) return;

		for (const job of jobs) {
			const event = await this.eventStore.get(job.eventId);
			if (!event) {
				await this.queue.ack(job.id);
				continue;
			}

			try {
				for (const handler of this.handlers) {
					await handler(event);
				}
				await this.eventStore.markDelivery(event.id, job.attempt, "delivered");
				await this.queue.ack(job.id);
			} catch (err) {
				const error = err instanceof Error ? err.message : "unknown error";
				await this.eventStore.markDelivery(
					event.id,
					job.attempt,
					"failed",
					error,
				);
				await this.queue.fail(job.id, error);

				const nextAttempt = job.attempt + 1;
				if (nextAttempt > this.maxAttempts) {
					await this.eventStore.putDLQ(event.id, error);
					continue;
				}

				const backoffMs = this.computeBackoffMs(nextAttempt);
				const retryJob: DeliveryJob = {
					id: crypto.randomUUID(),
					eventId: event.id,
					attempt: nextAttempt,
					nextRunAt: Date.now() + backoffMs,
				};
				await this.queue.enqueue(retryJob);
			}
		}
	}

	private async transformAndPrepareEvents(
		input: TransformInput,
		trigger: TriggerDefinition,
	): Promise<EventEnvelope[]> {
		const base: EventEnvelope = {
			id: "",
			type: "",
			occurredAt: new Date().toISOString(),
			receivedAt: input.receivedAt,
			provider: input.provider,
			triggerKey: input.triggerKey,
			triggerVersion: input.triggerVersion,
			tenantId: input.connection.tenantId,
			connectionId: input.connection.connectionId,
			dedupeKey: "",
			data: { raw: input.payload },
			meta: input.meta ?? {},
		};

		const transformed = await trigger.transform(input);
		return transformed.map((event) => ({
			...base,
			...event,
			data: { raw: input.payload, ...(event.data ?? {}) },
			meta: { ...base.meta, ...(event.meta ?? {}) },
		}));
	}

	private async persistAndQueue(
		events: EventEnvelope[],
		trigger: TriggerDefinition,
	): Promise<void> {
		this.startDeliveryLoop();

		for (const event of events) {
			const dedupeKey = trigger.dedupe(event);
			const hasDedupe = await this.eventStore.hasDedupe(
				event.provider,
				event.connectionId,
				dedupeKey,
			);
			if (hasDedupe) continue;

			const id = await createEventId(
				event.provider,
				event.connectionId,
				dedupeKey,
			);
			const stored: EventEnvelope = { ...event, id, dedupeKey };

			await this.eventStore.put(stored);

			const job: DeliveryJob = {
				id: crypto.randomUUID(),
				eventId: stored.id,
				attempt: 1,
				nextRunAt: Date.now(),
			};

			await this.queue.enqueue(job);
		}
	}

	private resolveTrigger(
		provider: Provider,
		triggerKey: string,
	): TriggerDefinition | undefined {
		return provider.getTriggers().find((t) => t.key === triggerKey);
	}

	private connectionKey(tenantId: string, connectionId: string): string {
		return `${tenantId}:${connectionId}`;
	}

	private stateKey(
		connection: CoreConnection,
		trigger: TriggerDefinition,
	): string {
		return `${connection.tenantId}:${connection.connectionId}:${trigger.key}:${trigger.version}`;
	}

	private async ensureTriggerSetup(
		connection: CoreConnection,
		trigger: TriggerDefinition,
		config: unknown,
	): Promise<unknown> {
		const stateKey = this.stateKey(connection, trigger);
		if (!this.triggerSetupDone.has(stateKey)) {
			const result = await trigger.setup({
				connection,
				config,
			});
			if (result?.state !== undefined) {
				this.triggerStates.set(stateKey, result.state);
			}
			this.triggerSetupDone.add(stateKey);
		}

		return this.triggerStates.get(stateKey);
	}

	private async teardownTrigger(
		connection: CoreConnection,
		trigger: TriggerDefinition,
	): Promise<void> {
		const stateKey = this.stateKey(connection, trigger);
		if (!this.triggerSetupDone.has(stateKey)) return;

		const state = this.triggerStates.get(stateKey);
		await trigger.teardown({
			connection,
			state,
		});

		this.triggerStates.delete(stateKey);
		this.triggerSetupDone.delete(stateKey);
	}

	private computeBackoffMs(attempt: number): number {
		return Math.min(60_000, 1000 * 2 ** (attempt - 1));
	}
}
