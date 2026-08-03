# Development Guide

## Prerequisites

- Go 1.22+

## Building

This is a library module with no binary output. To verify it compiles:

```bash
go build ./...
```

## Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run with coverage
go test -cover ./...
```

## Key Files

| File | Purpose |
|------|---------|
| `event.go` | Core `Event`, `EventSubject`, `EventActor` structs |
| `event_types.go` | Event type constants (e.g., `EventTypeIssueOpened`) |
| `event_data.go` | Typed data payloads (`IssueData`, `PullRequestData`, etc.) |
| `filter.go` | `Filter` struct, `Match`, `MatchFilters`, `MatchEventType` |
| `filter_deepcopy.go` | DeepCopy methods for CRD compatibility |
| `nats.go` | NATS stream names, subject patterns, helper functions |

## Adding a New Event Type

1. Add the constant to `event_types.go`
2. Add the data payload struct to `event_data.go` if needed
3. Update the normalizer in `ainsel-event-source-gateway-forgejo` to produce the new type
4. Add tests in `event_test.go`

## Adding a New Filter Operator

1. Add the case to the `Match` method's switch statement in `filter.go`
2. Add test cases in `filter_test.go`
3. Document the operator in `docs/event-schema.md`

## Contributing

- Follow standard Go conventions
- All exported types and functions must have doc comments
- All new functionality must have tests
