# Changelog

All notable changes to this project will be documented in this file.

### Bug Fixes

- Address personas review findings (501f7ca)
- Add shared/auth to Dockerfile for OIDC dependency (ce6db54)
- Capture specific label in issue.label.added and pull_request.label.added events (10056e4)
- Remove obsolete Forgejo connector code and unused functions (ee7219c)
- Address PR review comments (9ce3f4f)
- Derive event type from real webhook headers; add CI for webhook-receiver (3057fd6)
- Revert incorrect nuid and go-difflib version bumps in go.mod (f16b3f0)
- Clean up stale go.sum entries and fix operator tests (6dfbd1f)

### Documentation

- Refresh README to package template (e18f82e)
- Refresh README to library template (ea154b4)

### Features

- Add jwt + keyfunc dependencies (9ac8e1f)
- JWT-validating auth middleware (a322e5b)
- OIDC JWT validation middleware + /api/v1/auth/me (#66) (b0b5285)
- Add personas package — Store (DB layer) (94f30ea)
- Scaffold shared OIDC middleware module (f2a61a4)
- Add pull_request.label.added and pull_request.synchronize events (c5ea848)
- Add workflow_run.failed event type and normalizer support (67f5945)
- Configure MCP servers per agent image with URL + token and refresh tools (4fc763b)
- Simplify Event struct, remove EventsSubject event type segment (8bb0374)
- Remove EventType and IgnoreBotEvents from TriggerSpec, delete MatchEventType (dfda4ac)
- New generic inbound webhook service (HMAC verify → NATS publish) (32e8ac5)
- Add chat MCP sidecar service (e054d54)
- Add chat event handling in pi runner and NATS routing (ce04fc1)

### Refactoring

- Use shared/auth/oidc; rename OIDC_CLIENT_ID → OIDC_PROJECT_ID (7a48925)
- Make golang.org/x/sync a direct dependency (c1ce3e1)
- Remove all NATS dependencies from hub, MCP, and shared API (d1a065a)

### Testing

- Add trigger scenario tests for label-based triggers (0764159)

### Merge

- Resolve conflict with main (578a38e)

