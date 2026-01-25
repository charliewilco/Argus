import type { Connection as CoreConnection } from "@argus/core/connection";
import type { EventEnvelope } from "@argus/core/event";
import type { EventStore as CoreEventStore } from "@argus/core/eventStore";
import { createEventId } from "@argus/core/id";
import type { Provider } from "@argus/core/provider";
import type { Queue as CoreQueue, DeliveryJob } from "@argus/core/queue";
import type { TransformInput } from "@argus/core/runtimeTypes";
import type { TriggerDefinition } from "@argus/core/trigger";

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
	private tenantScope?: string;
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
		tenantScope?: string;
	}) {
		this.eventStore = opts.eventStore;
		this.queue = opts.queue;
		this.maxAttempts = opts.maxAttempts ?? DEFAULT_MAX_ATTEMPTS;
		this.pollIntervalMs = opts.pollIntervalMs ?? DEFAULT_POLL_INTERVAL_MS;
		this.tenantScope = opts.tenantScope;
	}

	registerProvider(provider: Provider): void {
		this.providers.set(provider.name, provider);
	}

	registerConnection(connection: CoreConnection): void {
		if (this.tenantScope && connection.tenantId !== this.tenantScope) {
			throw new Error("connection tenant out of scope");
		}
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
		triggerVersion?: string;
		body: unknown;
		headers: Record<string, string>;
		tenantId: string;
		connectionId: string;
	}): Promise<{ accepted: boolean; reason?: string }> {
		if (this.tenantScope && input.tenantId !== this.tenantScope) {
			return { accepted: false, reason: "tenant out of scope" };
		}
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

		const trigger = this.resolveTrigger(
			provider,
			input.triggerKey,
			input.triggerVersion,
		);
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

	shutdown(): void {
		this.stopPolling();
		this.stopDeliveryLoop();
	}

	async replay(filters?: {
		since?: string;
		until?: string;
		tenantId?: string;
		connectionId?: string;
		normalized?: Record<string, unknown>;
	}): Promise<number> {
		const scoped = this.applyTenantScope(filters);
		const events = await this.eventStore.list(scoped);
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

	async replayByIds(eventIds: string[]): Promise<number> {
		if (eventIds.length === 0) return 0;
		this.startDeliveryLoop();

		let count = 0;
		for (const eventId of eventIds) {
			const event = await this.eventStore.get(eventId);
			if (!event) continue;
			if (this.tenantScope && event.tenantId !== this.tenantScope) {
				throw new Error("event tenant out of scope");
			}
			const job: DeliveryJob = {
				id: crypto.randomUUID(),
				eventId: event.id,
				attempt: 1,
				nextRunAt: Date.now(),
			};
			await this.queue.enqueue(job);
			count += 1;
		}

		return count;
	}

	async replayDLQ(filters?: {
		tenantId?: string;
		connectionId?: string;
	}): Promise<number> {
		const scoped = this.applyTenantScope(filters);
		const entries = await this.eventStore.listDLQ(scoped);
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
			for (const trigger of this.getLatestTriggers(provider)) {
				if (!trigger.poll) continue;
				if (trigger.mode !== "poll" && trigger.mode !== "hybrid") continue;

				for (const connection of this.connections.values()) {
					if (connection.provider !== provider.name) continue;

					const config = (connection.config ?? {}) as unknown;
					if (trigger.validateConfig) {
						trigger.validateConfig(config);
					}

					const state = await this.ensureTriggerSetup(
						connection,
						trigger,
						config,
					);
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
				event.tenantId,
				event.connectionId,
				dedupeKey,
			);
			if (hasDedupe) continue;

			const id = await createEventId(
				event.provider,
				event.tenantId,
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
		triggerVersion?: string,
	): TriggerDefinition | undefined {
		const matches = provider.getTriggers().filter((t) => t.key === triggerKey);
		if (matches.length === 0) return undefined;
		if (triggerVersion) {
			return matches.find((t) => t.version === triggerVersion);
		}
		return this.selectLatestTrigger(matches);
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

	private selectLatestTrigger(
		triggers: TriggerDefinition[],
	): TriggerDefinition {
		return triggers.reduce((latest, current) =>
			this.compareVersions(current.version, latest.version) > 0
				? current
				: latest,
		);
	}

	private getLatestTriggers(provider: Provider): TriggerDefinition[] {
		const byKey = new Map<string, TriggerDefinition[]>();
		for (const trigger of provider.getTriggers()) {
			const list = byKey.get(trigger.key) ?? [];
			list.push(trigger);
			byKey.set(trigger.key, list);
		}

		const latest: TriggerDefinition[] = [];
		for (const list of byKey.values()) {
			latest.push(this.selectLatestTrigger(list));
		}
		return latest;
	}

	private compareVersions(a: string, b: string): number {
		const aParts = a.split(".").map((part) => Number.parseInt(part, 10));
		const bParts = b.split(".").map((part) => Number.parseInt(part, 10));
		const length = Math.max(aParts.length, bParts.length);

		for (let i = 0; i < length; i += 1) {
			const aVal = aParts[i] ?? 0;
			const bVal = bParts[i] ?? 0;
			if (aVal === bVal) continue;
			return aVal > bVal ? 1 : -1;
		}

		return 0;
	}

	private applyTenantScope<T extends { tenantId?: string }>(
		filters: T | undefined,
	): T | undefined {
		if (!this.tenantScope) return filters;
		if (!filters) return { tenantId: this.tenantScope } as T;
		if (filters.tenantId && filters.tenantId !== this.tenantScope) {
			throw new Error("tenant out of scope");
		}
		return { ...filters, tenantId: this.tenantScope };
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
