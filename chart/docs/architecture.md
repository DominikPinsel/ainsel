# Ainsel Chart Architecture

## Overview

The ainsel-chart is a Helm chart that deploys all Ainsel components into a single Kubernetes namespace. It creates the CRDs, operators, hub, UI, memory infrastructure, and optional bootstrap resources.

## Deployment Topology

```mermaid
graph TD
    subgraph "Kubernetes Cluster"
        subgraph "ainsel namespace"
            AO[ainsel-k8s-ai-agent-operator<br/>1 replica]
            CO[ainsel-k8s-event-source-gateway-operator<br/>1 replica]
            HUB[ainsel-hub-backend<br/>1 replica]
            UI[ainsel-hub-frontend<br/>1 replica]
            QD[qdrant<br/>1 replica<br/>5Gi PVC]
        end

        subgraph "platform namespace"
            NATS[NATS<br/>external]
        end

        subgraph "Ingress"
            ING[nginx Ingress Controller]
        end
    end

    ING -->|/ainsel/api| HUB
    ING -->|/ainsel| UI
    HUB --> NATS
    AO -.->|watches CRDs| HUB
    CO -.->|watches CRDs| HUB
```

## Template Structure

```mermaid
graph LR
    subgraph templates/
        NS[namespace.yaml]
        subgraph agent-operator/
            AO_D[deployment.yaml]
            AO_R[role.yaml]
            AO_RB[rolebinding.yaml]
            AO_SA[serviceaccount.yaml]
        end
        subgraph connector-operator/
            CO_D[deployment.yaml]
            CO_R[role.yaml]
            CO_RB[rolebinding.yaml]
            CO_SA[serviceaccount.yaml]
        end
        subgraph hub/
            H_D[deployment.yaml]
            H_S[service.yaml]
            H_I[ingress.yaml]
            H_R[role.yaml]
            H_RB[rolebinding.yaml]
            H_SA[serviceaccount.yaml]
        end
        subgraph ui/
            U_D[deployment.yaml]
            U_S[service.yaml]
        end
    end

    subgraph crds/
        CRD_A[agent.yaml]
        CRD_AI[agentimage.yaml]
        CRD_F[webhookconnector.yaml]
    end
```

## CRDs

The chart includes three CRDs installed in the `crds/` directory (applied before templates):

| CRD | File | API Group |
|-----|------|-----------|
| Agent | `crds/agent.yaml` | `ainsel.dev/v1alpha1` |
| AgentImage | `crds/agentimage.yaml` | `ainsel.dev/v1alpha1` |
| WebhookConnector | `crds/webhookconnector.yaml` | `ainsel.dev/v1alpha1` |
