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
| `hub-backend` | `component: hub-backend` | Ingress | `hub-frontend` pods, `mcp` pods, agent pods (`component: agents`), webhook connector pods (`managed-by: connector-operator`), pods in `networkPolicy.ingressNamespace` (browser API traffic) — all on `hub.port`; metrics port open for scraping |
| `hub-frontend` | `component: hub-frontend` | Ingress | any source, port 80 (served through the ingress controller) |
| `mcp` | `component: mcp` | Ingress | any source, port 8080 |
| `postgres` | `component: postgres` | Ingress | `hub-backend` only, port 5432 |
| `qdrant` | `component: qdrant` | Ingress | agent pods (`component: agents`) and `hub-backend` pods, ports 6333/6334 |
| `agent-egress` | `component: agents` | Egress | qdrant (6333), hub-backend (8080), DNS (53), any destination on 443/TCP (LLM APIs; FQDN scoping tracked in #652) |
| `connectors-webhook-ingress` | `managed-by: connector-operator` | Ingress | ingress controller's namespace (external webhooks) plus any peers listed in `networkPolicy.connectorWebhookSources`, port `http` |

Notes:

- `networkPolicy.ingressNamespace` must match the namespace of the ingress controller that serves the hub UI, otherwise browser API traffic is denied under default-deny.
- Policies are **additive**: an allow rule from *any* NetworkPolicy selecting a pod permits the traffic. Extensions never need to modify chart-managed policies.

### Agent pod selectors

Chart-managed policies select agent pods with
`app.kubernetes.io/component: agents` — the semantic identity label ("this pod
*is* an agent"). It stays precise even if the agent-operator later manages
other workload types, which would carry their own `component` value; selecting
by provenance (`managed-by: agent-operator`) instead would grant agent
privileges to any future operator-managed pod.

**Migration prerequisite:** every agent pod must carry the label. Deployments
created by operator versions before the label existed are reconciled by the
operator's preserve-selector fix, which keeps their immutable selector but
updates the pod template so rolled-out pods gain `component: agents`. Deploy
this chart only after that operator version has reconciled all agents —
otherwise pods without the label are isolated (ingress) or lose their egress
lockdown. Verify with:

```bash
kubectl get pods -n <ns> -l app.kubernetes.io/managed-by=agent-operator \
  -o json | jq -r '.items[] | select((.metadata.labels["app.kubernetes.io/component"]//"") != "agents") | .metadata.name'
# must print nothing
```

## Traffic Flows That Cross Policy Boundaries

These are the flows that break most often when a policy is missing:

| Flow | Why it exists | Allowed by |
|---|---|---|
| browser → ingress → hub-backend | UI API calls | chart `hub-backend` policy (ingressNamespace) |
| agent → hub-backend | token validation, API | chart `hub-backend` + `agent-egress` |
| connector → hub-backend | webhook receivers publish accepted webhooks (`POST /api/internal/events`) | chart `hub-backend` policy (connector pods); connectors have no egress restriction |
| external → connector | webhook delivery through the ingress controller | chart `connectors-webhook-ingress` (ingressNamespace) |
| in-cluster producer → connector | e.g. a Forgejo in the cluster delivering directly via the `*-webhook` service | chart `connectors-webhook-ingress` (`connectorWebhookSources`) |
| agent → qdrant | vector memory | chart `qdrant` + `agent-egress` |
| **hub-backend → MCP servers** | "Refresh MCP Tools" on AgentImages calls every configured MCP server directly from hub-backend to discover tools; the mcp gateway proxies MCP traffic | **not covered by the chart** for in-cluster servers outside the release — see below |
| agent → MCP servers | tools declared in the AgentImage | chart `agent-egress` covers 443/TCP; in-cluster servers on other ports need extension policies |

## Adding an MCP Server (Checklist)

MCP servers referenced by AgentImages (the `mcpServers` list) are reached from two consumers:

1. **hub-backend** — during tool discovery ("Refresh MCP Tools") and when the hub MCP registry proxies a server.
2. **agent pods** — at runtime, when agents call the tools.

What you have to configure depends on where the server runs:

| MCP server location | hub-backend | agent pods |
|---|---|---|
| External, HTTPS (port 443) | nothing — hub egress is unrestricted | nothing — `agent-egress` allows 443/TCP |
| External, plain HTTP | nothing | additive egress policy needed (prefer HTTPS instead) |
| In-cluster, other namespace or out-of-band | ingress allow on the server | ingress allow on the server **+** additive egress policy (unless port 443) |
| In-cluster, same namespace | ingress allow on the server | ingress allow on the server **+** additive egress policy (unless port 443) |

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

If agents reach the server in a different namespace, also extend **agent egress**: `agent-egress` only permits 443/TCP outbound, so a plain-HTTP in-cluster server needs an additive egress policy selecting `component: agents`.

### Label caveat for agent pods

The chart selects agent pods by `app.kubernetes.io/component: agents` (see
[Agent pod selectors](#agent-pod-selectors)). Extension policies that target
agent pods should use the same label. Pods only lack it while a pre-label
Deployment has not yet been reconciled/rolled by an operator that adds it —
see the migration prerequisite above.

## Integrating External Services

Services that live outside the AInsel namespace but talk to the platform:

| Service | Direction | What to configure |
|---|---|---|
| **Prometheus / metrics scraping** | external → hub-backend | Nothing for hub-backend: the chart opens `hub.metricsPort` (default 9090) to any source under the `hub-backend` policy. Scrape via the hub-backend Service. Components without such an open metrics port (operators, connectors) are unreachable under default-deny and need an additive ingress policy from the monitoring namespace. |
| **Ingress controller** | external → hub-backend, hub-frontend, mcp | Set `networkPolicy.ingressNamespace` to the namespace of the ingress controller that serves the hub. A second controller in a different namespace needs an additive policy. |
| **Auth proxies** (Authelia, oauth2-proxy, ...) | ingress controller or proxy → hub-backend | If the proxy runs as a pod in its own namespace rather than a sidecar of the ingress controller, add an ingress allow for its namespace/pods to the target service. |
| **Webhook sources** (Forgejo, GitHub, ...) | external → connector webhook receivers | Covered by the chart's `connectors-webhook-ingress` policy, which allows `networkPolicy.ingressNamespace` to reach connector pods (`managed-by: connector-operator`). For an in-cluster producer that delivers webhooks directly (bypassing the ingress controller), add its namespace/pods to `networkPolicy.connectorWebhookSources`. The reverse direction (connector → hub-backend publish) is covered by the chart's `hub-backend` policy. |
| **External MCP / LLM APIs** | hub-backend, agents → external | Nothing for HTTPS endpoints (see checklist above). |

## Temporarily Relaxing Policies (Dev/Testing)

To make the namespace quickly accessible while trying something out, pick the smallest relaxation that works. All options are reversible.

**Option 1 — temporary allow-all (recommended).** Additive policies are a union, so a single allow-all policy overrides both default-deny and the agent egress lockdown without touching anything the chart manages. It survives `helm upgrade`; delete it when done:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: tmp-allow-all
  namespace: <ainsel namespace>
spec:
  podSelector: {}
  policyTypes: [Ingress, Egress]
  ingress:
    - {}   # empty rule = all sources, all ports
  egress:
    - {}
```

```bash
kubectl delete networkpolicy tmp-allow-all -n <ainsel namespace>
```

**Option 2 — remove only default-deny.** Pods that have no policy of their own become fully reachable inbound; the component allow rules and the agent egress lockdown stay in place. Useful when a new experimental service must receive traffic:

```bash
kubectl delete networkpolicy default-deny-ingress -n <ainsel namespace>
# restore with: helm upgrade ...
```

**Option 3 — full off switch.** Set `networkPolicy.enabled: false` and run `helm upgrade`. Removes the entire chart policy set, including the agent egress lockdown. Re-enable by setting it back to `true`.

> Note: on a cluster that does not enforce NetworkPolicy at all (see [Enforcement Requirements](#enforcement-requirements)), these relaxations change nothing — and neither did the original restrictions.

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
- In-cluster webhook deliveries fail with `connect: connection refused` to a `*-webhook` service (producer → connector denied; add the producer to `networkPolicy.connectorWebhookSources`).
- Webhook deliveries are answered `500 publish failed` although signatures validate (connector → hub-backend publish denied).
- UI requests fail with 502/504 at the ingress while hub-backend is up (`ingressNamespace` mismatch).
- Agents stop claiming tasks and their poll loop logs `connection refused` against hub-backend (agent pods not matched by the chart policies; verify the pods carry `component: agents` — see [Agent pod selectors](#agent-pod-selectors)).

Fix: identify the client pod's labels and the target pod's labels, then add an additive NetworkPolicy granting ingress to the target (and egress on the client side if the client is an agent).
