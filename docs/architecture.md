# Ainsel Platform Architecture

> Looking for the product-level framing — who AInsel is for, what problem
> it solves, what administrators actually configure? See the root
> [`README.md`](../README.md) and the
> [administrator guide](administrator-guide.md). This document is the
> technical architecture reference.

AInsel is a Kubernetes-native platform that runs AI agents in response to events from code forges. A connector wraps webhook deliveries (raw body + headers) into an `Event` struct and POSTs it to the hub; the hub matches events against triggers and routes them to agents via the event queue; agents act on the forge (commenting, opening PRs, pushing code). This document captures the full data flow and the components involved.

The platform is a single Helm chart deployed into a single namespace. Every component listed below lives in this monoropo — paths in the diagrams point at the folder that owns each component.

## Overall System Architecture

```mermaid
graph TD
    FORGE[Forgejo Instance] -->|webhook POST| FC[services/webhook-receiver]

    subgraph "PostgreSQL event queue"
        NE[(EVENTS stream<br/>events.*)]
        NA[(AGENTS stream<br/>agent.*)]
        NH[(HUB stream<br/>hub.*)]
    end

    FC -->|publish| NE
    NE -->|subscribe| HUB[services/hub]
    HUB -->|publish matched| NA
    NA -->|subscribe| AR1[agent runtime<br/>code-reviewer]
    NA -->|subscribe| AR2[agent runtime<br/>issue-triager]
    AR1 -->|complete| NH
    AR2 -->|complete| NH
    NH -->|subscribe| HUB

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

    UI[frontend] -->|REST API| HUB
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
    participant NE as NATS EVENTS
    participant H as services/hub
    participant TI as Trigger Index
    participant NA as NATS AGENTS
    participant AR as agent runtime
    participant LLM as Claude/Mistral API

    F->>FC: POST webhook (issues, action=opened)
    FC->>FC: Validate HMAC signature
    FC->>FC: Wrap raw body + headers in Event
    FC->>NE: Publish events.forgejo

    NE->>H: Deliver event
    H->>TI: Match(event)
    TI->>TI: Check connector + derive type from headers
    TI->>TI: Check ignoreBotEvents
    TI->>TI: Evaluate data filters (AND)
    TI-->>H: Matched: [code-reviewer, issue-triager]

    H->>NA: Publish agent.code-reviewer
    H->>NA: Publish agent.issue-triager

    NA->>AR: Deliver to code-reviewer runner
    AR->>AR: Build prompt (persona + skills + context)
    AR->>LLM: Send prompt

    loop Tool-use loop
        LLM-->>AR: tool_use(forgejo.comment, ...)
        AR->>AR: Execute tool subprocess
        AR->>LLM: Tool result
    end

    LLM-->>AR: Final response
    AR->>F: Post comment / create PR
    AR->>NA: ACK event
    AR->>NE: Publish hub.invocation.completed
```

## Cron Trigger Flow

A `CronTrigger` is a time-based source of events. Where the event-gateway
turns webhooks into the EVENTS stream, the hub's cron emitter turns a cron
schedule into the AGENTS stream directly — no connector, no router match step.

```mermaid
sequenceDiagram
    participant K as Kubernetes API
    participant CE as services/hub<br/>(cron emitter)
    participant NA as NATS AGENTS
    participant AR as agent runtime
    participant LLM as LLM API

    K->>CE: CronTrigger watch (add/update/delete)
    CE->>CE: Parse schedule, register entry
    loop every scheduled minute
        CE->>CE: Schedule due
        CE->>CE: Record invocation (running)
        CE->>NA: Publish agent.<agentRef><br/>connector=cron, data.prompt
        NA->>AR: Deliver to agent replica
        AR->>AR: Render data.prompt verbatim (no forgejo template)
        AR->>LLM: Send prompt
        LLM-->>AR: Response
        AR->>NA: ACK + hub.invocation.completed
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
        +string eventType
        +bool ignoreBotEvents
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
            AO[ainsel-k8s-ai-agent-operator<br/>Deployment, 1 replica]
            CO[ainsel-k8s-event-source-gateway-operator<br/>Deployment, 1 replica]
            HUB[ainsel-hub-backend<br/>Deployment + Service]
            UI_POD[ainsel-hub-frontend<br/>Deployment + Service<br/>nginx]
            QD[qdrant<br/>StatefulSet + PVC]
            NATS_POD[NATS<br/>StatefulSet + PVC]

            subgraph "Dynamic (created by operators)"
                FC_POD[ainsel-event-source-gateway-forgejo<br/>Deployment + Service]
                AR1_POD[agent: code-reviewer<br/>Deployment]
                AR2_POD[agent: issue-triager<br/>Deployment]
            end
        end

        subgraph "Ingress Layer"
            ING[nginx Ingress]
        end

        subgraph "CRDs (cluster-scoped)"
            CRD1[Agent CRD]
            CRD2[Trigger CRD]
            CRD3[WebhookConnector CRD]
        end
    end

    ING -->|/ainsel/api| HUB
    ING -->|/ainsel| UI_POD
    HUB --> NATS_POD
    FC_POD --> NATS_POD
    AR1_POD --> NATS_POD
    AR2_POD --> NATS_POD
```

## NATS Streams and Subjects

```mermaid
graph LR
    subgraph "EVENTS stream"
        E1[events.forgejo]
    end

    subgraph "AGENTS stream"
        A1[agent.code-reviewer]
        A2[agent.issue-triager]
        A3[agent.devops-bot]
    end

    subgraph "HUB stream"
        H1[hub.invocation.completed]
    end

    FC[services/webhook-receiver] -->|publish| E1

    HUB[services/hub] -->|subscribe events.*| E1

    HUB -->|publish| A1
    HUB -->|publish| A2

    AR1[agent runtime] -->|subscribe agent.code-reviewer| A1
    AR2[agent runtime] -->|subscribe agent.issue-triager| A2

    AR1 -->|publish| H1
    AR2 -->|publish| H1
    HUB -->|subscribe hub.*| H1
```

### Subject Format

| Stream | Subject Pattern | Format | Example |
|--------|----------------|--------|---------|
| EVENTS | `events.<connector>` | Single level | `events.forgejo` |
| AGENTS | `agent.<agentName>` | Single level | `agent.code-reviewer` |
| HUB | `hub.invocation.completed` | Fixed | `hub.invocation.completed` |

## Component Interactions

| Component | Depends On | Produces | Consumes |
|-----------|-----------|----------|----------|
| services/webhook-receiver | NATS | EVENTS stream events | Forgejo webhooks |
| services/hub | Kubernetes API | AGENTS stream events | EVENTS stream, HUB stream |
| services/hub (cron emitter) | Kubernetes API | AGENTS stream events (scheduled) | CronTrigger CRDs |
| operators/agent | Kubernetes API | Deployments, ConfigMaps | Agent CRDs, Trigger CRDs |
| operators/event-gateway | Kubernetes API, Forgejo API | Deployments, Services, Webhooks | WebhookConnector CRDs |
| frontend | services/hub REST API | User actions | Hub API responses |