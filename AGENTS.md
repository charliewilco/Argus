# Repository Guidelines

## Project Structure & Module Organization
- Monorepo managed with Bun workspaces (see `package.json`).
- `packages/` contains core libraries, runtime, storage, queue, providers, and CLI packages.
- `apps/` contains example or integration apps (see `apps/example`).
- Each workspace is a real package with its own `package.json` and is designed to be testable in isolation.
- Workspace imports use package names like `@argus/core` (no TS path aliases).

## Build, Test, and Development Commands
- `bun install`: install workspace dependencies.
- `bun test`: run tests for the current workspace (root runs the default test suite).
- `bun test packages/core`: run tests scoped to a specific workspace directory.

## Coding Style & Naming Conventions
- TypeScript, ESM-only (`"type": "module"`).
- Follow existing patterns in `packages/*` for file layout and exports.
- Use explicit package names for cross-workspace imports (example: `import { Runtime } from "@argus/runtime"`).
- Keep provider implementations in `packages/providers/<provider-name>` and name exports with clear provider-specific prefixes (example: `GitHubProvider`).

## Testing Guidelines
- Test runner: `bun test`.
- Place tests alongside source or in a `tests/` folder within each workspace, consistent with existing packages.
- Name tests descriptively by behavior (example: `runtime.dedupe.test.ts`).

## Commit & Pull Request Guidelines
- No commit history exists yet, so no established commit message convention.
- For PRs: include a concise summary, affected packages, and any manual test steps.
- If changes affect providers or runtime behavior, include example payloads or replay steps.

## Architecture Notes
- Providers implement the shared trigger interface and can be tested independently.
- Runtime does not host HTTP; apps (like `apps/example`) wire HTTP routes to `runtime.handleWebhook(...)`.

## Agent-Specific Instructions
- Follow `SPEC.md` for current module responsibilities and build order.
- Avoid adding TS path aliases; use workspace package imports instead.
