export type DeliveryJob = {
  id: string
  eventId: string
  attempt: number
  nextRunAt: number
}

export interface Queue {
  enqueue(job: DeliveryJob): Promise<void>
  lease(limit: number): Promise<DeliveryJob[]>
  ack(jobId: string): Promise<void>
  fail(jobId: string, error: string): Promise<void>
}
