import { expect, test } from "bun:test";
import { AbstractProvider } from "./abstractProvider";
import type { Connection } from "./connection";
import type { TriggerDefinition } from "./trigger";

class TestProvider extends AbstractProvider {
	name = "test";
	version = "0.0.1";

	validateConnection(_connection: unknown): asserts _connection is Connection {}

	addTrigger(trigger: TriggerDefinition) {
		this.registerTrigger(trigger);
	}
}

test("AbstractProvider tracks registered triggers", () => {
	const provider = new TestProvider();
	expect(provider.getTriggers()).toHaveLength(0);

	const trigger: TriggerDefinition = {
		provider: "test",
		key: "event",
		version: "1",
		mode: "webhook",
		async setup() {
			return {};
		},
		async teardown() {},
		async transform() {
			return [];
		},
		dedupe() {
			return "dedupe";
		},
	};

	provider.addTrigger(trigger);
	expect(provider.getTriggers()).toHaveLength(1);
	expect(provider.getTriggers()[0]?.key).toBe("event");
});
