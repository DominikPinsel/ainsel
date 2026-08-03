# Configuration

## Operator Deployment

The ainsel-k8s-event-source-gateway-operator is configured via the ainsel-chart Helm values:

| Helm Value | Description |
|------------|-------------|
| `connectorOperator.image.repository` | Container image repository |
| `connectorOperator.image.tag` | Container image tag |
| `connectorOperator.resources` | CPU/memory requests and limits |

## WebhookConnector CRD Configuration

See [CRD Reference](crd-reference.md) for the full spec.

### Required Secrets

The WebhookConnector CRD references two Secrets:

**Admin API Token** (`credentials.secretRef`):
- Used to register webhooks via the Forgejo API
- Must have admin permissions on the target repositories

**Webhook Secret** (`webhookSecret.secretRef`):
- Shared HMAC secret for signature validation
- Passed to the ainsel-event-source-gateway-forgejo pod as `WEBHOOK_SECRET`

### Example Secrets

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: forgejo-admin-token
  namespace: ainsel
type: Opaque
stringData:
  token: "your-forgejo-admin-api-token"
---
apiVersion: v1
kind: Secret
metadata:
  name: forgejo-webhook-secret
  namespace: ainsel
type: Opaque
stringData:
  secret: "your-webhook-hmac-secret"
```

## RBAC

The operator requires permissions to:
- Watch and manage `WebhookConnector` CRDs
- Create and manage Deployments and Services
- Read Secrets (for credentials and webhook secrets)
