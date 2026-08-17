# Deployment Guide

## Prerequisites

- Kubernetes cluster (tested on k3s)
- Helm 3
- Container registry accessible from the cluster (Zot, Docker Hub, etc.)
- Forgejo instance
- kubectl with cluster access

**NATS:** The chart bundles a NATS instance by default (`nats.enabled: true` in `values.yaml`). You do **not** need to deploy NATS separately unless you want to use an external NATS server. To use an external server, set `nats.enabled: false` in your `values.yaml` and point `hub.nats.url` at your existing NATS endpoint.

## Step 1: Building Images

Images for every Ainsel component are built and pushed by CI on every merge
to `main` and on every release tag. See the CI build workflows for the build
configuration.

For local development against an out-of-cluster registry, build a single
component using the Makefile in its folder. Operator folders ship
`docker-build` / `docker-push` targets, for example:

```bash
make -C operators/agent docker-build docker-push IMG=<registry>/ainsel/agent-operator:dev
```

Other components keep their build commands per folder; check the relevant
`Makefile` or `Dockerfile` for the exact target.

## Generating Required Secrets

Before running `helm install`, generate the secrets that the chart expects.

### Webhook HMAC secret

The connector uses an HMAC secret to verify that webhook payloads originate from Forgejo. Generate a random value and store it as a Kubernetes secret:

```bash
kubectl create secret generic connector-webhook-secret \
  --from-literal=secret=$(openssl rand -hex 32) \
  -n <namespace>
```

Reference this secret name in your connector configuration or `values.yaml` as appropriate.

### Postgres secret (external database only)

By default the chart provisions its own Postgres instance and auto-generates a password. If you want to use an external database, set `postgres.enabled: false` and supply an existing secret:

```bash
kubectl create secret generic ainsel-hub-db \
  --from-literal=username=<db-user> \
  --from-literal=password=<db-password> \
  --from-literal=database=<db-name> \
  --from-literal=dsn=postgres://<db-user>:<db-password>@<host>:5432/<db-name> \
  -n <namespace>
```

Then set `postgres.auth.existingSecret: ainsel-hub-db` in your `values.yaml`.

### Image pull secret (private registry)

If your images are hosted in a private registry that requires authentication, create a pull secret and add it to the default service account (or reference it in your `values.yaml`):

```bash
kubectl create secret docker-registry regcred \
  --docker-server=<registry-host> \
  --docker-username=<username> \
  --docker-password=<password> \
  -n <namespace>
```

---

## Step 2: Create Secrets

```bash
kubectl create namespace ainsel

# Forgejo admin API token (for webhook registration and agent accounts)
kubectl create secret generic forgejo-admin-token -n ainsel \
  --from-literal=token=<your-forgejo-admin-api-token>

# Webhook HMAC secret (shared between Forgejo and the connector)
kubectl create secret generic forgejo-webhook-hmac -n ainsel \
  --from-literal=secret=<your-webhook-hmac-secret>

# LLM API key (for agent runtime pods — Ollama Cloud via pi).
# The operator looks for a secret named <agent-name>-ollama-key with key "api-key".
kubectl create secret generic code-reviewer-ollama-key -n ainsel \
  --from-literal=api-key=<your-ollama-cloud-api-key>
```

## Step 3: Configure values.yaml

The Helm chart lives at [`chart/`](../chart/) in this monorepo. Create a `values.yaml` with your settings:

```yaml
namespace: ainsel

agentOperator:
  image:
    repository: <registry>/ainsel/ainsel-k8s-ai-agent-operator
    tag: latest

connectorOperator:
  image:
    repository: <registry>/ainsel/ainsel-k8s-event-source-gateway-operator
    tag: latest

hub:
  image:
    repository: <registry>/ainsel/ainsel-hub-backend
    tag: latest
  nats:
    url: nats://nats.platform.svc.cluster.local:4222
  ingress:
    enabled: true
    host: your-domain.com
    path: /ainsel/api

ui:
  enabled: true
  image:
    repository: <registry>/ainsel/ainsel-hub-frontend
    tag: latest
  ingress:
    enabled: true
    host: your-domain.com
    path: /ainsel
```

> **Note:** Connectors, agents, triggers, and personas are created at runtime via the hub UI or REST API (`/api/v1/connectors`, `/api/v1/agents`, `/api/v1/triggers`, `/api/v1/personas`). The chart does not bootstrap them.

## Authentication

By default, when `auth.oidcIssuer` and `auth.oidcClientId` are left empty in `values.yaml`, the hub backend skips JWT validation entirely. This is intentional for local development but **must not be used in production**.

To enable OIDC-based authentication, set the following fields in your `values.yaml`:

```yaml
auth:
  oidcIssuer: "https://your-oidc-provider.example.com"
  oidcClientId: "ainsel"
```

Any OIDC provider that exposes a standard discovery endpoint (`/.well-known/openid-configuration`) is supported — for example Zitadel, Keycloak, or Dex.

The frontend reads the same values from the chart-rendered `runtime-config.js` and uses them to drive the login flow. No additional configuration is required beyond the two fields above.

---

## Step 4: Install

```bash
helm install ainsel ./chart -n ainsel -f values.yaml
```

The chart does not create the namespace itself by default (`createNamespace:
false`), so either pre-create it as in Step 2 or install with
`--create-namespace` instead.

## Step 5: Create a WebhookConnector and Configure the Forgejo Webhook

Create a connector via the hub UI (Settings → Connectors → New) or the REST API. Once saved, the connector operator provisions an Ingress whose path is shown in the connector detail view.

Point Forgejo at that URL:

1. Go to Forgejo Site Administration > Webhooks (or per-org webhooks)
2. Add webhook:
   - **URL**: The webhook endpoint shown in the hub UI connector detail
   - **Secret**: The HMAC secret you configured on the connector
   - **Content Type**: `application/json`
   - **Events**: Select issues, pull requests, push, etc.

## Step 6: Verify

```bash
# Check all pods are running
kubectl get pods -n ainsel

# Check CRDs are registered (no resources expected on a fresh install — create them via UI)
kubectl get agentimages,webhookconnectors -n ainsel

# Test API
curl https://your-domain.com/ainsel/api/health
curl https://your-domain.com/ainsel/api/v1/agents
```

## ArgoCD Deployment

For GitOps, create an ArgoCD Application that points at this repo's `chart/` directory:

```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: ainsel
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://your-forgejo/AInsel/ainsel.git
    targetRevision: main
    path: chart
    helm:
      valueFiles:
        - values.yaml
  destination:
    server: https://kubernetes.default.svc
    namespace: ainsel
  syncPolicy:
    automated:
      selfHeal: true
      prune: true
```

## Upgrading

```bash
helm upgrade ainsel ./chart -n ainsel -f values.yaml
```

Note: Helm does not update CRDs on upgrade. If CRDs changed, apply them manually:

```bash
kubectl apply -f chart/crds/
```

## Next steps

Once the platform is running, see the
[administrator guide](administrator-guide.md) to configure your first
agent, trigger, and connector. The guide includes a cookbook with
worked examples (code review, issue triage, comment Q&A,
issue → PR implementation).

For troubleshooting common deployment issues, see [docs/troubleshooting.md](troubleshooting.md).
