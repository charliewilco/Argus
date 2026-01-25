# Queue

Queues implement `Queue` from `@argus/core/queue`.

## Implementations
- `@argus/queue-memory`: in-memory queue with `nextRunAt` scheduling

The runtime polls the queue at a fixed interval and handles retries.
