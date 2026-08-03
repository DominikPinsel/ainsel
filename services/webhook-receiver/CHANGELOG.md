# Changelog

All notable changes to this project will be documented in this file.

### Bug Fixes

- Capture specific label in issue.label.added and pull_request.label.added events (10056e4)
- Remove obsolete Forgejo connector code and unused functions (ee7219c)
- Address PR review comments (9ce3f4f)
- Derive event type from real webhook headers; add CI for webhook-receiver (3057fd6)
- Address review feedback on PR #485 (3ac41a7)

### Documentation

- Refresh README to library template (ea154b4)

### Features

- Scaffold shared OIDC middleware module (f2a61a4)
- Add pull_request.label.added and pull_request.synchronize events (c5ea848)
- Add workflow_run.failed event type and normalizer support (67f5945)
- Simplify Event struct, remove EventsSubject event type segment (8bb0374)
- Remove EventType and IgnoreBotEvents from TriggerSpec, delete MatchEventType (dfda4ac)
- New generic inbound webhook service (HMAC verify → NATS publish) (32e8ac5)
- Add chat MCP sidecar service (e054d54)
- Add chat event handling in pi runner and NATS routing (ce04fc1)

### Refactoring

- Publish events via HTTP to hub (3aecff5)

### Testing

- Add trigger scenario tests for label-based triggers (0764159)

### Deps

- Add prometheus/client_golang and google/uuid (0d8c34a)

### Merge

- Resolve conflict with main (578a38e)

