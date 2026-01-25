import { expect, test } from "bun:test"
import { createEventId } from "./id"

test("createEventId is deterministic", async () => {
  const a = await createEventId("github", "conn_1", "key_1")
  const b = await createEventId("github", "conn_1", "key_1")
  const c = await createEventId("github", "conn_2", "key_1")

  expect(a).toBe(b)
  expect(a).not.toBe(c)
})
