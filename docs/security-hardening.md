# Security Hardening

This document describes the security hardening applied to AInsel agent pods, the threat model it addresses, and what is not yet covered.

## What Is Hardened

### Pod-Level SecurityContext

Agent pods run with the following pod-level security settings (when `spec.runtime.securityHardened` is `true`, the default):

| Field | Value | Purpose |
|---|---|---|
| `runAsNonRoot` | `true` | Prevents containers from running as UID 0 |
| `runAsUser` | `1000` | All containers run as the `agent` user |
| `runAsGroup` | `1000` | All containers use GID 1000 |
| `fsGroup` | `1000` | Mounted volumes are group-owned by GID 1000 |
| `seccompProfile.type` | `RuntimeDefault` | Applies the container runtime's default seccomp profile |

### Container-Level SecurityContext

The `agent` container and the `setup-pi-models` init container run with:

| Field | Value | Purpose |
|---|---|---|
| `allowPrivilegeEscalation` | `false` | Prevents gaining more privileges than the parent process |
| `readOnlyRootFilesystem` | `true` | Prevents writes to the container image filesystem |
| `capabilities.drop` | `[ALL]` | Drops all Linux capabilities |

### Writable Volumes

With `readOnlyRootFilesystem: true`, the following `emptyDir` volumes provide writable scratch space:

- `/workspace` — agent working directory for code checkouts
- `/home/agent/.pi/agent` — Pi runtime session data, models.json, skills
- `/tmp` — temporary files for shell tools, git, etc.

### NetworkPolicy Egress Rules

When `networkPolicy.enabled: true` (the default), agent pods are restricted to egress traffic only to:

| Destination | Port | Purpose |
|---|---|---|
| NATS | 4222 | Message bus |
| Qdrant | 6333 | Vector memory |
| Hub backend | 8080 | Agent token validation, API calls |
| DNS | 53 (UDP/TCP) | Name resolution |
| Any (HTTPS) | 443/TCP | LLM API endpoints (see caveat below) |

Ingress to agent pods is denied by the namespace-wide default-deny policy.

## Threat Model

These controls protect against:

- **Container escape:** Dropping all capabilities, preventing privilege escalation, and running as non-root reduce the attack surface for kernel exploits and container breakout.
- **Filesystem tampering:** A read-only root filesystem prevents an attacker (or a confused agent) from modifying binaries, configs, or dropping malicious payloads into the container image layer.
- **Lateral movement:** NetworkPolicy egress rules restrict agent pods to only the services they need. A compromised agent cannot freely scan or attack other pods in the cluster.
- **Seccomp filtering:** The default seccomp profile blocks dangerous syscalls (e.g. `mount`, `reboot`, `ptrace`) that are not needed by the agent runtime.

## What Is NOT Covered Yet

The following are tracked in the parent Epic (#652) and are not part of this initial hardening:

- **FQDN-based egress filtering:** The current 443/TCP egress rule is broad by necessity — agents must reach external LLM APIs (ollama.com, opencode.ai, aliyuncs.com, custom providers) over HTTPS. Standard Kubernetes NetworkPolicy cannot express FQDN-based rules. The Epic tracks a sidecar proxy approach for FQDN-scoped egress filtering.
- **Per-request audit logging:** Network flows are not logged per-request today.
- **TLS inspection:** Egress TLS traffic is not inspected or intercepted.
- **Secret isolation from agent-visible scope:** Agent pods can read their own API key secrets. Further isolation (e.g. short-lived tokens, secret injection via sidecar) is deferred.
- **Sidecar container hardening:** Pod-level `runAsNonRoot`/`runAsUser: 1000` applies to all containers in the pod, including sidecars. Sidecar images (AgentImage-declared sidecars and the auto-injected `chat` sidecar) **must** already run as non-root UID 1000 or agent pods will fail to start. Container-level hardening (`readOnlyRootFilesystem`, `drop ALL`) is not applied to sidecars because their images are not under this repo's guaranteed control.

## How to Opt Out

Set `spec.runtime.securityHardened: false` on an `Agent` CR to disable pod and container security hardening for that specific agent:

```yaml
apiVersion: ainsel.dev/v1alpha1
kind: Agent
metadata:
  name: my-agent
spec:
  runtime:
    securityHardened: false
  # ...
```

When `securityHardened` is `false`:
- Pod `securityContext` is limited to `fsGroup: 1000` only (no `runAsNonRoot`, `runAsUser`, `runAsGroup`, or `seccompProfile`)
- No container-level `securityContext` is set on the agent or init containers
- No `/tmp` emptyDir volume is added
- The init container retains `chown` commands for backward compatibility

The default (when the field is `nil` or omitted) is `true` — hardening is enabled.

## CNI Requirement

**NetworkPolicy enforcement requires a CNI plugin that supports it.** The k3s default (flannel) does **not** enforce NetworkPolicy rules. If you are using k3s, install Calico or Cilium as your CNI:

```bash
# k3s with Calico
k3s install --flannel-backend=none --cluster-cidr=192.168.0.0/16
# Then install Calico per https://docs.tigera.io/calico/latest/getting-started/kubernetes/k3s/quickstart

# k3s with Cilium
k3s install --flannel-backend=none
# Then install Cilium per https://docs.cilium.io/en/stable/installation/k3s/
```

Without a supporting CNI, enabling `networkPolicy.enabled: true` creates the NetworkPolicy resources but they have no effect — giving a false sense of security.

## Sidecar Non-Root Constraint

Pod-level `runAsNonRoot: true` and `runAsUser: 1000` apply to **every** container in the agent pod, including:

- The `agent` container
- The `setup-pi-models` init container
- Any AgentImage-declared sidecar containers
- The auto-injected `chat` sidecar (when `CHAT_MCP_IMAGE` is configured)

All sidecar images **must** be able to run as UID 1000. If a sidecar image runs as root or a different UID, the pod will fail to start with a `RunAsNonRoot` or permission error. Use `spec.runtime.securityHardened: false` as an escape hatch while migrating sidecar images.
