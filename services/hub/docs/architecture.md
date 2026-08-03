# Ainsel Hub Backend Architecture

## Overview

The hub backend is the central control plane of Ainsel. It combines an event router, a trigger index, and a REST API into a single process.

## Component Structure

```mermaid
graph TD
    subgraph "ainsel-hub-backend process"
        MAIN[main.go]
        MAIN --> MGR[controller-runtime Manager<br/>cache + informers]
        MAIN --> RTR[Router<br/>internal/router/]
        MAIN --> API[API Server<br/>internal/api/]
        MAIN --> MET[Metrics<br/>internal/metrics/]
        MGR --> IDX[Trigger Index<br/>internal/trigger/]
        RTR --> IDX
    end

    MGR -->|watches| K8s[Kubernetes API]
    RTR -->|subscribe| NATS_E[(NATS EVENTS)]
    RTR -->|publish| NATS_A[(NATS AGENTS)]
    API -->|CRUD via client| K8s
```

## Event Routing Flow

```mermaid
sequenceDiagram
    participant C as Connector
    participant NE as NATS EVENTS
    participant R as Router
    participant TI as Trigger Index
    participant NA as NATS AGENTS
    participant AR as ainsel-ai-agent

    C->>NE: Publish event<br/>subject: events.forgejo.issue.opened
    NE->>R: Deliver event
    R->>TI: Match(event)
    TI->>TI: Check event type (wildcard)
    TI->>TI: Check ignoreBotEvents
    TI->>TI: Evaluate data filters
    TI-->>R: Matched triggers → agent names
    loop For each matched agent
        R->>NA: Publish to agent.<name>
    end
    NA->>AR: Deliver to ainsel-ai-agent
```

## Trigger Index

The trigger index is an in-memory data structure that stores all Trigger CRDs and provides fast event matching.

```mermaid
flowchart TD
    A[Event arrives] --> B{For each Trigger}
    B --> C{Event type matches?<br/>supports wildcards}
    C -->|No| B
    C -->|Yes| D{ignoreBotEvents?}
    D -->|Yes & actor.is_bot| B
    D -->|No or not bot| E{Filters match?}
    E -->|No| B
    E -->|Yes| F[Add to matched set]
    F --> B
    B -->|Done| G[Return matched agents]
```

### Sync with Kubernetes

The index is kept in sync using Kubernetes informers:
- **Add**: New Trigger CRD is added to the index
- **Update**: Trigger CRD is updated in the index
- **Delete**: Trigger CRD is removed from the index

## REST API

The API server provides a thin HTTP layer over the Kubernetes API, translating REST operations to CRD CRUD:

```mermaid
graph LR
    CLIENT[ainsel-hub-frontend / CLI] -->|HTTP| API[API Server]
    API -->|controller-runtime client| K8s[Kubernetes API]
    K8s --> AGENT[Agent CRDs]
    K8s --> TRIGGER[Trigger CRDs]
    K8s --> CONNECTOR[WebhookConnector CRDs]
```

### Handler Structure

| File | Endpoints |
|------|-----------|
| `handlers_agents.go` | `/api/v1/agents`, `/api/v1/agents/:name` |
| `handlers_connectors.go` | `/api/v1/connectors`, `/api/v1/connectors/:name` |
| `handlers_triggers.go` | `/api/v1/triggers`, `/api/v1/triggers/:name` |
| `server.go` | Route registration, health check, JSON helpers |
