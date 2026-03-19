# Queue

Queues implement `Queue` from `@argus/core/queue`.

## Implementations
- `@argus/queue-memory`: in-memory queue with `nextRunAt` scheduling

The runtime polls the queue at a fixed interval and handles retries.

## MemoryQueue

```typescript
import { MemoryQueue } from "@argus/queue-memory";
import { Runtime } from "@argus/runtime/runtime";
import { MemoryEventStore } from "@argus/storage-memory";

const runtime = new Runtime({
  eventStore: new MemoryEventStore(),
  queue: new MemoryQueue(),
});
```

The `MemoryQueue` supports:
- Job scheduling via `nextRunAt` (millisecond epoch timestamp)
- Job leasing — jobs are hidden from other consumers while being processed
- Acknowledgment (`ack`) on success
- Failure recording (`fail`) with automatic retry scheduling by the runtime

Jobs are scheduled with exponential backoff by the runtime — you do not need to manage retry timing manually.

## Implementing a custom queue

To integrate an external queue (e.g. Redis, SQS), implement the `Queue` interface from `@argus/core/queue`:

```typescript
import type { Queue, DeliveryJob } from "@argus/core/queue";

class MyQueue implements Queue {
  async enqueue(job: DeliveryJob): Promise<void> {
    // Add job to your queue backend
  }

  async lease(limit: number): Promise<DeliveryJob[]> {
    // Return up to `limit` jobs whose nextRunAt <= Date.now()
    // Mark them as in-flight so they are not returned again
    return [];
  }

  async ack(jobId: string): Promise<void> {
    // Remove job from the queue — delivery succeeded
  }

  async fail(jobId: string, error: string): Promise<void> {
    // Record failure — runtime will enqueue a retry job with backoff
  }
}
```
