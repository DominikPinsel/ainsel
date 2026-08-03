# Shared API

Go module shared by every Go component in the platform. Provides:

- The canonical `Event` struct and event-type constants.
- Typed event-data payloads (`IssueData`, `PullRequestData`, `PushData`, …).
- The `Filter` struct and `MatchFilters` evaluation engine used by
  `Trigger` CRDs.
- NATS stream and subject constants (`EVENTS`, `AGENTS`, `HUB`).

## Role in the platform

See [`docs/architecture.md`](../../docs/architecture.md).

Pure library — no binary, no runtime, no main package. Imported by
[`services/hub/`](../../services/hub/),
[`services/webhook-receiver/`](../../services/webhook-receiver/),
[`operators/agent/`](../../operators/agent/),
[`operators/event-gateway/`](../../operators/event-gateway/), and the
agent runtime.

## Importing from another package in the workspace

Inside the monorepo, `go.work` joins this module automatically — no
`replace` directive needed. Just import:

```go
import api "github.com/DominikPinsel/ainsel/shared/api"
```

Outside the monorepo, vendor the source or pin a version tag.

## Testing

```bash
go test ./...
```

## Reference

- [Event schema](../../docs/event-schema.md)
