# Chart

Helm chart that deploys the entire AInsel platform into a single namespace.

## What it deploys

- The two operators ([`operators/agent`](../operators/agent/),
  [`operators/event-gateway`](../operators/event-gateway/)).
- The hub backend ([`services/hub`](../services/hub/)) with its `Ingress`.
- The frontend console ([`frontend`](../frontend/)) with its `Ingress`.
- PostgreSQL event queue and Postgres (in-cluster, optional).
- Vector database infrastructure (Qdrant).
- The MCP server ([`services/mcp`](../services/mcp/), opt-in).
- All CRDs (`Agent`, `AgentImage`, `WebhookConnector`).

## Usage

For the full deployment guide, see [`docs/deployment.md`](../docs/deployment.md).

Lint and render locally:

```bash
helm lint .
helm template . -f your-values.yaml
```

Install:

```bash
helm install ainsel . -n ainsel --create-namespace -f your-values.yaml
```

Upgrade:

```bash
helm upgrade ainsel . -n ainsel -f your-values.yaml
```

Single-namespace deployment with default values:

```bash
helm install ainsel . --namespace ainsel --create-namespace
```

The chart does not render the target `Namespace` object unless
`createNamespace: true` is set — pre-creating the namespace yourself and then
installing also works.

See `values-example.yaml` for a starting point for your own
environment-specific overlay.

## Resource Sizing

The default values in `values.yaml` are the **small** tier — sized for a
single-node dev cluster with one or two users. For production deployments,
use one of the overlay files:

```bash
# Medium tier — ~10 active agents (estimated, not yet load-tested)
helm install ainsel . -n ainsel -f chart/values-medium.yaml

# Large tier — ~100 active agents (estimated, not yet load-tested)
helm install ainsel . -n ainsel -f chart/values-large.yaml
```

| Component | Small (default) | Medium (~10 agents) | Large (~100 agents) |
|---|---|---|---|
| agentOperator | 50m/64Mi → 200m/256Mi | 100m/128Mi → 400m/512Mi | 300m/512Mi → 1/2Gi |
| connectorOperator | 50m/64Mi → 200m/256Mi | 100m/128Mi → 400m/512Mi | 300m/512Mi → 1/2Gi |
| hub | 50m/128Mi → 200m/256Mi | 200m/512Mi → 500m/1Gi | 500m/1Gi → 2/4Gi |
| ui | 10m/16Mi → 100m/64Mi | 50m/64Mi → 100m/128Mi | 100m/128Mi → 200m/256Mi |
| qdrant | 50m/256Mi → 500m/512Mi | 200m/512Mi → 1/2Gi | 500m/1Gi → 2/4Gi |
| postgres | 50m/256Mi → 500m/512Mi | 200m/512Mi → 1/2Gi | 500m/1Gi → 2/4Gi |
| mcp | 50m/64Mi → 200m/128Mi | 100m/128Mi → 400m/256Mi | 300m/256Mi → 1/1Gi |

Values are shown as `requests → limits` (CPU/memory). Medium and large
tiers are **estimates** — measure under your actual workload and adjust.

StatefulSet storage sizes (`qdrant.storage`, `postgres.storage`)
are orthogonal to CPU/memory tiers. Scale storage independently based on
data volume and retention requirements.

### Fresh-install behavior and limitations

- **Namespace:** keep `createNamespace: false` (default) and use
  `--create-namespace` or a pre-created namespace. Rendering the namespace
  inside the chart fails ownership validation on Helm 3.x when the namespace
  already exists.
- **Auth:** the hub backend refuses to start without OIDC unless
  `auth.allowInsecureNoAuth: true` is set (local testing only). The frontend
  always requires a complete OIDC configuration — the UI is not usable in
  no-auth mode.
- **One event gateway operator per cluster:** released operator images
  (`0.1.0`) watch connector resources across all namespaces even though the
  chart sets `POD_NAMESPACE`. Running two AInsel releases in the same
  cluster makes their gateway operators fight over connector deployments.
  Namespace-scoped watching lands in a later operator release; until then
  keep a single release per cluster or set `connectorOperator.enabled: false`
  on additional releases.
- **TLS with cert-manager:** when `networkPolicy.enabled` is true, the
  default-deny policy still allows ingress-controller traffic to cert-manager
  HTTP-01 solver pods (`networkPolicy.allowAcmeSolver`, default true), so
  http01 issuance works out of the box.

### Prometheus Alert Rules (bonus)

To install CPU throttling and OOM alert rules, enable the `prometheusRules`
key in your values file:

```yaml
observability:
  prometheusRules:
    enabled: true
```

This requires the `monitoring.coreos.com/v1` PrometheusRule CRD (installed
by the Prometheus Operator). The rules are disabled by default to avoid
install failures on clusters without the Prometheus Operator.

## Notable values

| Key | Default | Purpose |
|---|---|----|
| `namespace` | `ainsel` | Target namespace for all platform resources |
| `createNamespace` | `false` | Render a `Namespace` object; prefer `--create-namespace` |
| `hub.image` / `hub.port` | tag set in chart / `8080` | Hub backend image and REST API port |
| `hub.namespace` | `""` (falls back to `namespace`) | Namespace the hub operates on (agents, backfill) |
| `hub.replicas` | `1` | Hub backend replicas |
| `hub.ingress` | enabled (nginx) | Hub `Ingress` host, path, and TLS annotations |
| `ui.enabled` / `ui.image` | `true` / chart tag | Frontend console toggle and image |
| `ui.basePath` | `/ainsel` | URL base path the frontend is served under |
| `agentOperator.enabled` / `replicas` | `true` / `1` | Toggle and scale the agent operator |
| `agentOperator.image` / `connectorOperator.image` | tags set in chart | Operator images |
| `connectorOperator.enabled` / `replicas` | `true` / `1` | Toggle and scale the event gateway operator |
| `connectorOperator.clusterWideRbac` | `true` | ClusterRole/CRB required by released operator images |
| `mcp.enabled` | `false` | Enable the AInsel MCP server (`services/mcp`) |
| `qdrant` / `postgres` | enabled | In-cluster vector DB + relational DB |
| `auth.allowInsecureNoAuth` | `false` | Run hub without auth middleware (local testing only) |
| `networkPolicy.allowAcmeSolver` | `true` | Let ingress reach cert-manager HTTP-01 solver pods |
| `observability.prometheus.url` / `observability.loki.url` | `""` | External Prometheus / Loki for the hub's observability API |
| `observability.prometheusRules.enabled` | `false` | Install CPU throttling / OOM PrometheusRule (requires Prometheus Operator) |

Run `grep -E '^[a-zA-Z]' values.yaml | grep -v '^#'` for the full
top-level key list; comments in `values.yaml` document every nested key.

## CRDs

The chart installs the CRDs in `crds/`. CRD definitions are generated by
Kubebuilder from `operators/*/api/v1alpha1/` and copied here.

## Reference

- [Deployment guide](../docs/deployment.md)
- [CRD reference](../docs/crd-reference.md)
- [Configuration reference](docs/configuration.md)