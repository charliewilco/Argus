export async function createEventId(
  provider: string,
  connectionId: string,
  dedupeKey: string,
): Promise<string> {
  const input = `${provider}:${connectionId}:${dedupeKey}`
  const encoder = new TextEncoder()
  const data = encoder.encode(input)
  const hashBuffer = await crypto.subtle.digest("SHA-256", data)
  const hashArray = Array.from(new Uint8Array(hashBuffer))
  return hashArray.map((b) => b.toString(16).padStart(2, "0")).join("")
}
