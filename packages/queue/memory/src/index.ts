import type { DeliveryJob, Queue } from "@argus/core/queue";

export class MemoryQueue implements Queue {
	private pending: DeliveryJob[] = [];
	private inFlight = new Map<string, DeliveryJob>();

	async enqueue(job: DeliveryJob): Promise<void> {
		this.pending.push(job);
	}

	async lease(limit: number): Promise<DeliveryJob[]> {
		const now = Date.now();
		const ready: DeliveryJob[] = [];
		const remaining: DeliveryJob[] = [];

		for (const job of this.pending) {
			if (ready.length >= limit) {
				remaining.push(job);
				continue;
			}

			if (job.nextRunAt <= now) {
				ready.push(job);
				this.inFlight.set(job.id, job);
			} else {
				remaining.push(job);
			}
		}

		this.pending = remaining;
		return ready;
	}

	async ack(jobId: string): Promise<void> {
		this.inFlight.delete(jobId);
	}

	async fail(jobId: string, _error: string): Promise<void> {
		this.inFlight.delete(jobId);
	}
}
