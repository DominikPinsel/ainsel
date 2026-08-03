# Configuration Reference

## Full values.yaml Reference

### Global

| Value | Type | Default | Description |
|-------|------|---------|-------------|
| `namespace` | string | `ainsel` | Kubernetes namespace for all resources |

### Resource Sizing Tiers

The default resource values in `values.yaml` are the **small** tier (single-node
dev cluster). Overlay files are provided for larger deployments:

| Overlay | Target | Usage |
|---|---|---|
| `values.yaml` (defaults) | 1–2 users, dev | `helm install ainsel . -n ainsel` |
| `values-medium.yaml` | ~10 active agents | `helm install ainsel . -n ainsel -f chart/values-medium.yaml` |
| `values-large.yaml` | ~100 active agents | `helm install ainsel . -n ainsel -f chart/values-large.yaml` |

Per-component resource requests/limits by tier (CPU/memory):

| Component | Small (default) | Medium (~10 agents) | Large (~100 agents) |
|---|---|---|---|
| agentOperator | 50m/64Mi → 200m/256Mi | 100m/128Mi → 400m/512Mi | 300m/512Mi → 1/2Gi |
| connectorOperator | 50m/64Mi → 200m/256Mi | 100m/128Mi → 400m/512Mi | 300m/512Mi → 1/2Gi |
| hub | 50m/128Mi → 200m/256Mi | 200m/512Mi → 500m/1Gi | 500m/1Gi → 2/4Gi |
| ui | 10m/16Mi → 100m/64Mi | 50m/64Mi → 100m/128Mi | 100m/128Mi → 200m/256Mi |
| qdrant | 50m/256Mi → 500m/512Mi | 200m/512Mi → 1/2Gi | 500m/1Gi → 2/4Gi |
| postgres | 50m/256Mi → 500m/512Mi | 200m/512Mi → 1/2Gi | 500m/1Gi → 2/4Gi |
| mcp | 50m/64Mi → 200m/128Mi | 100m/128Mi → 400m/256Mi | 300m/256Mi → 1/1Gi |

Medium and large values are **estimates** — not yet load-tested. Measure under
your actual workload and adjust accordingly. StatefulSet storage sizes
(`qdrant.storage`, `postgres.storage`) are orthogonal to
CPU/memory tiers; scale them independently.

### Agent Operator

| Value | Type | Default | Description |
|-------|------|---------|-------------|
| `agentOperator.image.repository` | string | `localhost:30500/ainsel/ainsel-k8s-ai-agent-operator` | Image repository |
| `agentOperator.image.tag` | string | `latest` | Image tag |
| `agentOperator.resources.requests.cpu` | string | `50m` | CPU request |
| `agentOperator.resources.requests.memory` | string | `64Mi` | Memory request |
| `agentOperator.resources.limits.cpu` | string | `200m` | CPU limit |
| `agentOperator.resources.limits.memory` | string | `256Mi` | Memory limit |

### Connector Operator

| Value | Type | Default | Description |
|-------|------|---------|-------------|
| `connectorOperator.image.repository` | string | `localhost:30500/ainsel/ainsel-k8s-event-source-gateway-operator` | Image repository |
| `connectorOperator.image.tag` | string | `latest` | Image tag |
| `connectorOperator.resources` | object | same as ainsel-k8s-ai-agent-operator | CPU/memory |

### Hub

| Value | Type | Default | Description |
|-------|------|---------|-------------|
| `hub.image.repository` | string | `localhost:30500/ainsel/ainsel-hub-backend` | Image repository |
| `hub.image.tag` | string | `latest` | Image tag |
| `hub.port` | int | `8080` | API server port |
| `hub.metricsPort` | int | `9090` | Metrics port |
| `hub.namespace` | string | `ainsel` | CRD namespace |
| `hub.ingress.enabled` | bool | `true` | Enable ingress |
| `hub.ingress.host` | string | `ainsel.example.com` | Ingress hostname |
| `hub.ingress.path` | string | `/ainsel/api` | Ingress path |
| `hub.ingress.className` | string | `nginx` | Ingress class |
| `hub.ingress.annotations` | object | cert-manager, ssl, rewrite | Ingress annotations |
| `hub.resources` | object | 50m/128Mi - 200m/256Mi | CPU/memory |

#### Hub Environment Variables

These environment variables are read by the hub backend at startup. Pass them
via `hub.extraEnv` in your `values.yaml`.

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `TASK_CLAIM_TIMEOUT_SECONDS` | int | `1800` | Duration (in seconds) after which a `claimed` task is considered stale. A background reaper runs every 5m and resets stale claims to `pending` (or `failed` if max attempts reached) with a 30s retry delay. |

### UI

| Value | Type | Default | Description |
|-------|------|---------|-------------|
| `ui.enabled` | bool | `true` | Enable/disable UI deployment |
| `ui.image.repository` | string | `localhost:30500/ainsel/ainsel-hub-frontend` | Image repository |
| `ui.image.tag` | string | `latest` | Image tag |
| `ui.ingress.enabled` | bool | `true` | Enable ingress |
| `ui.ingress.host` | string | `ainsel.example.com` | Ingress hostname |
| `ui.ingress.path` | string | `/ainsel` | Ingress path |
| `ui.ingress.className` | string | `nginx` | Ingress class |
| `ui.resources` | object | 10m/16Mi - 100m/64Mi | CPU/memory |

### Qdrant (Vector Database)

| Value | Type | Default | Description |
|-------|------|---------|-------------|
| `qdrant.enabled` | bool | `true` | Enable/disable qdrant |
| `qdrant.image.repository` | string | `qdrant/qdrant` | Image |
| `qdrant.image.tag` | string | `latest` | Tag |
| `qdrant.storage` | string | `5Gi` | PVC storage size |
| `qdrant.resources` | object | 50m/256Mi - 500m/512Mi | CPU/memory |

### Observability

| Value | Type | Default | Description |
|-------|------|---------|-------------|
| `observability.prometheus.url` | string | `""` | External Prometheus URL for hub observability API |
| `observability.loki.url` | string | `""` | External Loki URL for hub observability API |
| `observability.serviceMonitor.enabled` | bool | `false` | Create ServiceMonitor resources |
| `observability.podMonitor.enabled` | bool | `false` | Create PodMonitor resources |
| `observability.prometheusRules.enabled` | bool | `false` | Install CPU throttling / OOM PrometheusRule (requires Prometheus Operator) |
