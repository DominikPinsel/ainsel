# Changelog

All notable changes to this project will be documented in this file.

### Bug Fixes

- Capture specific label in issue.label.added and pull_request.label.added events (10056e4)
- Add missing secret field to AgentImage env var CRD schema (157c098)
- Remove obsolete Forgejo connector code and unused functions (ee7219c)
- Address PR review comments (9ce3f4f)
- Derive event type from real webhook headers; add CI for webhook-receiver (3057fd6)
- Include shared/api in manifests target (674579d)
- Correct kubebuilder PROJECT domain to compose to ainsel.dev (bf84a47)
- Clean up stale go.sum entries and fix operator tests (6dfbd1f)

### Documentation

- Update CRD group references in docs to ainsel.dev (38d4343)
- Refresh README to package template (528029b)
- Refresh README to library template (ea154b4)
- Correct reconciled-resources and pi-extensions layout (41f5a21)

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

- Remove NATS consumer management (b59b1e0)

### Testing

- Add trigger scenario tests for label-based triggers (0764159)

### Merge

- Resolve conflict with main (578a38e)

