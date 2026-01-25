# GitHub Provider

Package: `@argus/provider-github`

Exports:
- `GitHubProvider`
- `verifyGitHubSignature(secret, body, signatureHeader)`

## Webhook Trigger
Key: `issue.created`

Behavior:
- Handles `issues` webhooks with `action=opened`.
- Dedupe uses `x-github-delivery` header; fallback `issue.id + updated_at`.
- Normalized fields:
  - `repoFullName`
  - `issueNumber`
  - `title`
  - `userLogin`
  - `url`

## Polling Trigger
Key: `issues.updated`

Behavior:
- Polls GitHub issues updated since cursor time.
- Default lookback is 24h on first run.
- Pagination uses the `link` header.
