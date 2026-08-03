# Configuration

## Operator Deployment

The ainsel-k8s-ai-agent-operator itself requires minimal configuration. It runs as a standard Kubebuilder operator with the following settings managed by the ainsel-chart:

| Helm Value | Description |
|------------|-------------|
| `agentOperator.image.repository` | Container image repository |
| `agentOperator.image.tag` | Container image tag |
| `agentOperator.resources` | CPU/memory requests and limits |

## RBAC

The operator requires cluster-level permissions to:
- Watch and manage `Agent` and `Trigger` CRDs
- Read `WebhookConnector` CRDs (for trigger validation)
- Create and manage Deployments, ConfigMaps, Services, and ServiceAccounts

These permissions are defined in the `config/rbac/` directory and applied via Kustomize or Helm.

## Operator CLI Flags

The operator binary accepts the following flags (in addition to standard kubebuilder flags):

| Flag | Default | Description |
|------|---------|-------------|
| `--nats-ack-wait` | `30m` | JetStream redelivery timeout for agent consumers. Gives a long-running agent task enough headroom before NATS redelivers. |
| `--nats-max-deliver` | `10` | Maximum delivery attempts before JetStream gives up on a message. |
| `--agent-grace-period` | `1800` | Pod `terminationGracePeriodSeconds` for agent deployments. Allows in-flight tasks to finish on SIGTERM. |

### Recommended values

| Workload profile | `--nats-ack-wait` | `--nats-max-deliver` | `--agent-grace-period` |
|-----------------|-------------------|---------------------|------------------------|
| Default (coding agents) | `30m` | `10` | `1800` |
| Short-lived tasks (chat) | `5m` | `3` | `300` |
| Long-running analysis | `1h` | `5` | `3600` |

## Operator Environment Variables

The operator pod itself reads the following environment variables to control what gets injected into agent deployments:

| Variable | Required | Description |
|----------|----------|-------------|
| `FORGEJO_URL` | Optional | Forgejo API URL. When set, propagated as a literal value to each agent's `FORGEJO_URL`. |
| `FORGEJO_TOKEN_SECRET_NAME` | Optional | Name of a Kubernetes Secret in the agent's namespace containing the Forgejo API token. When set, agents get `FORGEJO_TOKEN` via `valueFrom.secretKeyRef` (the operator never reads the token value). |
| `FORGEJO_TOKEN_SECRET_KEY` | Optional | Key inside `FORGEJO_TOKEN_SECRET_NAME`. Defaults to `token`. |
| `NATS_URL` | Optional | Default NATS client URL. Used when `spec.natsUrl` is not set on the Agent. Defaults to `nats://nats.platform.svc.cluster.local:4222`. |
| `NATS_MONITORING_URL` | Optional | NATS HTTP monitoring endpoint queried by the KEDA NATS JetStream scaler. Defaults to `http://nats.platform.svc.cluster.local:8222`. Only used when an Agent has `spec.scaling.maxReplicas` set. |
| `NATS_ACCOUNT` | Optional | JetStream account name passed to the KEDA scaler. Defaults to `$G`. |

## Agent Environment Variables

When the Agent controller creates a Deployment for an Agent CR, it sets these environment variables on the ainsel-ai-agent container:

| Variable | Source | Description |
|----------|--------|-------------|
| `AGENT_NAME` | `metadata.name` | Agent name |
| `AGENT_PROVIDER` | `spec.runtime.provider` | LLM provider |
| `CLAUDE_MODEL` / `MISTRAL_MODEL` | `spec.llm.model` | LLM model |
| `NATS_URL` | Hub config | NATS connection URL |
| `NATS_STREAM` | Constant | `AGENTS` |
| `NATS_CONSUMER` | `metadata.name` | JetStream durable consumer name. The agent runtime MUST bind to this consumer; KEDA references the same name to read pending counts. |
| `NATS_MAX_ACK_PENDING` | Constant | `1`. The agent runtime MUST configure its consumer with `max_ack_pending=1` so each pod processes exactly one event at a time (true parallel processing: 3 replicas = 3 concurrent events). |
| `AGENT_EVENT_SUBJECTS` | Derived | `agent.<name>` |
| `AGENT_PERSONA_PATH` | Mount path | Path to persona file |
| `FORGEJO_URL` | Operator env (`FORGEJO_URL`) | Forgejo API URL |
| `FORGEJO_TOKEN` | `secretKeyRef` (configured via `FORGEJO_TOKEN_SECRET_NAME` / `FORGEJO_TOKEN_SECRET_KEY` on the operator) | Forgejo API token |
| `HUB_ENABLED` | Constant: `true` | Enables publishing of task lifecycle events to the hub backend |

## Autoscaling (KEDA)

When an Agent declares `spec.scaling.maxReplicas`, the operator additionally creates a
[KEDA](https://keda.sh) `ScaledObject` in the agent's namespace targeting the agent's Deployment.
The ScaledObject is owned by the Agent CR and is garbage-collected with it.

### Mapping

| Agent field | KEDA field |
|-------------|------------|
| `spec.scaling.minReplicas` (default `0`) | `spec.minReplicaCount` |
| `spec.scaling.maxReplicas` (required) | `spec.maxReplicaCount` |
| `spec.scaling.cooldownPeriod` (default `300`) | `spec.cooldownPeriod` |
| `spec.scaling.lagThreshold` (default `1`) | `spec.triggers[0].metadata.lagThreshold` |

### Trigger

A single `nats-jetstream` trigger is configured with:

- `natsServerMonitoringEndpoint` from the operator's `NATS_MONITORING_URL` env var
- `account` from the operator's `NATS_ACCOUNT` env var (default `$G`)
- `stream`: `AGENTS`
- `consumer`: the agent's name (also injected as `NATS_CONSUMER` on the agent pod)

### Prerequisites

KEDA must be installed in the cluster (CRDs and controller). If the
`scaledobjects.keda.sh` CRD is missing the operator logs a notice and skips the
ScaledObject reconcile — the Deployment is still created with `spec.scaling.minReplicas`
replicas, but no autoscaling will occur.

### Behavior

- **Scale-up** kicks in as soon as the agent's NATS consumer reports more pending
  messages than `lagThreshold` (default 1: scale on the first waiting event).
- **Scale-to-zero** is supported when `minReplicas: 0` and KEDA is installed; vanilla
  Kubernetes HPAs cannot reach 0 replicas.
- **Scale-down** is delayed by `cooldownPeriod` after the queue drains.
- Once a ScaledObject exists, the operator stops mutating `Deployment.spec.replicas`
  on subsequent reconciles so it does not fight KEDA.
- Removing `spec.scaling.maxReplicas` deletes the ScaledObject and the operator
  resumes managing replicas directly (using `spec.scaling.minReplicas` or `1`).
