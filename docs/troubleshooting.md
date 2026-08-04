# Troubleshooting

This guide covers the most common failure modes when operating AInsel. Use `<namespace>` as a placeholder for the Kubernetes namespace where your platform is installed.

---

## Hub pod not starting

Check the hub logs first:

```bash
kubectl logs -n <namespace> deploy/ainsel-hub
```

Common causes and fixes:

- **Missing Postgres secret** — The hub exits early if the database secret is absent or has the wrong key names. Verify the secret exists and contains the expected keys (`host`, `port`, `user`, `password`, `dbname` or a single `dsn`). Re-create the secret and restart the hub deployment.
- **Hub not reachable** — If you see `hub: connection refused` or a connection-refused error, confirm the hub URL in `values.yaml` is correct and that the hub pod in the platform namespace is running: `kubectl get pods -n <nats-namespace>`.
- **Bad OIDC config** — An `oidc: failed to fetch provider metadata` error means the issuer URL is unreachable from inside the cluster. Check that the URL is correct, that DNS resolves, and that the cluster can reach the OIDC provider. Verify the client ID matches what is registered.
- **CRD not installed** — A `no kind "Agent" is registered` error means CRDs were not applied. Run `kubectl apply -f chart/crds/` and restart the hub.

After fixing the root cause, rollout-restart the deployment:

```bash
kubectl rollout restart -n <namespace> deploy/ainsel-hub
kubectl rollout status -n <namespace> deploy/ainsel-hub
```

---

## Webhook not being received

Start by confirming the connector resource is healthy:

```bash
kubectl get webhookconnector -n <namespace>
kubectl describe webhookconnector <name> -n <namespace>
```

The `Ready` condition must be `True`. If it is not, the operator has not yet provisioned the receiver Deployment and Ingress — check operator logs:

```bash
kubectl logs -n <namespace> deploy/ainsel-connector-operator
```

Check the webhook-receiver (event-gateway) pod logs for incoming requests:

```bash
kubectl logs -n <namespace> deploy/<connector-name>-event-gateway
```

A `403 Forbidden` or `HMAC mismatch` error means the secret on the Forgejo webhook does not match the secret stored in the Kubernetes secret referenced by the connector. Update the Forgejo webhook secret or recreate the Kubernetes secret to match, then restart the event-gateway pod.

Verify the Ingress is present and routing correctly:

```bash
kubectl get ingress -n <namespace>
curl -v <webhook-url>/healthz
```

If the Ingress exists but requests are not arriving, check that the Ingress controller is running and that the external DNS or IP resolves to the cluster. With `networkPolicy.enabled: true`, also confirm a NetworkPolicy allows ingress from the ingress controller's namespace to the connector's webhook-receiver pods — connector pods are created by the operator, not the chart, so the chart's default-deny posture requires an additive policy for them. See [Network Policies](network-policies.md).

---

## Agent not triggering

Check that trigger resources exist and are healthy:

```bash
kubectl get trigger -n <namespace>
kubectl describe trigger <name> -n <namespace>
```

Inspect hub logs for routing decisions. The hub logs each event it receives and whether it matched a trigger:

```bash
kubectl logs -n <namespace> deploy/ainsel-hub | grep -i trigger
```

Look for lines indicating an event was received but no trigger matched — this usually means the event type or filter does not align with what is being sent. Adjust the trigger's `eventType` and `filters` fields via the UI or API.

Verify the connector is publishing events to the event queue by checking the `hub_events_consumed_total` metric:

```bash
kubectl exec -n <namespace> deploy/ainsel-hub -- curl -s http://localhost:8080/metrics | grep hub_events_consumed_total
```

A counter that never increments means events are not reaching the hub. Re-check the connector and hub connectivity.

---

## Agent pod stuck in Pending or CrashLoopBackOff

Describe the pod to get the full picture:

```bash
kubectl describe pod -n <namespace> <pod-name>
```

Common causes:

- **Insufficient cluster resources** — A `Pending` pod with `Insufficient cpu` or `Insufficient memory` in the events means the cluster does not have capacity. Scale the cluster or reduce the agent's resource requests in `values.yaml`.
- **Missing image pull secret** — `ImagePullBackOff` means the cluster cannot pull the agent image. Verify the registry is accessible and the image pull secret is configured on the ServiceAccount.
- **API key not set** — A `CrashLoopBackOff` with an error like `api-key secret not found` means the LLM API key secret is missing. The operator looks for a secret named `<agent-name>-<provider>-key` with key `api-key`. Create it: `kubectl create secret generic <agent-name>-<provider>-key -n <namespace> --from-literal=api-key=<key>`.
- **Persona or tool misconfiguration** — Check the agent pod logs for startup errors: `kubectl logs -n <namespace> <pod-name> --previous`.

---

## OIDC / authentication errors

If the hub logs show `authMW is nil`, the hub is running without authentication middleware. This is expected in local development mode and is not an error — the API is open.

If authentication is configured and you are seeing `401 Unauthorized` responses:

- Verify the OIDC issuer URL in the hub configuration is reachable from inside the cluster.
- Confirm the client ID matches the one registered with the OIDC provider.
- Validate a token manually against the OIDC userinfo endpoint:

```bash
curl -H "Authorization: Bearer <token>" <issuer-url>/protocol/openid-connect/userinfo
```

A `token is expired` error means the client needs to refresh its token before calling the API. A `audience mismatch` error means the token was issued for a different client ID.

---

## MCP server not connecting

Check the MCPServer resource status:

```bash
kubectl get mcpserver -n <namespace>
kubectl describe mcpserver <name> -n <namespace>
```

Look for connection errors in the hub logs:

```bash
kubectl logs -n <namespace> deploy/ainsel-hub | grep -i mcp
```

Verify the MCP server URL is reachable from inside the cluster by exec-ing into the hub pod:

```bash
kubectl exec -n <namespace> deploy/ainsel-hub -- curl -sv <mcp-server-url>/health
```

If the URL is not reachable, confirm the MCP server Deployment is running and that the Service name and port are correct. If the MCP server is external to the cluster, verify network policies and egress rules allow the connection.

Remember that MCP servers are reached from **two** clients: hub-backend (tool discovery for AgentImages — the "Refresh MCP Tools" action — and the hub MCP registry) and agent pods at runtime. Both need ingress to the server, and agents additionally need egress if the server is in-cluster and not on 443/TCP. With `networkPolicy.enabled: true`, an in-cluster MCP server without an allow rule is silently unreachable — connections are rejected rather than timing out. See [Network Policies](network-policies.md) for the policy inventory and a verification recipe.

---

## Checking platform health

The hub exposes a health endpoint that checks all internal subsystems (database, operator connectivity):

```bash
kubectl exec -n <namespace> deploy/ainsel-hub -- curl -s http://localhost:8080/api/v1/platform/health
```

A healthy response returns `200 OK` with a JSON body listing each subsystem and its status. Any subsystem showing `"status": "unhealthy"` indicates where to investigate next.

---

## Collecting diagnostics

Use these commands to gather a full picture before escalating or filing a bug:

```bash
# Overview of all resources in the namespace
kubectl get all -n <namespace>

# CRD resource status
kubectl get agents,triggers,webhookconnectors,mcpservers -n <namespace>

# Describe a specific resource for events and conditions
kubectl describe agent <name> -n <namespace>
kubectl describe trigger <name> -n <namespace>

# Recent logs from all platform components
kubectl logs -n <namespace> deploy/ainsel-hub --tail=100
kubectl logs -n <namespace> deploy/ainsel-agent-operator --tail=100
kubectl logs -n <namespace> deploy/ainsel-connector-operator --tail=100

# Events in the namespace (often the fastest way to find the root cause)
kubectl get events -n <namespace> --sort-by='.lastTimestamp'
```

If Loki is configured in your cluster, use its query interface to aggregate logs across all pods in the namespace by filtering on `namespace=<namespace>`. This is especially useful for correlating a NATS event with the hub routing decision and the agent pod startup that followed it.
