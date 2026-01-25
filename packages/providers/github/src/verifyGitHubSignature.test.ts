import { expect, test } from "bun:test"
import { createHmac } from "node:crypto"
import { verifyGitHubSignature } from "./verifyGitHubSignature"

function sign(secret: string, body: string): string {
  const hmac = createHmac("sha256", secret)
  hmac.update(body)
  return `sha256=${hmac.digest("hex")}`
}

test("verifyGitHubSignature validates correct signatures", () => {
  const secret = "super-secret"
  const body = JSON.stringify({ hello: "world" })
  const signature = sign(secret, body)

  expect(verifyGitHubSignature(secret, body, signature)).toBe(true)
})

test("verifyGitHubSignature rejects invalid signatures", () => {
  const secret = "super-secret"
  const body = "payload"

  expect(verifyGitHubSignature(secret, body, "sha256=deadbeef")).toBe(false)
})
