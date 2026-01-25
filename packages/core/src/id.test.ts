import { expect, test } from "bun:test";
import { createEventId } from "./id";

test("createEventId is deterministic", async () => {
	const a = await createEventId("github", "tenant_a", "conn_1", "key_1");
	const b = await createEventId("github", "tenant_a", "conn_1", "key_1");
	const c = await createEventId("github", "tenant_a", "conn_2", "key_1");
	const d = await createEventId("github", "tenant_b", "conn_1", "key_1");

	expect(a).toBe(b);
	expect(a).not.toBe(c);
	expect(a).not.toBe(d);
});
