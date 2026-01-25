import type { EventEnvelope } from "./event";
import type {
	PollContext,
	SetupContext,
	TeardownContext,
	TransformInput,
	WebhookContext,
} from "./runtimeTypes";

export type TriggerMode = "webhook" | "poll" | "hybrid";

export interface TriggerDefinition<TConfig = unknown, TState = unknown> {
	provider: string;
	key: string;
	version: string;
	mode: TriggerMode;

	validateConfig?(config: unknown): asserts config is TConfig;

	setup(ctx: SetupContext<TConfig>): Promise<{ state?: TState }>;
	teardown(ctx: TeardownContext<TState>): Promise<void>;

	ingest?(ctx: WebhookContext<TConfig, TState>): Promise<void>;
	poll?(ctx: PollContext<TConfig, TState>): Promise<{ state?: TState }>;

	transform(input: TransformInput): Promise<EventEnvelope[]>;
	dedupe(event: EventEnvelope): string;
}
