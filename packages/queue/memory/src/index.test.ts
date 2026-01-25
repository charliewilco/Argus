import { expect, test } from "bun:test"
import { MemoryQueue } from "./index"

test("MemoryQueue leases ready jobs", async () => {
  const queue = new MemoryQueue()
  await queue.enqueue({
    id: "job-1",
    eventId: "event-1",
    attempt: 1,
    nextRunAt: Date.now() - 1000,
  })

  const jobs = await queue.lease(10)
  expect(jobs.length).toBe(1)
  expect(jobs[0]?.id).toBe("job-1")
})
