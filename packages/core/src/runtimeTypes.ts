import type { Connection } from "./connection"

export type SetupContext<TConfig> = {
  connection: Connection
  config: TConfig
}

export type TeardownContext<TState> = {
  connection: Connection
  state: TState | undefined
}

export type WebhookContext<TConfig, TState> = {
  connection: Connection
  config: TConfig
  state: TState | undefined
  body: unknown
  headers: Record<string, string>
  tenantId: string
  triggerKey: string
  provider: string
}

export type PollContext<TConfig, TState> = {
  connection: Connection
  config: TConfig
  state: TState | undefined
  provider: string
  triggerKey: string
}

export type TransformInput = {
  provider: string
  triggerKey: string
  triggerVersion: string
  connection: Connection
  receivedAt: string
  payload: unknown
  meta?: Record<string, unknown>
}
