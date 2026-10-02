# CRD Reference

## Agent

**API Group:** `ainsel.dev`
**Version:** `v1alpha1`
**Kind:** `Agent`

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `displayName` | string | Yes | Human-readable agent name |
| `description` | string | No | Agent description |
| `forgejo.username` | string | Yes | Forgejo username for the agent |
| `forgejo.email` | string | Yes | Forgejo email for the agent |
| `runtime.image` | string | Yes | Container image for ainsel-ai-agent |
| `runtime.imagePullPolicy` | string | No | Image pull policy (default: IfNotPresent) |
| `runtime.provider` | string | Yes | LLM provider (`claude` or `mistral`) |
| `runtime.resources` | ResourceRequirements | No | CPU/memory requests and limits |
| `llm.model` | string | Yes | LLM model name |
| `llm.maxTurns` | int | No | Maximum tool-use loop turns |
| `llm.temperature` | float64 | No | LLM temperature |
| `llm.vision` | bool | No | Model accepts image input (pi attaches screenshots/images only when true). Default false |
| `persona.inline` | string | No | Inline persona text |
| `persona.configMapRef.name` | string | No | ConfigMap containing persona |
| `persona.configMapRef.key` | string | No | Key in ConfigMap |
| `skills` | []AgentSkill | No | Additional skill ConfigMaps |
| `scaling.minReplicas` | int32 | No | Floor for queue-driven scaling, and the opt-in: unset means pin exactly `replicas`. `0` lets the agent go dormant. Must not exceed `replicas`. |
| `scaling.replicas` | int32 | No | Pod count in static mode, ceiling in queue mode. Default `1`. |
| `memory.enabled` | bool | No | Enable shared memory |
| `memory.provider` | string | No | Memory provider |

### Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | Standard Kubernetes conditions |
| `replicas` | int32 | Current ready replica count |
| `lastInvocation` | Time | Last time work was handed to the agent; the idle clock scale-down measures against |
| `pendingTasks` | int32 | Tasks queued for this agent. Written by the hub, not the operator. |
| `activeTasks` | int32 | Tasks claimed but not finished. Written by the hub. |
| `queueObservedAt` | Time | When the hub measured those counts. Past `--queue-signal-ttl` the operator holds its pod count instead of scaling down. |
| `scaling` | AgentScalingStatus | What the operator is doing with the pods: `mode` (`static`/`queue`), `desired`, `reason` and a one-line `message` |
| `observedGeneration` | int64 | Last observed spec generation |

### Example

```yaml
apiVersion: ainsel.dev/v1alpha1
kind: Agent
metadata:
  name: code-reviewer
  namespace: ainsel
spec:
  displayName: "Code Reviewer"
  description: "Reviews pull requests for code quality"
  forgejo:
    username: code-reviewer
    email: code-reviewer@ainsel.dev
  runtime:
    image: localhost:30500/ainsel/ainsel-ai-agent:latest
    provider: claude
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 500m
        memory: 512Mi
  llm:
    model: claude-sonnet-4-20250514
    maxTurns: 10
    temperature: 0.3
  persona:
    configMapRef:
      name: code-reviewer-persona
      key: CLAUDE.md
  scaling:
    replicas: 3
    minReplicas: 0
  memory:
    enabled: true
    provider: example
```

---

## Trigger

**API Group:** `ainsel.dev`
**Version:** `v1alpha1`
**Kind:** `Trigger`

### Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `agentRef` | string | Yes | Name of the Agent to invoke |
| `connectorRef` | string | Yes | Name of the WebhookConnector source |
| `eventType` | string | Yes | Event type pattern (supports `*` wildcard) |
| `ignoreBotEvents` | bool | No | Skip events from bot actors (default: `true`) |
| `filters` | []Filter | No | Additional data payload filters (AND logic) |

### Filter Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `field` | string | Yes | Dotted path into event data |
| `op` | string | Yes | Operator: `eq`, `neq`, `prefix`, `suffix`, `contains`, `in`, `regex` |
| `value` | string | No | Single comparison value |
| `values` | []string | No | List of values (for `in` operator) |

### Example

```yaml
apiVersion: ainsel.dev/v1alpha1
kind: Trigger
metadata:
  name: review-prs-on-main
  namespace: ainsel
spec:
  agentRef: code-reviewer
  connectorRef: forgejo
  eventType: "pull_request.opened"
  ignoreBotEvents: true
  filters:
    - field: "pull_request.base"
      op: eq
      value: "main"
```

### Wildcard Event Types

```yaml
# Match all issue events
eventType: "issue.*"

# Match all events
eventType: "*"

# Match specific event
eventType: "push"
```
