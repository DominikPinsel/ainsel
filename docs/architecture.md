# Ainsel Platform Architecture

> Looking for the product-level framing — who AInsel is for, what problem
> it solves, what administrators actually configure? See the root
> [`README.md`](../README.md) and the
> [administrator guide](administrator-guide.md). This document is the
> technical architecture reference.

AInsel is a Kubernetes-native platform that runs AI agents in response to events from code forges. A connector wraps webhook deliveries (raw body + headers) into an `Event` struct and POSTs it to the hub; the hub matches events against triggers and routes them to agents via the event queue; agents act on the forge (commenting, opening PRs, pushing code). This document captures the full data flow and the components involved.

The platform is a single Helm chart deployed into a single namespace. Every component listed below lives in this monorepo — paths in the diagrams point at the folder that owns each component.

## Overall System Architecture

The event infrastructure is a PostgreSQL-backed queue inside the hub's
PostgreSQL instance (migration `0014_create_event_queue`):

- The **`events`** table stores every ingested event, stamped with its
  connector (`routed_at` is `NULL` until the router processes it).
- The **`agent_tasks`** table stores one delivery per matched
  `(event, agent)` pair — the event fan-out.

```mermaid
graph TD
    FORGE[Forgejo Instance] -->|webhook POST| FC[services/webhook-receiver]

    subgraph "PostgreSQL event queue"
        E[(events table<br/>id, connector, headers, data,<br/>routed_at)]
        T[(agent_tasks table<br/>one row per matched agent)]
    end

    FC -->|"POST /api/internal/events"| HUB[services/hub]
    HUB -->|insert, routed_at = NULL| E
    E -->|"router polls unrouted (2s)"| HUB
    HUB -->|enqueue task per matched agent| T

    HUB -->|"cron emitter: synthetic event + direct task"| E
    HUB -->|"chat handler: user message event + direct task"| E

    T -->|"HTTP long-poll /next-task<br/>LISTEN/NOTIFY wakeup"| AR1[agent runtime<br/>code-reviewer]
    T -->|long-poll /next-task| AR2[agent runtime<br/>issue-triager]
    AR1 -->|ack / nack| T
    AR2 -->|ack / nack| T

    AR1 -->|API calls| FORGE
    AR2 -->|API calls| FORGE

    subgraph "Kubernetes Control Plane"
        AO[operators/agent]
        CO[operators/event-gateway]
        K8s[(Kubernetes API<br/>CRDs)]
    end

    AO -->|watches Agent + Trigger| K8s
    CO -->|watches WebhookConnector| K8s
    AO -->|manages Deployments| AR1
    AO -->|manages Deployments| AR2
    CO -->|manages Deployment| FC

    UI[frontend] -->|"REST API + WebSocket"| HUB
    HUB -->|CRUD| K8s

    subgraph "Vector Database"
        QD[(qdrant)]
    end

    AR1 -.->|store/recall| QD
    AR2 -.->|store/recall| QD
```

## Event Flow

This diagram shows the complete lifecycle of a single event from webhook to agent response.

```mermaid
sequenceDiagram
    participant F as Forgejo
    participant FC as services/webhook-receiver
    participant H as hub ingest API
    participant E as events table
    participant R as hub router (2s poll)
    participant TI as Trigger Index
    participant T as agent_tasks table
    participant AR as agent runtime
    participant LLM as Claude/Mistral API

    F->>FC: POST webhook (issues, action=opened)
    FC->>FC: Validate HMAC signature
    FC->>FC: Wrap raw body + headers in Event
    FC->>H: POST /api/internal/events
    H->>E: INSERT (routed_at = NULL)

    R->>E: FetchUnrouted (batch of 10)
    R->>TI: Match(event)
    TI->>TI: Check connectorRef == event.connector
    TI->>TI: Evaluate data filters (AND)
    TI-->>R: Matched: [code-reviewer, issue-triager]

    loop each matched agent (deduplicated)
        R->>R: Record invocation (running)
        R->>T: INSERT task (event_id, agent, trigger, invocation)
    end
    R->>E: MarkRouted(routed_at = now)
    R->>R: Broadcast activity entry + stats (WebSocket)

    AR->>T: Long-poll /api/internal/agents/{name}/next-task
    Note over T: Atomic claim:<br/>SELECT FOR UPDATE SKIP LOCKED
    AR->>AR: Build prompt (persona + skills + context)
    AR->>LLM: Send prompt

    loop Tool-use loop
        LLM-->>AR: tool_use(forgejo.comment, ...)
        AR->>AR: Execute tool subprocess
        AR->>LLM: Tool result
    end

    LLM-->>AR: Final response
    AR->>F: Post comment / create PR
    AR->>T: ACK task (status = completed)
    Note over T: hub completes the invocation<br/>on ack / nack
```

## Channels

A **channel** is the named stream an event lives in. Channels are not CRDs:
they are rows in the hub's Postgres, and every stored event records the
channel it was born in (`events.channel_id`).

| Kind | Provisioned from | Deletable | What the channel means |
|------|------------------|-----------|------------------------|
| `connector` | each `WebhookConnector` | no | where that connector's events arrive |
| `agent` | each `Agent` | no | that agent's inbox — everything in it is prompted to the agent |
| `custom` | created by a user | yes, while nothing is attached | a grouping of subscriptions that can be handed to another agent |

Identity is the channel **id**, never the name: a connector `forgejo` and an
agent `forgejo` are two distinct channels that happen to share a display
label. The reconciler (`services/hub/internal/channels/reconcile.go`) runs on
a ticker, provisions a channel per registry entry, flags the ones whose
entity is gone as orphaned (kept, so history stays readable), and stamps the
birth channel on events recorded before channels existed.

Two kinds of subscription move events between channels:

| Source | Owned by | Route |
|--------|----------|-------|
| `trigger` | the trigger registry | connector channel → agent inbox |
| `bridge` | the channel graph | any channel → any channel, at least one end custom |

Triggers stay the single source of truth for connector→agent routing; bridges
never copy them. Bridge edges cannot join two provisioned channels directly
(that pairing is a trigger), cannot point a channel at itself, and the graph
must stay acyclic — `POST /api/v1/channels/{id}/bridges` rejects a cycle
rather than letting events loop.

```mermaid
flowchart LR
    CF[forgejo<br/>connector channel] -->|trigger: on-issues| IB[review-bot<br/>agent inbox]
    CF -->|bridge| GRP[code review<br/>custom channel]
    GRP -->|bridge| INB[internal-bot<br/>agent inbox]
    CRON[cron tick] -->|born directly| IB
    CHAT[chat message] -->|born directly| IB
```

Scheduled ticks and chat messages have no connector: they are born directly
in the target agent's inbox channel. The synthetic `cron` / `chat` producer
labels are declared once in `shared/api` so the reconciler, the emitters and
the UI agree that they are not channels.

Both mechanisms run for every event. The router matches triggers first, then
walks the bridge graph from the event's birth channel and enqueues a task per
agent inbox reachable along it — skipping inboxes the trigger step already
delivered to. A transfer failure is logged and counted; it never aborts the
batch or stalls routing, because an event that partially arrived is worth
more than one stuck in redelivery.

## Cron Trigger Flow

A `CronTrigger` is a time-based source of events. Where the router turns
webhook events into agent tasks via trigger matching, the hub's cron emitter
inserts a synthetic event into the `events` table and enqueues the task for
the trigger's agent directly — no connector, no trigger match step. Cron
triggers live in the `cron_triggers` table and are synced into the emitter's
schedule by the hub's 30-second DB sync loop.

```mermaid
sequenceDiagram
    participant DB as PostgreSQL<br/>(cron_triggers table)
    participant CE as services/hub<br/>(cron emitter)
    participant E as events table
    participant T as agent_tasks table
    participant AR as agent runtime
    participant LLM as LLM API

    CE->>DB: Sync every 30s (upsert/delete schedule entries)
    loop every 30s tick
        CE->>CE: Schedule due
        CE->>E: INSERT synthetic event<br/>(connector = "cron", headers.type = "cron")
        CE->>E: MarkRouted immediately (bypasses router)
        CE->>CE: Record invocation (running)
        CE->>T: INSERT task for trigger's agentRef
        AR->>T: Long-poll claim
        AR->>AR: Render data.prompt verbatim (no forgejo template)
        AR->>LLM: Send prompt
        LLM-->>AR: Response
        AR->>T: ACK task
    end
```

The cron event carries `connector: "cron"` and a `data` payload of
`{cronTrigger, prompt}`. The agent runtime recognises this and renders the
prompt verbatim rather than wrapping it in the forgejo event template, so the
`prompt` field is the full user message the model receives.

## CRD Relationship Diagram

```mermaid
classDiagram
    class Agent {
        +string displayName
        +AgentForgejo forgejo
        +AgentRuntime runtime
        +AgentLLM llm
        +AgentPersona persona
        +[]AgentSkill skills
        +AgentScaling scaling
        +AgentMemory memory
        ---
        +AgentStatus status
    }

    class Trigger {
        +string agentRef
        +string connectorRef
        +[]Filter filters
        ---
        +TriggerStatus status
    }

    class WebhookConnector {
        +string url
        +string externalUrl
        +string webhookEndpoint
        +SecretKeyRef credentials
        +SecretKeyRef webhookSecret
        +[]string events
        +ConnectorImage image
        ---
        +WebhookConnectorStatus status
    }

    class Filter {
        +string field
        +string op
        +string value
        +[]string values
    }

    Trigger --> Agent : agentRef
    Trigger --> WebhookConnector : connectorRef
    Trigger --> Filter : filters[]

    class CronTrigger {
        +string agentRef
        +string schedule
        +string prompt
        +bool enabled
        ---
        +CronTriggerStatus status
    }

    CronTrigger --> Agent : agentRef
    Agent --> Deployment : creates
    Agent --> ConfigMap : creates (persona)
    WebhookConnector --> Deployment : creates
```

## Deployment Diagram

```mermaid
graph TD
    subgraph "Kubernetes Cluster"
        subgraph "ainsel namespace"
            AO[k8s-ai-agent-operator<br/>Deployment, 1 replica]
            CO[k8s-event-source-gateway-operator<br/>Deployment, 1 replica]
            HUB[hub-backend<br/>Deployment + Service]
            UI_POD[hub-frontend<br/>Deployment + Service<br/>nginx]
            QD[qdrant<br/>StatefulSet + PVC]
            PG[PostgreSQL<br/>StatefulSet + PVC]

            subgraph "Dynamic (created by operators)"
                FC_POD[connector-<id><br/>webhook-receiver<br/>Deployment + Service, one per WebhookConnector]
                AR1_POD[agent: code-reviewer<br/>Deployment]
                AR2_POD[agent: issue-triager<br/>Deployment]
            end
        end

        subgraph "Ingress Layer"
            ING[nginx Ingress]
        end

        subgraph "CRDs (cluster-scoped)"
            CRD1[Agent CRD]
            CRD2[AgentImage CRD]
            CRD3[WebhookConnector CRD]
        end
    end

    ING -->|/ainsel/api| HUB
    ING -->|/ainsel| UI_POD
    HUB --> PG
    FC_POD -->|POST /api/internal/events| HUB
    AR1_POD -->|long-poll /next-task| HUB
    AR2_POD -->|long-poll /next-task| HUB
```

## PostgreSQL Event Queue

The queue lives in the hub's PostgreSQL database (migration
`0014_create_event_queue`). Two tables carry the event flow; the hub also
tracks run state in the `invocations` table.

```mermaid
erDiagram
    EVENTS ||--o{ AGENT_TASKS : "fan-out (one row per matched agent)"
    EVENTS {
        text id PK
        text connector
        jsonb headers
        jsonb data
        text raw
        timestamptz received_at
        timestamptz routed_at
    }
    AGENT_TASKS {
        bigint id PK
        text event_id FK
        text agent_name
        text trigger_name
        text invocation_id
        jsonb headers
        jsonb payload
        text status "pending | claimed | completed | failed"
        int attempts "max_attempts = 10"
    }
```

Key properties:

- **Connector-stamped ingestion.** Every event is inserted with the
  connector that produced it. Webhook events carry the `WebhookConnector`
  CR name — the connector's `c-…` id; the cron emitter and the chat handler
  insert synthetic events with the pseudo-connectors `"cron"` and `"chat"`.
- **Fan-out by trigger match.** The router matches unrouted events against
  the trigger index (`trigger.connectorRef == event.connector` plus data
  filters) and inserts one `agent_tasks` row per matched agent. The
  `UNIQUE (event_id, agent_name)` constraint guarantees an event is
  delivered to each agent at most once.
- **Unmatched events stay in `events`** only — visible in the
  observability UI as `status: unmatched`.
- **Long-poll consumption.** Agent runtimes claim tasks via
  `GET /api/internal/agents/{name}/next-task` (SELECT FOR UPDATE SKIP
  LOCKED) with `pg_notify('agent_tasks', …)` wake-up, then ack (completed)
  or nack (retry with backoff, failed after `max_attempts`).

### Derived Subject

There is no message broker any more, but events are still addressed by a
two-level **derived subject** `<connector>.<eventType>` — used by the
observability event filters and by trigger filters. The event type is
derived from the webhook headers: any header ending in `-Event` (e.g.
`X-Forgejo-Event`), falling back to a generic `type` header
(`trigger.CanonicalEventType`).

| Level | Meaning | Example |
|-------|---------|---------|
| 1 | Connector (channel) | `c-1a2b3c`, `cron`, `chat` |
| 2 | Event type | `push`, `issues`, `chat.message` |
| Pattern | `*` / `>` wildcards supported | `c-1a2b3c.*`, `*.push` |

## Component Interactions

| Component | Depends On | Produces | Consumes |
|-----------|-----------|----------|----------|
| services/webhook-receiver | hub internal API | `events` rows (via ingest API) | Forgejo webhooks |
| services/hub | PostgreSQL, Kubernetes API | `agent_tasks` rows, invocations, WebSocket activity | unrouted `events` rows |
| services/hub (cron emitter) | PostgreSQL | synthetic `events` + `agent_tasks` rows (scheduled) | `cron_triggers` table |
| services/hub (chat handler) | PostgreSQL | synthetic `events` + `agent_tasks` rows | chat sessions |
| operators/agent | Kubernetes API | Deployments, ConfigMaps | Agent CRDs |
| operators/event-gateway | Kubernetes API, Forgejo API | Deployments, Services, Webhooks | WebhookConnector CRDs |
| agent runtime (pi runner) | hub internal API | acks/nacks, task logs | `agent_tasks` rows (long-poll) |
| frontend | services/hub REST API + WebSocket | User actions | Hub API responses, activity events |