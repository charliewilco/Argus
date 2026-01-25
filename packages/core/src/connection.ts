export type Connection = {
  tenantId: string
  connectionId: string
  provider: string
  auth: Record<string, unknown>
  config?: Record<string, unknown>
}
