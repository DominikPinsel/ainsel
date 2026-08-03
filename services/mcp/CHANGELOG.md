# Changelog

All notable changes to this project will be documented in this file.

### Bug Fixes

- Add shared/auth to Dockerfile for OIDC dependency (d5b8d12)
- Add missing require for shared/auth/oidc module (3c70e3d)
- Remove obsolete Forgejo connector code and unused functions (ee7219c)
- Address PR review comments (9ce3f4f)
- Derive event type from real webhook headers; add CI for webhook-receiver (3057fd6)
- Remove mem0-specific code from ainsel — it's a generic MCP server (244c921)
- Clean up stale go.sum entries and fix operator tests (6dfbd1f)

### Documentation

- Write README (27a0e05)
- Document new read tools (45f0976)
- Document persona tools (3caf71a)
- Add AInsel MCP server guide and wire into sidebar (819f68a)

### Features

- Scaffold shared OIDC middleware module (f2a61a4)
- Replace static-token middleware with OIDC JWT validation (1efd552)
- Add delete_agent_image tool and tools param on update (ba4491e)
- New generic inbound webhook service (HMAC verify → NATS publish) (32e8ac5)
- Add chat MCP sidecar service (e054d54)

### Refactoring

- Remove all NATS dependencies from hub, MCP, and shared API (d1a065a)

### Merge

- Resolve conflict with main (578a38e)

### Revert

- Drop get_persona tool and ConfigMap RBAC, pending #61 (fe95fb7)

