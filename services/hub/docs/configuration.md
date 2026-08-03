# Configuration

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `NATS_URL` | `nats://localhost:4222` | NATS server connection URL |
| `HUB_PORT` | `8080` | Port for the REST API server |
| `HUB_METRICS_PORT` | `9090` | Port for Prometheus metrics |
| `HUB_NAMESPACE` | `ainsel` | Kubernetes namespace for CRD operations |
| `HUB_LOKI_URL` | _(unset)_ | Loki HTTP base URL. When unset, log endpoints return `503`. |
| `HUB_LOKI_NAMESPACE` | `ainsel` | Kubernetes namespace label used in the simple stream selector for `/api/observability/logs?app=...`. |
| `HUB_PROMETHEUS_URL` | _(unset)_ | Prometheus HTTP base URL. When unset, metrics endpoints return `503`. |

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
