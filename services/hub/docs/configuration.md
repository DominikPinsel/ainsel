# Configuration

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NATS_URL` | `nats://localhost:4222` | NATS server connection URL |
| `HUB_DB_URL` | _required_ | Postgres DSN. The hub refuses to start without it: it is the event queue, the log and conversation store, and the source the metric panels fall back to when there is no Prometheus. |
| `HUB_PORT` | `8080` | Port for the REST API server |
| `HUB_METRICS_PORT` | `9090` | Port for Prometheus metrics |
| `HUB_NAMESPACE` | `ainsel` | Kubernetes namespace for CRD operations |
| `HUB_PROMETHEUS_URL` | _(unset)_ | Prometheus HTTP base URL. When unset the event metrics endpoints fall back to the hub's own records, and only the token endpoints and raw PromQL return `503`. |

## Helm Values

When deployed via the ainsel-chart:

```yaml
hub:
  image:
    repository: localhost:30500/ainsel/ainsel-hub-backend
    tag: latest
  port: 8080
  metricsPort: 9090
  nats:
    url: nats://nats.platform.svc.cluster.local:4222
  namespace: ainsel
  ingress:
    enabled: true
    host: ainsel.example.com
    path: /ainsel/api
    className: nginx
  resources:
    requests:
      cpu: 50m
      memory: 128Mi
    limits:
      cpu: 200m
      memory: 256Mi
```

## NATS Streams

The hub backend interacts with three NATS JetStream streams:

| Stream | Role | Subject |
|--------|------|---------|
| `EVENTS` | Consumer | `events.>` -- reads all connector events |
| `AGENTS` | Publisher | `agent.<name>` -- publishes matched events to agents |
| `HUB` | Consumer | `hub.>` -- reads agent completion signals |

## Kubernetes RBAC

The hub backend needs read access to:
- `Agent` CRDs (for API and trigger resolution)
- `Trigger` CRDs (for the trigger index)
- `WebhookConnector` CRDs (for API)

And write access for API mutations (create, update, delete).
