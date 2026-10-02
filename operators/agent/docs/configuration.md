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
| `--agent-grace-period` | `1800` | Pod `terminationGracePeriodSeconds` for agent deployments. Allows in-flight tasks to finish on SIGTERM. |
| `--agent-scale-down-window` | `0` → `2m` | How long an agent with `minReplicas` set must stay free of queued and in-flight work before its last pod is removed. `0` leaves the controller default. |
| `--queue-signal-ttl` | `0` → `5m` | How long the hub's published queue measurement may be trusted. Older than this, the operator holds the current pod count instead of scaling to zero. `0` leaves the controller default. |

The two scaling knobs are durations (`90s`, `2m`, `5m`), not bare numbers. Both default to `0`
so the real values live in one place, `internal/controller/scaling.go`; the Helm chart exposes
them as `agentOperator.agentScaleDownWindow` and `agentOperator.queueSignalTTL`.

### Recommended values

| Workload profile | `--agent-grace-period` | `--agent-scale-down-window` |
|-----------------|------------------------|-----------------------------|
| Default (coding agents) | `1800` | `2m` |
| Short-lived tasks (chat) | `300` | `5m` |
| Long-running analysis | `3600` | `1m` |

The window is a latency trade, not a resource one: a short window parks pods sooner and makes
the next request pay the cold boot. `queueSignalTTL` only needs to stay comfortably above the
hub's 60s publish sweep, so it rarely wants tuning.

## Operator Environment Variables

The operator pod itself reads the following environment variables to control what gets injected into agent deployments:

| Variable | Required | Description |
|----------|----------|-------------|
| `FORGEJO_URL` | Optional | Forgejo API URL. When set, propagated as a literal value to each agent's `FORGEJO_URL`. |
| `FORGEJO_TOKEN_SECRET_NAME` | Optional | Name of a Kubernetes Secret in the agent's namespace containing the Forgejo API token. When set, agents get `FORGEJO_TOKEN` via `valueFrom.secretKeyRef` (the operator never reads the token value). |
| `FORGEJO_TOKEN_SECRET_KEY` | Optional | Key inside `FORGEJO_TOKEN_SECRET_NAME`. Defaults to `token`. |
| `HUB_URL` | Optional | Hub API base URL, propagated to agent pods and MCP sidecars so the runtime can poll for tasks. |

## Agent Environment Variables

When the Agent controller creates a Deployment for an Agent CR, it sets these environment variables on the ainsel-ai-agent container:

| Variable | Source | Description |
|----------|--------|-------------|
| `AGENT_NAME` | `metadata.name` | Agent name |
| `AGENT_PROVIDER` | `spec.runtime.provider` | LLM provider |
| `CLAUDE_MODEL` / `MISTRAL_MODEL` | `spec.llm.model` | LLM model |
| `HUB_URL` | Operator env (`HUB_URL`) | Hub API the runtime polls for tasks |
| `HUB_INTERNAL_VALIDATE_SECRET` | Operator env | `X-Internal-Token` used against the hub's internal endpoints. Platform-owned: declarations on the AgentImage are dropped in favour of this. |
| `AGENT_PERSONA_PATH` | Mount path | Path to persona file |
| `FORGEJO_URL` | Operator env (`FORGEJO_URL`) | Forgejo API URL |
| `FORGEJO_TOKEN` | `secretKeyRef` (configured via `FORGEJO_TOKEN_SECRET_NAME` / `FORGEJO_TOKEN_SECRET_KEY` on the operator) | Forgejo API token |
| `HUB_ENABLED` | Constant: `true` | Enables publishing of task lifecycle events to the hub backend |

## Scaling

The operator owns `Deployment.spec.replicas` directly. There is no HPA and no
KEDA `ScaledObject` in the picture: pod count is decided from the queue depth the
hub publishes onto the Agent's own status, so the same component that owns the
task store decides when an agent has work and when it does not.

### Modes

| `spec.scaling` | Mode | Pod count |
|----------------|------|-----------|
| `minReplicas` unset | `static` | Exactly `replicas` (default `1`), regardless of queue depth. |
| `minReplicas` set | `queue` | Between `minReplicas` and `replicas`, following the queue. |

Setting `minReplicas` is itself the opt-in. An agent that does not set it is never
compared against the queue, so the feature cannot change the behaviour of anything
that was configured before it existed. `0` is the value that lets an agent go
dormant between tasks; a floor of `1` keeps a warm pod and only sheds burst
capacity.

### The decision

In order, as `resolveReplicas` applies it:

1. **Static** — no `minReplicas`: pin `replicas`, consult nothing.
2. **Disabled** — `replicas: 0`: an explicit zero is a user saying "none", and
   queued work does not overrule it.
3. **Stale or missing signal** — older than `--queue-signal-ttl`, or never
   published: hold the pods that are running, clamped into
   [`minReplicas`, `replicas`], and look again in 15s. Absence of a signal is not
   evidence of an empty queue; a hub that stopped publishing looks exactly like a
   queue that drained, and only one of those is permission to sleep.
4. **Work waiting or in flight** — one pod per task, bounded by the ceiling. A
   runtime claims exactly one task at a time, so queue depth is also the
   concurrency need; claimed tasks count, because losing a pod does not lose the
   task but parks it until the reaper gives up on the claim.
5. **Quiet, not quiet long enough** — shed burst capacity down to one pod and keep
   it for the rest of `--agent-scale-down-window`.
6. **Quiet past the window** — drop to the floor, which is zero for a dormant
   agent.

Quiet is measured from `status.lastInvocation`, which only moves when work
arrives. An agent the hub is reporting on but has never been handed anything skips
the grace window and sits at its floor (`Dormant`): there is no quiet period to
wait out if nothing has ever been queued. A brand-new agent is created at its
floor for the same reason it would be held without a signal — the operator starts
from the Deployment it is about to write, and on creation that count is zero.

### The signal

The hub's `queuesignal` publisher owns `status.pendingTasks`,
`status.activeTasks`, `status.queueObservedAt` and `status.lastInvocation`; the
operator owns everything else on status, and neither writes the other's fields.

| Knob | Default | Effect |
|------|---------|--------|
| hub publish tick | `250ms` | How soon an enqueued task becomes visible to the operator. |
| hub debounce | `2s` | Minimum gap between non-edge updates for one agent. Queue emptying or filling bypasses it, because that is the case a human is waiting on. |
| hub sweep | `60s` | Heartbeat. Re-measures and republishes every opted-in agent whether or not anything changed, which is what keeps a live hub's "this queue is empty" distinguishable from "nobody has looked". |
| `--queue-signal-ttl` | `5m` | How long the operator trusts a measurement it did not write. Must stay comfortably above the 60s sweep. |
| `--agent-scale-down-window` | `2m` | How long an agent must be quiet before its last pod goes. |

The operator reads the live decision back into `status.scaling`, so "asleep, waiting
for work" is distinguishable from "broken": `mode`, `desired`, and a `reason` of
`Static`, `QueueDepth`, `Dormant`, `ScaledToZero`, `IdleGrace`, `QueueSignalStale` or
`Disabled`.

```console
$ kubectl get agent.ainsel.dev a-reviewer -n ainsel-dev -o jsonpath='{.status.scaling}'
{"desired":0,"message":"quiet for 4m12s","mode":"queue","reason":"ScaledToZero"}
```

### Waking

Parked is not stopped. A task arrives at the hub, the publisher treats the empty
to non-empty transition as an edge and writes it on the next tick, the operator's
watch on Agent fires, and the Deployment scales up. The agent pays a pod creation
and boot, which is what the scale-down window exists to amortise.
