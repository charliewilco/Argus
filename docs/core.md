# Core Types

## EventEnvelope
`@argus/core/event`

An event carries both raw payloads and optional normalized data:
- `data.raw` is the original webhook/poll payload
- `data.normalized` is a provider-specific subset

## TriggerDefinition
`@argus/core/trigger`

Triggers define:
- `mode`: `webhook` | `poll` | `hybrid`
- `setup` / `teardown` lifecycle hooks
- `ingest` (webhook) and/or `poll` (polling)
- `transform` into `EventEnvelope[]`
- `dedupe` key generation

## Provider
`@argus/core/provider`

Providers expose:
- `name` and `version`
- `getTriggers()`
- `validateConnection()`

`AbstractProvider` is a helper base class for building providers.

## Connection
`@argus/core/connection`

Connections bind a tenant + provider to auth/config data.
