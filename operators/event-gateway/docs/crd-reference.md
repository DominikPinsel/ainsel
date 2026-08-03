# CRD Reference: WebhookConnector

**API Group:** `ainsel.dev`
**Version:** `v1alpha1`
**Kind:** `WebhookConnector`

## Spec Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string | Yes | Internal Forgejo URL for API calls within the cluster |
| `externalUrl` | string | Yes | Public-facing Forgejo URL |
| `webhookEndpoint` | string | Yes | URL that Forgejo sends webhook POST requests to |
| `credentials.secretRef.name` | string | Yes | Secret name containing Forgejo admin API token |
| `credentials.secretRef.key` | string | Yes | Key in the Secret |
| `webhookSecret.secretRef.name` | string | Yes | Secret name containing HMAC secret |
| `webhookSecret.secretRef.key` | string | Yes | Key in the Secret |
| `events` | []string | Yes | List of Forgejo event types to subscribe to |
| `image.repository` | string | Yes | Container image repository for forgejo-connector |
| `image.tag` | string | Yes | Container image tag |
| `resources` | ResourceRequirements | No | CPU/memory requests and limits |

## Status Fields

| Field | Type | Description |
|-------|------|-------------|
| `conditions` | []Condition | Standard Kubernetes conditions |
| `webhookId` | int64 | Forgejo webhook ID once registered |
| `observedGeneration` | int64 | Last observed spec generation |

## kubectl Columns

```
NAME     URL                                    READY   WEBHOOK   AGE
forgejo  https://forgejo.example.com    True    True      5d
```

## Full Example

```yaml
apiVersion: ainsel.dev/v1alpha1
kind: WebhookConnector
metadata:
  name: forgejo
  namespace: ainsel
spec:
  url: "http://forgejo.forgejo.svc:3000"
  externalUrl: "https://forgejo.example.com"
  webhookEndpoint: "http://forgejo-connector.ainsel.svc:8080/"
  credentials:
    secretRef:
      name: forgejo-admin-token
      key: token
  webhookSecret:
    secretRef:
      name: forgejo-webhook-secret
      key: secret
  events:
    - issues
    - pull_request
    - issue_comment
    - pull_request_review
    - push
    - repository
  image:
    repository: localhost:30500/ainsel/ainsel-event-source-gateway-forgejo
    tag: latest
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 128Mi
```

## Supported Forgejo Events

| Event | Description |
|-------|-------------|
| `issues` | Issue lifecycle (open, close, reopen, assign, label) |
| `issue_comment` | Comments on issues |
| `pull_request` | PR lifecycle (open, close, merge) |
| `pull_request_review` | PR reviews |
| `push` | Git push events |
| `repository` | Repository lifecycle (create, delete) |
