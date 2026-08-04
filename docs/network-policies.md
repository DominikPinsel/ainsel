# Network Policies

AInsel ships a default-deny network posture for its own namespace. This document inventories every policy the Helm chart creates, explains how enforcement works, and gives rules for extending the policy set when you add components the chart does not manage.

## Default Posture

Network policies are controlled by a single flag in `chart/values.yaml`:

```yaml
networkPolicy:
  enabled: true          # default: true
  ingressNamespace: ingress-nginx
```

With `enabled: true` (the default) the chart installs:

1. **`default-deny-ingress`** — denies all ingress to every pod in the namespace. Nothing receives inbound traffic unless an allow rule exists.
2. **Per-component ingress policies** — explicit allows for each chart-managed workload (see inventory below).
3. **`agent-egress`** — locks down agent pod egress to a known set of destinations (see [Security Hardening](security-hardening.md) for the rationale).

There is **no egress restriction** on non-agent components (hub-backend, hub-frontend, mcp, connectors). This is deliberate: hub-backend must reach MCP servers declared in AgentImages, and those URLs can point at arbitrary in-cluster or external endpoints. See [Known gaps](#known-gaps).

## Enforcement Requirements

NetworkPolicy resources only have an effect if the cluster's network stack enforces them.

| Cluster type | Enforcement |
|---|---|
| **k3s (default install)** | Enforced. k3s ships a built-in network policy controller that is enabled by default and works with all flannel backends, including `wireguard-native`. It is only turned off if the server was started with `--disable-network-policy`. |
| **k3s with `--disable-network-policy`** | Not enforced. Install Calico or Cilium instead (see below). |
| **Cilium / Calico / Weave / kube-router (with policy)** | Enforced. |
| **Plain flannel on upstream Kubernetes** | Not enforced. |

To check a k3s server's flags:

```bash
grep -A20 'ExecStart=' /etc/systemd/system/k3s.service | grep -i network-policy
```

If the flag is absent, the built-in controller is active.

> **Warning:** when policies are not enforced, `networkPolicy.enabled: true` still creates all NetworkPolicy resources — but they have no effect. That gives a false sense of security. Verify enforcement on a fresh install before relying on the posture (see [Verifying enforcement](#verifying-enforcement)).

## Policy Inventory (Chart-Managed)

All policies live in the release namespace (`.Values.namespace`).

| Policy | Applies to (podSelector) | Direction | Allows |
|---|---|---|---|
| `default-deny-ingress` | all pods | Ingress | nothing (baseline deny) |
| `hub-backend` | `component: hub-backend` | Ingress | `hub-frontend` pods, `mcp` pods, `agents` pods, pods in `networkPolicy.ingressNamespace` (browser API traffic) — all on `hub.port`; metrics port open for scraping |
| `hub-frontend` | `component: hub-frontend` | Ingress | any source, port 80 (served through the ingress controller) |
| `mcp` | `component: mcp` | Ingress | any source, port 8080 |
| `postgres` | `component: postgres` | Ingress | `hub-backend` only, port 5432 |
| `qdrant` | `component: qdrant` | Ingress | `agents` and `hub-backend` pods, ports 6333/6334 |
| `agent-egress` | `component: agents` | Egress | qdrant (6333), hub-backend (8080), DNS (53), any destination on 443/TCP (LLM APIs; FQDN scoping tracked in #652) |

Notes:

- `networkPolicy.ingressNamespace` must match the namespace of the ingress controller that serves the hub UI, otherwise browser API traffic is denied under default-deny.
- Policies are **additive**: an allow rule from *any* NetworkPolicy selecting a pod permits the traffic. Extensions never need to modify chart-managed policies.

## Traffic Flows That Cross Policy Boundaries

These are the flows that break most often when a policy is missing:

| Flow | Why it exists | Allowed by |
|---|---|---|
| browser → ingress → hub-backend | UI API calls | chart `hub-backend` policy (ingressNamespace) |
| agent → hub-backend | token validation, API | chart `hub-backend` + `agent-egress` |
| agent → qdrant | vector memory | chart `qdrant` + `agent-egress` |
| **hub-backend → MCP servers** | "Refresh MCP Tools" on AgentImages calls every configured MCP server directly from hub-backend to discover tools; the mcp gateway proxies MCP traffic | **not covered by the chart** for in-cluster servers outside the release — see below |
| agent → MCP servers | tools declared in the AgentImage | chart `agent-egress` covers 443/TCP; in-cluster servers on other ports need extension policies |

### Rule: hub-backend and agents must be allowed into every in-cluster MCP server

MCP servers referenced by AgentImages (the `mcpServers` list) are reached from two consumers:

1. **hub-backend** — during tool discovery ("Refresh MCP Tools") and when the hub MCP registry proxies a server.
2. **agent pods** — at runtime, when agents call the tools.

If the MCP server runs **outside** the AInsel release (a separate namespace or an out-of-band deployment like a mem0 stack), the chart cannot create its ingress policy. You must add one, allowing both consumers:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: my-mcp-ingress
  namespace: <namespace of the MCP server>
spec:
  podSelector:
    matchLabels: { ... }        # the MCP server pods
  policyTypes: [Ingress]
  ingress:
    # hub-backend: tool discovery + registry proxy
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: <ainsel namespace>
          podSelector:
            matchLabels:
              app.kubernetes.io/component: hub-backend
      ports: [{port: 8080}]
    # agents: runtime tool calls
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: <ainsel namespace>
          podSelector:
            matchLabels:
              app.kubernetes.io/component: agents
      ports: [{port: 8080}]
```

Cross-namespace rules need both `namespaceSelector` and `podSelector` in the same `from` entry — a `namespaceSelector` alone would allow every pod in that namespace.

If agents reach the server in a different namespace, also extend **agent egress**: `agent-egress` only permits 443/TCP outbound, so a plain-HTTP in-cluster server needs an additive egress policy selecting `component: agents` (or `managed-by: agent-operator`, see below).

### Label caveat for agent pods

The chart's policies select agent pods with `app.kubernetes.io/component: agents`. Agent pods created by older operator versions only carry `app.kubernetes.io/managed-by: agent-operator`. Extension policies should match whichever label population exists in your cluster (match both in separate `from` entries if in doubt).

## Known Gaps

- **No egress lockdown for hub-backend / mcp / connectors.** hub-backend intentionally needs open egress because AgentImages can declare MCP servers at arbitrary external URLs. FQDN-scoped egress filtering for agents is tracked in #652 (sidecar proxy approach).
- **`hub-frontend` and `mcp` ingress is open to any source** on their serving port; they are expected to sit behind an ingress controller with its own access controls (e.g. an auth proxy).
- **Metrics ports** on hub-backend are open for scraping without source restriction.

## Verifying Enforcement

Test a flow from the exact network namespace of the client pod using an ephemeral debug container:

```bash
kubectl -n <ns> debug <client-pod> \
  --image=<any image with curl> --target=<container> \
  -- sh -c 'curl -s -m 8 -o /dev/null -w "%{http_code}\n" http://<target-svc>:<port>/'
```

Interpretation:

- Connection fails immediately (curl rc=7, connection reset) while the same call succeeds from a pod that is allowed → a NetworkPolicy is denying the traffic.
- The call hangs until timeout → check firewalls/routing outside the cluster; NetworkPolicy denials on k3s's built-in controller typically reject rather than drop.

Compare against a control source that you know is allowed to rule out the server itself being down.

## Troubleshooting Checklist

Symptoms of a missing allow rule:

- **Refresh MCP Tools** in the UI shows warnings for in-cluster servers, or newly added servers never populate tools.
- Agents fail to call an MCP tool that works from other clients.
- Webhook connectors receive no events (ingress controller → connector denied).
- UI requests fail with 502/504 at the ingress while hub-backend is up (`ingressNamespace` mismatch).

Fix: identify the client pod's labels and the target pod's labels, then add an additive NetworkPolicy granting ingress to the target (and egress on the client side if the client is an agent).
