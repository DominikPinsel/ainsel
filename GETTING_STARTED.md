# Getting Started with AInsel

This guide takes you from zero to a running AInsel install with one connector, one agent, and one trigger. By the end, a test webhook will invoke an AI agent and you'll be ready to wire real connectors.

## Prerequisites

- A Kubernetes cluster (k3s or any CNCF-conformant distribution)
- `kubectl` configured for the target cluster
- Helm 3.x (`helm version` to verify)
- A container registry the cluster can pull images from
- NATS JetStream — bundled by default in the AInsel Helm chart, no separate install needed
- _(Optional)_ An OIDC provider (e.g. Dex, Keycloak, Zitadel) for authentication on the operations console

## 1. Install AInsel

Install the Helm chart from the repository with a minimal values override:

```bash
helm install ainsel ./chart \
  --namespace ainsel \
  --create-namespace \
  -f my-values.yaml
```

At a minimum, override the image repositories in your values file to point at your registry. See [`chart/values-example.yaml`](chart/values-example.yaml) for a starting point and [`docs/deployment.md`](docs/deployment.md) for the full values reference.

## 2. Verify the Install

```bash
kubectl get pods -n ainsel
```

Expected output (all pods `Running`):

```
NAME                                          READY   STATUS    RESTARTS   AGE
ainsel-hub-xxxxxxxxx-xxxxx                    1/1     Running   0          2m
ainsel-agent-operator-xxxxxxxx-xxxxx          1/1     Running   0          2m
ainsel-connector-operator-xxxxxxxx-xxxxx      1/1     Running   0          2m
ainsel-nats-0                                 1/1     Running   0          2m
ainsel-frontend-xxxxxxxxx-xxxxx               1/1     Running   0          2m
```

If a pod is not `Running`, check logs:

```bash
kubectl logs -n ainsel deployment/ainsel-hub
```

## 3. Create a Connector

A connector turns external webhook events into AInsel's canonical event stream. The built-in `WebhookConnector` CRD handles generic webhook sources (configured for Forgejo today); for other sources see [`docs/writing-a-connector.md`](docs/writing-a-connector.md).

Create the webhook secret, then apply a `WebhookConnector`:

```bash
kubectl create secret generic my-forge-webhook-secret \
  --namespace ainsel \
  --from-literal=secret=<your-webhook-secret>
```

```yaml
# connector.yaml
apiVersion: ainsel.dev/v1alpha1
kind: WebhookConnector
metadata:
  name: my-forge
  namespace: ainsel
spec:
  displayName: My Forgejo
  webhookEndpoint: https://your-domain.example.com/webhooks/my-forge
  signatureHeader: X-Forgejo-Signature
  webhookSecret:
    secretRef:
      name: my-forge-webhook-secret
      key: secret
  image:
    repository: <your-registry>/ainsel-webhook-receiver  # defaults to dpinsel/ in values-example.yaml
    tag: latest
```

```bash
kubectl apply -f connector.yaml
```

The operator reconciles the connector and starts a webhook-receiver deployment that listens for incoming webhooks.

## 4. Create an AgentImage and Agent

An `AgentImage` defines the container image and tool configuration for an agent runtime. An `Agent` references an image and adds the LLM configuration and persona.

```yaml
# agent-image.yaml
apiVersion: ainsel.dev/v1alpha1
kind: AgentImage
metadata:
  name: pi-runtime
  namespace: ainsel
spec:
  displayName: Pi Runtime
  image:
    repository: <your-registry>/ainsel-pi
    tag: latest
```

```yaml
# agent.yaml
apiVersion: ainsel.dev/v1alpha1
kind: Agent
metadata:
  name: pr-reviewer
  namespace: ainsel
spec:
  displayName: PR Reviewer
  imageRef:
    name: pi-runtime
  runtime:
    imagePullPolicy: IfNotPresent
  llm:
    model: claude-opus-4-7
    provider: opencode
    maxTurns: 10
  persona:
    id: <persona-ulid>   # create a persona via the UI or API first
```

```bash
kubectl apply -f agent-image.yaml
kubectl apply -f agent.yaml
```

The agent operator creates a Deployment for the agent runtime. Check it is running:

```bash
kubectl get pods -n ainsel -l ainsel.dev/agent=pr-reviewer
```

## 5. Create a Trigger

Triggers are managed via the hub REST API (stored in Postgres, not as CRDs). A trigger says: _when an event matching these filters arrives from this connector, invoke this agent_.

```bash
curl -X POST https://your-domain.example.com/api/v1/triggers \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <token>" \
  -d '{
    "name": "Review new PRs",
    "agentRef": "pr-reviewer",
    "connectorRef": "my-forge",
    "filters": [
      {"field": "type", "operator": "eq", "value": "pull_request"},
      {"field": "action", "operator": "eq", "value": "opened"}
    ]
  }'
```

You can also create triggers through the operations console UI under **Agents → Triggers**.

## 6. Send a Test Webhook

Simulate an incoming webhook to verify the full path:

```bash
curl -s -X POST https://your-domain.example.com/webhooks/my-forge \
  -H "Content-Type: application/json" \
  -H "X-Forgejo-Signature: sha256=<hmac-of-body-with-your-webhook-secret>" \
  -d '{
    "event": "pull_request",
    "action": "opened",
    "pull_request": {
      "number": 1,
      "title": "Test PR",
      "diff": "--- a/foo.go\n+++ b/foo.go\n@@ -1 +1 @@\n-old\n+new"
    },
    "repository": {"full_name": "my-org/my-repo"}
  }'
```

Then check the agent logs:

```bash
kubectl logs -n ainsel -l ainsel.dev/agent=pr-reviewer --tail=50
```

You should see the event received, the LLM invocation, and the tool call that posts the review comment.

## Next Steps

| Document | What it covers |
|---|---|
| [`docs/deployment.md`](docs/deployment.md) | Full Helm values reference, TLS, OIDC, external NATS, upgrades |
| [`docs/writing-a-connector.md`](docs/writing-a-connector.md) | Building a connector for a new source system |
| [`docs/administrator-guide.md`](docs/administrator-guide.md) | Concepts, persona authoring, cost management, cookbook |
| [`docs/crd-reference.md`](docs/crd-reference.md) | Full CRD specs for `Agent`, `AgentImage`, `WebhookConnector` |
| [`docs/troubleshooting.md`](docs/troubleshooting.md) | Common failure modes and remediation steps |
