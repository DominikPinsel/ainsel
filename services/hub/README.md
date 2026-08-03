# Hub

Control plane for the AInsel agent platform. Consumes canonical events from
NATS, matches them against the trigger index, routes matched events to
agents over NATS subjects, and exposes a REST API for managing CRDs.

## Role in the platform

See [`docs/architecture.md`](../../docs/architecture.md) for the full data
flow.

The hub backend:

- Subscribes to the NATS `EVENTS` stream produced by
  [`services/webhook-receiver/`](../webhook-receiver/).
- Holds an in-memory index of `Trigger` CRDs, kept in sync via Kubernetes
  informers.
- Publishes matched events to the NATS `AGENTS` stream for
  [`operators/agent/`](../../operators/agent/) to dispatch.
- Exposes `/api/v1/*` for the [`frontend/`](../../frontend/) console.

## Local development

```bash
# Build
make build
# Or directly:
go build -o bin/hub ./cmd/hub

# Run (needs NATS reachable, a kubeconfig, and a Postgres DSN)
NATS_URL=nats://localhost:4222 \
HUB_DB_URL=postgres://ainselhub:pw@localhost:5432/ainselhub?sslmode=disable \
KUBECONFIG=~/.kube/config \
./bin/hub
```

Ports:

- `8080` — REST API (`HUB_PORT`)
- `9090` — Prometheus metrics (`HUB_METRICS_PORT`)

Required env vars: `HUB_DB_URL` (Postgres DSN — MCP server registry).
Optional: `HUB_LOKI_URL`, `HUB_PROMETHEUS_URL` for observability endpoints,
`HUB_NAMESPACE` (default `ainsel`).

## Testing

```bash
make test
# Or:
go test ./...
```

## Reference

- [REST API endpoints](../../docs/api-reference.md)
- [Event schema](../../docs/event-schema.md)
- [CRD specs (Agent, Trigger)](../../docs/crd-reference.md)
