import { expect, test } from "bun:test";
import { MemoryQueue } from "./";

test("MemoryQueue leases ready jobs", async () => {
	const queue = new MemoryQueue();
	await queue.enqueue({
		id: "job-1",
		eventId: "event-1",
		attempt: 1,
		nextRunAt: Date.now() - 1000,
	});

	const jobs = await queue.lease(10);
	expect(jobs.length).toBe(1);
	expect(jobs[0]?.id).toBe("job-1");
});

test("MemoryQueue respects lease limits and future jobs", async () => {
	const queue = new MemoryQueue();
	await queue.enqueue({
		id: "job-ready",
		eventId: "event-ready",
		attempt: 1,
		nextRunAt: Date.now() - 10,
	});
	await queue.enqueue({
		id: "job-ready-2",
		eventId: "event-ready-2",
		attempt: 1,
		nextRunAt: Date.now() - 10,
	});
	await queue.enqueue({
		id: "job-future",
		eventId: "event-future",
		attempt: 1,
		nextRunAt: Date.now() + 10_000,
	});

	const jobs = await queue.lease(1);
	expect(jobs.length).toBe(1);
	expect(jobs[0]?.id).toBe("job-ready");

	const remaining = await queue.lease(10);
	expect(remaining.map((job) => job.id)).toEqual(["job-ready-2"]);
});

test("MemoryQueue tracks pending and inflight stats", async () => {
	const queue = new MemoryQueue();
	await queue.enqueue({
		id: "job-1",
		eventId: "event-1",
		attempt: 1,
		nextRunAt: Date.now() + 5000,
	});

	let stats = queue.getStats();
	expect(stats.pending).toBe(1);
	expect(stats.inFlight).toBe(0);
	expect(stats.nextRunAt).not.toBeNull();

	const leased = await queue.lease(1);
	expect(leased.length).toBe(0);

	stats = queue.getStats();
	expect(stats.pending).toBe(1);

	await queue.enqueue({
		id: "job-2",
		eventId: "event-2",
		attempt: 1,
		nextRunAt: Date.now() - 10,
	});
	const leasedReady = await queue.lease(1);
	expect(leasedReady.length).toBe(1);

	stats = queue.getStats();
	expect(stats.inFlight).toBe(1);

	await queue.ack(leasedReady[0]?.id ?? "");
	stats = queue.getStats();
	expect(stats.inFlight).toBe(0);
});
