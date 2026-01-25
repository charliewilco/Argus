import type { Connection } from "./connection"
import type { Provider } from "./provider"
import type { TriggerDefinition } from "./trigger"

export abstract class AbstractProvider implements Provider {
  abstract name: string
  abstract version: string
  protected triggers: TriggerDefinition[] = []

  getTriggers() {
    return this.triggers
  }

  protected registerTrigger(t: TriggerDefinition) {
    this.triggers.push(t)
  }

  abstract validateConnection(connection: unknown): asserts connection is Connection
}
