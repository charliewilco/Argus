import { expect, test } from "bun:test"
import { SqliteEventStore } from "./"

test("SqliteEventStore is not implemented yet", () => {
  expect(() => new SqliteEventStore()).toThrow("SqliteEventStore not implemented yet")
})
