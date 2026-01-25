#!/usr/bin/env bun

import { SqliteEventStore } from "@argus/storage-sqlite";
import type { EventEnvelope } from "@argus/core/event";
import { pathToFileURL } from "node:url";
import path from "node:path";

const USAGE = `argus <command> [options]

Commands:
  replay --since <iso> --until <iso> [--tenant <id>] [--connection <id>] [--handler <path>]
  dlq list [--tenant <id>] [--connection <id>]
  dlq replay --event <id> [--handler <path>]

Options:
  --sqlite <path>   Path to SQLite DB (or set ARGUS_SQLITE_PATH)
  --handler <path>  Module exporting default or handleEvent(event)
  --help            Show help
`;

type Flags = Record<string, string | boolean>;

type ReplayFilters = {
	since?: string;
	until?: string;
	tenantId?: string;
	connectionId?: string;
};

function parseArgs(args: string[]): { positionals: string[]; flags: Flags } {
	const positionals: string[] = [];
	const flags: Flags = {};

	for (let i = 0; i < args.length; i += 1) {
		const arg = args[i];
		if (arg.startsWith("--")) {
			const key = arg.slice(2);
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
): Promise<((event: EventEnvelope) => Promise<void> | void) | null> {
	const handlerPath = flags.handler;
	if (typeof handlerPath !== "string" || handlerPath.length === 0) return null;

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

async function handleReplay(flags: Flags): Promise<void> {
	const store = new SqliteEventStore({ filename: getSqlitePath(flags) });
	const filters = buildFilters(flags);
	const events = await store.list(filters);
	const handler = await loadHandler(flags);

	if (handler) {
		for (const event of events) {
			await handler(event);
		}
	}

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
	if (handler) {
		await handler(event);
	}
	printJsonLines<EventEnvelope>([event]);
	process.stderr.write(`replayed dlq event ${eventId}\n`);
}

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

	printUsage(1);
}

try {
	await main();
} catch (err) {
	const message = err instanceof Error ? err.message : "unknown error";
	process.stderr.write(`${message}\n`);
	process.stderr.write(`${USAGE}\n`);
	process.exit(1);
}
