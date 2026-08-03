# Ainsel K8s Event Source Gateway Operator Architecture

## Overview

The ainsel-k8s-event-source-gateway-operator manages the lifecycle of WebhookConnector resources. It automates the deployment of ainsel-event-source-gateway-forgejo instances and the registration of webhooks with Forgejo.

## Controller Structure

```mermaid
graph TD
    subgraph "ainsel-k8s-event-source-gateway-operator"
        MGR[Controller Manager]
        FC[WebhookConnector Controller]
    end

    MGR --> FC

    subgraph "Managed Resources"
        DEP[Deployment<br/>ainsel-event-source-gateway-forgejo]
        SVC[Service]
        WH[Forgejo Webhook]
    end

    FC -->|watches| CRD[WebhookConnector CRD]
    FC -->|manages| DEP
    FC -->|manages| SVC
    FC -->|API call| WH
```

## Reconciliation Flow

```mermaid
sequenceDiagram
    participant K as Kubernetes API
    participant CC as Connector Controller
    participant D as Deployment
    participant S as Service
    participant F as Forgejo API

    K->>CC: WebhookConnector created/updated
    CC->>CC: Read credentials from Secret
    CC->>CC: Read webhook secret from Secret
    CC->>D: Create/update Deployment
    Note over CC,D: Sets WEBHOOK_SECRET, NATS_URL,<br/>CONNECTOR_NAME, BOT_USERNAMES
    CC->>S: Create/update Service
    CC->>F: Register/update webhook
    Note over CC,F: POST /api/v1/repos/{owner}/{repo}/hooks
    F-->>CC: webhook ID
    CC->>K: Update status (webhookId, conditions)
```

## Webhook Lifecycle

```mermaid
flowchart TD
    A[WebhookConnector Created] --> B{Webhook exists?}
    B -->|No| C[Create webhook via Forgejo API]
    B -->|Yes| D[Update webhook config]
    C --> E[Store webhook ID in status]
    D --> E

    F[WebhookConnector Deleted] --> G[Delete webhook via Forgejo API]
    G --> H[Remove finalizer]

    I[WebhookConnector Updated] --> J{Config changed?}
    J -->|Yes| D
    J -->|No| K[No-op]
```

## CRD Fields

### WebhookConnectorSpec

| Field | Type | Description |
|-------|------|-------------|
| `url` | string | Internal Forgejo URL (for API calls from within cluster) |
| `externalUrl` | string | Public-facing Forgejo URL |
| `webhookEndpoint` | string | URL Forgejo sends webhooks to |
| `credentials` | SecretKeyRef | Admin API token for webhook registration |
| `webhookSecret` | SecretKeyRef | HMAC secret for webhook validation |
| `events` | []string | Forgejo event types to subscribe to |
| `image` | ConnectorImage | Container image for forgejo-connector |
| `resources` | ResourceRequirements | CPU/memory for the connector pod |

### Status Conditions

| Condition | Meaning |
|-----------|---------|
| `Ready` | Overall readiness of the connector |
| `WebhookRegistered` | Webhook successfully registered with Forgejo |
