import { createHmac, timingSafeEqual } from "node:crypto"

export function verifyGitHubSignature(
  secret: string,
  body: string | Uint8Array,
  signatureHeader: string | null | undefined,
): boolean {
  if (!signatureHeader) return false
  const expected = `sha256=${hmacSha256(secret, body)}`

  const a = Buffer.from(expected)
  const b = Buffer.from(signatureHeader)
  if (a.length !== b.length) return false

  return timingSafeEqual(a, b)
}

function hmacSha256(secret: string, body: string | Uint8Array): string {
  const hmac = createHmac("sha256", secret)
  hmac.update(body)
  return hmac.digest("hex")
}
