#!/usr/bin/env bun

import { access, writeFile } from "node:fs/promises";
import path from "node:path";
import { pathToFileURL } from "node:url";
import type { EventEnvelope } from "@argus/core/event";
import { MemoryQueue } from "@argus/queue-memory";
import { Runtime } from "@argus/runtime/runtime";
import { SqliteEventStore } from "@argus/storage-sqlite";

const USAGE = `argus <command> [options]

Commands:
  replay --since <iso> --until <iso> [--tenant <id>] [--connection <id>] [--normalized <json>] --handler <path>
  dlq list [--tenant <id>] [--connection <id>]
  dlq replay --event <id> --handler <path>
  scaffold handler <path>

Options:
  --sqlite <path>   Path to SQLite DB (or set ARGUS_SQLITE_PATH)
  --handler <path>  Module exporting default or handleEvent(event)
  --wait-ms <ms>    Max time to wait for delivery (default: 30000)
  --normalized <json> Filter by data.normalized fields (exact match)
  --help            Show help
`;

type Flags = Record<string, string | boolean>;

type ReplayFilters = {
	since?: string;
	until?: string;
	tenantId?: string;
	connectionId?: string;
	normalized?: Record<string, unknown>;
};

function parseArgs(args: string[]): { positionals: string[]; flags: Flags } {
	const positionals: string[] = [];
	const flags: Flags = {};
	const booleanFlags = new Set(["help", "dry-run"]);

	for (let i = 0; i < args.length; i += 1) {
		const arg = args[i];
		if (arg.startsWith("--")) {
			const key = arg.slice(2);
			if (booleanFlags.has(key)) {
				flags[key] = true;
				continue;
			}
			const next = args[i + 1];
			if (!next || next.startsWith("--")) {
				flags[key] = true;
			} else {
				flags[key] = next;
				i += 1;
			}
		} else {
			positionals.push(arg);
		}
	}

	return { positionals, flags };
}

function requireStringFlag(flags: Flags, key: string): string {
	const value = flags[key];
	if (typeof value === "string" && value.length > 0) return value;
	throw new Error(`Missing required flag: --${key}`);
}

function getSqlitePath(flags: Flags): string {
	const value = flags.sqlite;
	if (typeof value === "string" && value.length > 0) return value;
	const env = process.env.ARGUS_SQLITE_PATH;
	if (env) return env;
	throw new Error("Missing SQLite path (use --sqlite or ARGUS_SQLITE_PATH)");
}

function buildFilters(flags: Flags): ReplayFilters {
	const filters: ReplayFilters = {};
	if (typeof flags.since === "string") filters.since = flags.since;
	if (typeof flags.until === "string") filters.until = flags.until;
	if (typeof flags.tenant === "string") filters.tenantId = flags.tenant;
	if (typeof flags.connection === "string")
		filters.connectionId = flags.connection;
	if (typeof flags.normalized === "string") {
		let parsed: unknown;
		try {
			parsed = JSON.parse(flags.normalized);
		} catch {
			throw new Error("Invalid --normalized JSON");
		}
		if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
			throw new Error("--normalized must be a JSON object");
		}
		filters.normalized = parsed as Record<string, unknown>;
	} else if (flags.normalized === true) {
		throw new Error("--normalized requires a JSON value");
	}
	return filters;
}

function printJsonLines<T>(items: T[]): void {
	for (const item of items) {
		process.stdout.write(`${JSON.stringify(item)}\n`);
	}
}

function printUsage(exitCode = 0): never {
	process.stdout.write(`${USAGE}\n`);
	process.exit(exitCode);
}

async function loadHandler(
	flags: Flags,
): Promise<(event: EventEnvelope) => Promise<void> | void> {
	const handlerPath = flags.handler;
	if (typeof handlerPath !== "string" || handlerPath.length === 0) {
		throw new Error("Missing required flag: --handler");
	}

	const resolved = path.isAbsolute(handlerPath)
		? handlerPath
		: path.resolve(process.cwd(), handlerPath);
	const moduleUrl = pathToFileURL(resolved).href;
	const mod = await import(moduleUrl);
	const handler = (mod.default ?? mod.handleEvent) as
		| ((event: EventEnvelope) => Promise<void> | void)
		| undefined;

	if (typeof handler !== "function") {
		throw new Error(
			"handler module must export a default or handleEvent function",
		);
	}

	return handler;
}

async function waitForQueueIdle(
	queue: MemoryQueue,
	timeoutMs: number,
): Promise<void> {
	const deadline = Date.now() + timeoutMs;

	while (Date.now() < deadline) {
		const stats = queue.getStats();
		if (stats.pending === 0 && stats.inFlight === 0) return;

		const now = Date.now();
		const nextDelay =
			stats.nextRunAt === null ? 100 : Math.max(25, stats.nextRunAt - now);
		await new Promise((resolve) =>
			setTimeout(resolve, Math.min(250, nextDelay)),
		);
	}

	throw new Error("Timed out waiting for delivery to finish");
}

async function fileExists(targetPath: string): Promise<boolean> {
	try {
		await access(targetPath);
		return true;
	} catch {
		return false;
	}
}

async function handleScaffoldHandler(targetPath: string): Promise<void> {
	const resolved = path.isAbsolute(targetPath)
		? targetPath
		: path.resolve(process.cwd(), targetPath);

	if (await fileExists(resolved)) {
		throw new Error(`file already exists: ${resolved}`);
	}

	const template = `export default async function handleEvent(event) {\n\tconsole.log("EVENT", JSON.stringify(event, null, 2));\n}\n`;
	await writeFile(resolved, template, "utf8");
	process.stderr.write(`wrote ${resolved}\n`);
}

async function handleReplay(flags: Flags): Promise<void> {
	const store = new SqliteEventStore({ filename: getSqlitePath(flags) });
	const filters = buildFilters(flags);
	const events = await store.list(filters);
	const handler = await loadHandler(flags);
	const queue = new MemoryQueue();
	const runtime = new Runtime({ eventStore: store, queue });
	runtime.onEvent(handler);

	const ids = events.map((event) => event.id);
	await runtime.replayByIds(ids);
	const waitMs =
		typeof flags["wait-ms"] === "string" ? Number(flags["wait-ms"]) : 30_000;
	if (!Number.isFinite(waitMs) || waitMs <= 0) {
		throw new Error("Invalid --wait-ms value");
	}
	await waitForQueueIdle(queue, waitMs);
	runtime.shutdown();

	printJsonLines(events);
	process.stderr.write(`replayed ${events.length} events\n`);
}

async function handleDlqList(flags: Flags): Promise<void> {
	const store = new SqliteEventStore({ filename: getSqlitePath(flags) });
	const entries = await store.listDLQ(buildFilters(flags));
	printJsonLines(entries);
	process.stderr.write(`listed ${entries.length} dlq entries\n`);
}

async function handleDlqReplay(flags: Flags): Promise<void> {
	const eventId = requireStringFlag(flags, "event");
	const store = new SqliteEventStore({ filename: getSqlitePath(flags) });
	const entries = await store.listDLQ();
	const entry = entries.find((item) => item.eventId === eventId);
	if (!entry) {
		throw new Error(`event not found in dlq: ${eventId}`);
	}
	const event = await store.get(eventId);
	if (!event) {
		throw new Error(`event missing from store: ${eventId}`);
	}
	const handler = await loadHandler(flags);
	const queue = new MemoryQueue();
	const runtime = new Runtime({ eventStore: store, queue });
	runtime.onEvent(handler);

	await runtime.replayByIds([eventId]);
	const waitMs =
		typeof flags["wait-ms"] === "string" ? Number(flags["wait-ms"]) : 30_000;
	if (!Number.isFinite(waitMs) || waitMs <= 0) {
		throw new Error("Invalid --wait-ms value");
	}
	await waitForQueueIdle(queue, waitMs);
	runtime.shutdown();

	printJsonLines<EventEnvelope>([event]);
	process.stderr.write(`replayed dlq event ${eventId}\n`);
}

export const __test__ = {
	parseArgs,
	requireStringFlag,
	getSqlitePath,
	buildFilters,
	waitForQueueIdle,
};

async function main(): Promise<void> {
	const { positionals, flags } = parseArgs(process.argv.slice(2));
	if (flags.help) printUsage(0);

	const [command, subcommand] = positionals;

	if (!command) printUsage(1);

	if (command === "replay") {
		await handleReplay(flags);
		return;
	}

	if (command === "dlq" && subcommand === "list") {
		await handleDlqList(flags);
		return;
	}

	if (command === "dlq" && subcommand === "replay") {
		await handleDlqReplay(flags);
		return;
	}

	if (command === "scaffold" && subcommand === "handler") {
		const target = positionals[2];
		if (!target) {
			throw new Error("Missing scaffold handler path");
		}
		await handleScaffoldHandler(target);
		return;
	}

	printUsage(1);
}

if (import.meta.main) {
	try {
		await main();
	} catch (err) {
		const message = err instanceof Error ? err.message : "unknown error";
		process.stderr.write(`${message}\n`);
		process.stderr.write(`${USAGE}\n`);
		process.exit(1);
	}
}
