import { expect, test } from "bun:test";
import { MemoryQueue } from "@argus/queue-memory";
import { __test__ } from "./cli";

test("cli workspace is wired for tests", () => {
	expect(true).toBe(true);
});

test("parseArgs splits positionals and flags", () => {
	const { positionals, flags } = __test__.parseArgs([
		"replay",
		"--since",
		"2024-01-01",
		"--dry-run",
		"extra",
	]);

	expect(positionals).toEqual(["replay", "extra"]);
	expect(flags.since).toBe("2024-01-01");
	expect(flags["dry-run"]).toBe(true);
});

test("requireStringFlag enforces required flags", () => {
	expect(() => __test__.requireStringFlag({}, "handler")).toThrow(
		"Missing required flag: --handler",
	);
	expect(__test__.requireStringFlag({ handler: "path" }, "handler")).toBe(
		"path",
	);
});

test("getSqlitePath reads flags or env", () => {
	const original = process.env.ARGUS_SQLITE_PATH;
	process.env.ARGUS_SQLITE_PATH = "/tmp/argus.sqlite";
	try {
		expect(__test__.getSqlitePath({})).toBe("/tmp/argus.sqlite");
		expect(__test__.getSqlitePath({ sqlite: "db.sqlite" })).toBe("db.sqlite");
	} finally {
		if (original === undefined) {
			delete process.env.ARGUS_SQLITE_PATH;
		} else {
			process.env.ARGUS_SQLITE_PATH = original;
		}
	}
});

test("buildFilters maps flag names to filter fields", () => {
	const filters = __test__.buildFilters({
		since: "2024-01-01",
		until: "2024-02-01",
		tenant: "tenant",
		connection: "conn",
		normalized: '{"repo":"a"}',
	});
	expect(filters).toEqual({
		since: "2024-01-01",
		until: "2024-02-01",
		tenantId: "tenant",
		connectionId: "conn",
		normalized: { repo: "a" },
	});
});

test("waitForQueueIdle resolves when queue is empty", async () => {
	const queue = new MemoryQueue();
	await __test__.waitForQueueIdle(queue, 50);
});
