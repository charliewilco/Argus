# Example App

`apps/example` shows how to wire the runtime into a Bun server.

Highlights:
- Uses `Bun.serve()` to expose a webhook endpoint.
- Verifies GitHub signatures with `verifyGitHubSignature`.
- Calls `runtime.handleWebhook(...)` and logs events.
- Optional forced handler failure via `ARGUS_FAIL_HANDLER=1`.
