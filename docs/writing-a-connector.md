# Writing a Connector for AInsel

This tutorial walks you through building a new connector end-to-end: from
understanding the architecture to shipping a working connector that integrates
an external event source with the AInsel platform.

**Who this is for:** a competent Go developer who wants to add support for a
new event source (Jira, Slack, Office365, a proprietary CI system, etc.).

**Time estimate:** under a day for a working connector, given this guide and
the reference implementation at
[`services/webhook-receiver/`](../services/webhook-receiver/).

> **Last verified against:** main branch (commit `0c9e3c6`), May 2026.
> File paths and struct names should match the current codebase; if they
> drift, please open an issue or PR to update this doc.

---

## Table of Contents

1. [How Connectors Work](#how-connectors-work)
2. [The Canonical Event Schema](#the-canonical-event-schema)
3. [What You Need to Build](#what-you-need-to-build)
4. [Step 1: Define the Connector CRD](#step-1-define-the-connector-crd)
5. [Step 2: Implement the Service](#step-2-implement-the-service)
   - [Webhook Handler (Inbound)](#webhook-handler-inbound)
   - [Normalizer](#normalizer)
   - [Publisher](#publisher)
   - [Reactor (Outbound)](#reactor-outbound)
   - [Bot Detection](#bot-detection)
   - [Wiring `main.go`](#wiring-maingo)
6. [Step 3: Implement the Operator](#step-3-implement-the-operator)
7. [Step 4: Helm Chart Integration](#step-4-helm-chart-integration)
8. [Step 5: Register Event Types](#step-5-register-event-types)
9. [Testing](#testing)
10. [Checklist](#checklist)
11. [Reference Implementation](#reference-implementation)

---

## How Connectors Work

A connector is the bridge between an external system and the AInsel platform.
It has two parts:

| Part | Location | Role |
|------|----------|------|
| **Operator** | `operators/event-gateway/` | A Kubernetes controller that watches a connector CRD and reconciles a Deployment + Service + (optionally) Ingress |
| **Service** | `services/new-source-gateway/` | A long-running Go binary that receives webhooks, normalizes them, and publishes canonical events to the event queue |

```
External System ──webhook──▶ Connector Service ──normalize──▶ NATS EVENTS stream ──▶ Hub ──▶ Agents
                                                                    ▲
Agent ──👀 reaction─────── Connector Service ◀──subscribe──────────┘
                              (outbound reactor)
```

The **inbound path** is:

1. The external system sends a webhook POST to the connector service.
2. The service validates the webhook signature (HMAC, OAuth, etc.).
3. The normalizer transforms the source-specific payload into a canonical
   [`Event`](../shared/api/event.go).
4. The publisher sends the event to PostgreSQL event queue on subject
   `events.<connectorName>.<eventType>`.

The **outbound path** (optional) subscribes to the NATS `AGENTS` stream and
posts reactions back to the source system when the hub routes an event to an
agent. This is how the Forgejo connector adds 👀 emoji reactions.

---

## The Canonical Event Schema

Every connector must produce events in the canonical format defined in
[`shared/api/event.go`](../shared/api/event.go). The hub and agents only
understand this format — they never see raw source payloads.

```go
type Event struct {
    ID        string       `json:"id"`
    Version   string       `json:"version"`     // always "1"
    Source    string       `json:"source"`      // e.g. "forgejo", "github", "jira"
    Connector string       `json:"connector"`   // CRD instance name
    Type      string       `json:"type"`        // e.g. "issue.opened"
    Timestamp time.Time    `json:"timestamp"`
    Subject   EventSubject `json:"subject"`     // what the event is about
    Actor     EventActor   `json:"actor"`       // who triggered it
    Action    string       `json:"action"`      // e.g. "opened", "closed"
    Data      RawJSON      `json:"data,omitempty"`  // type-specific payload
    Raw       string       `json:"raw,omitempty"`   // original webhook body
}
```

### Event types

Event types follow a dotted convention (`<kind>.<action>`):

| Group | Types |
|-------|-------|
| Issues | `issue.opened`, `issue.closed`, `issue.reopened`, `issue.assigned`, `issue.label.added`, `issue.comment.created`, `issue.edited` |
| Pull requests | `pull_request.opened`, `pull_request.closed`, `pull_request.merged`, `pull_request.comment.created`, `pull_request.review.submitted`, `pull_request.review.requested`, `pull_request.label.added`, `pull_request.synchronize`, `pull_request.edited` |
| Other | `push`, `repository.created` |

See [`shared/api/event_types.go`](../shared/api/event.go) for the full
list of constants and [`docs/event-schema.md`](event-schema.md) for field
definitions.

**Key rule:** your connector produces canonical events; the hub and agents
never see raw source payloads. This means adding a new connector requires
*no changes* to the hub or agent runtime.

---

## What You Need to Build

For each new connector source, you need:

| Artifact | Where it lives | What it does |
|----------|---------------|--------------|
| `<Source>Connector` CRD types | `shared/api/api/v1alpha1/` | Kubernetes type definition for the connector config |
| Connector service | `services/<source>-event-gateway/` | Webhook receiver, normalizer, publisher, optional reactor |
| Operator reconciliation | `operators/event-gateway/internal/controller/` | Watches the CRD, creates Deployment + Service |
| CRD YAML | `chart/crds/` | Installs the CRD into the cluster |
| Helm template | `chart/templates/connectors/` | Optional: default connector manifests |
| Event type registration | `shared/api/event_types.go` | Add `XxxEventTypes()` for your source |

For the rest of this tutorial, we'll use **Jira** as the example source.
Replace `jira` with your source name everywhere.

---

## Step 1: Define the Connector CRD

Each connector source gets its own typed CRD.
Create the types file:

**File:** `shared/api/api/v1alpha1/jiraconnector_types.go`

```go
package v1alpha1

import (
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JiraConnectorSpec defines the desired state.
type JiraConnectorSpec struct {
    // DisplayName is the user-facing name of the connector.
    DisplayName string `json:"displayName"`
    // URL is the Jira instance URL (e.g. "https://yourorg.atlassian.net").
    URL string `json:"url"`
    // WebhookEndpoint is the URL Jira sends webhooks to.
    WebhookEndpoint string `json:"webhookEndpoint"`
    // WebhookSecret for HMAC signature validation.
    WebhookSecret SecretKeyRef `json:"webhookSecret"`
    // APIToken references a Secret with the Jira API token.
    APIToken SecretKeyRef `json:"apiToken"`
    // Events to subscribe to (e.g. "jira:issue_created", "jira:issue_updated").
    Events []string `json:"events"`
    // Image for the jira-connector container.
    Image ConnectorImage `json:"image"`
    // Resources for the connector pod.
    // +optional
    Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// JiraConnectorStatus defines the observed state.
type JiraConnectorStatus struct {
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`
    // +optional
    ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// JiraConnector is the Schema for the jiraconnectors API.
type JiraConnector struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec   JiraConnectorSpec   `json:"spec,omitempty"`
    Status JiraConnectorStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// JiraConnectorList contains a list of JiraConnector.
type JiraConnectorList struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ListMeta   `json:"metadata,omitempty"`
    Items           []JiraConnector `json:"items"`
}

func init() {
    SchemeBuilder.Register(&JiraConnector{}, &JiraConnectorList{})
}
```

The CRD spec fields are source-specific. A Jira connector needs a URL and an
API token; a GitHub connector needs an App ID and private key. There is no
one-size-fits-all schema — that's the point of typed CRDs.

Generate the CRD YAML and register it:

```bash
cd shared/api
make manifests   # generates chart/crds/jiraconnector.yaml
```

Add the CRD YAML to `chart/crds/jiraconnector.yaml`.

---

## Step 2: Implement the Service

Create the connector service directory:

```
services/jira-event-gateway/
├── cmd/connector/main.go
├── internal/
│   ├── webhook/handler.go      # HTTP handler (inbound)
│   ├── normalizer/normalizer.go # source→canonical transformation
│   ├── publisher/publisher.go   # NATS publishing (reuse shared logic)
│   ├── jira/                   # source-specific API client (for reactor)
│   │   └── client.go
│   ├── reactor/reactor.go      # optional outbound reactions
│   └── metrics/metrics.go      # Prometheus counters
├── go.mod
├── go.sum
├── Dockerfile
├── Makefile
└── README.md
```

### Webhook Handler (Inbound)

The handler receives HTTP requests from your source system, validates the
signature, calls the normalizer, and publishes the event. The pattern is the
same for every connector:

1. **Validate the request** — check the HTTP method, read the body, verify the
   HMAC or signature header.
2. **Extract the event type** — from a header (e.g. `X-Forgejo-Event`,
   `X-GitHub-Event`) or from the payload itself.
3. **Normalize** — call the normalizer to convert the raw payload into a
   canonical `Event`.
4. **Publish** — send the event to the event queue.
5. **Metrics** — increment counters for received/unknown/published events.

Reference: [`services/webhook-receiver/internal/webhook/handler.go`](../services/webhook-receiver/internal/webhook/handler.go)

Key points:
- Body size limit: 1 MiB (`const maxBodySize = 1 << 20`).
- Always return 200 for valid webhook deliveries; return 401 for bad
  signatures, 400 for unknown event types.
- Log every published event with its ID and type.

### Normalizer

The normalizer is where source-specific knowledge lives. It maps the source's
webhook payload to the canonical `Event` struct.

```go
package normalizer

import (
    "encoding/json"
    "fmt"
    "time"

    ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
)

type Normalizer struct {
    connectorName string
    botUsernames  map[string]bool
}

func New(connectorName string, botUsernames []string) *Normalizer {
    bots := make(map[string]bool, len(botUsernames))
    for _, u := range botUsernames {
        bots[u] = true
    }
    return &Normalizer{connectorName: connectorName, botUsernames: bots}
}

func (n *Normalizer) Normalize(jiraEvent string, rawBody []byte) (*ainselapishared.Event, error) {
    var payload map[string]any
    if err := json.Unmarshal(rawBody, &payload); err != nil {
        return nil, fmt.Errorf("unmarshal payload: %w", err)
    }

    action := strVal(payload, "webhookEvent")  // e.g. "jira:issue_created"
    issue  := objVal(payload, "issue")
    user   := objVal(payload, "user")

    ev := &ainselapishared.Event{
        ID:        fmt.Sprintf("evt_%s_%d", n.connectorName, time.Now().UnixNano()),
        Version:   "1",
        Source:    "jira",
        Connector: n.connectorName,
        Type:      jiraToCanonicalType(action),
        Timestamp: time.Now().UTC(),
        Action:    action,
        Subject:   n.extractSubject(issue),
        Actor:     n.extractActor(user),
        Raw:       string(rawBody),
    }

    // Build type-specific data payload
    data, err := json.Marshal(n.extractData(payload))
    if err != nil {
        return nil, fmt.Errorf("marshal event data: %w", err)
    }
    ev.Data = data

    return ev, nil
}
```

Your normalizer must:
- Set `Source` to your connector type string (e.g. `"jira"`, `"slack"`).
- Set `Connector` to the CRD instance name.
- Map source-specific event types to canonical types. If a source event has
  no canonical equivalent, return an error — the handler will return 400 and
  increment the `events_unknown` counter.
- Populate `Actor.IsBot` for known bot usernames.
- Include the raw payload in `Raw` for debugging.

Reference: [`services/webhook-receiver/internal/webhook/handler.go`](../services/webhook-receiver/internal/webhook/handler.go)

### Publisher

The publisher connects to PostgreSQL event queue and publishes events on subjects
following the pattern `events.<connectorName>.<eventType>`.

The Forgejo connector's publisher is a good reference and can be reused almost
verbatim — the only thing that changes is the hub URL and stream subject
pattern, which is already parameterized via `shared/api` constants:

```go
// Stream names
StreamEvents = "EVENTS"
StreamAgents = "AGENTS"
StreamHub    = "HUB"

// Subject patterns
SubjectEventsAll = "events.>"
SubjectAgentsAll = "agent.>"

// EventsSubject builds the NATS subject
func EventsSubject(connectorName, eventType string) string {
    return "events." + connectorName + "." + eventType
}
```

Reference: [`services/webhook-receiver/internal/publisher/publisher.go`](../services/webhook-receiver/internal/publisher/publisher.go)

### Reactor (Outbound)

The reactor is *optional*. It subscribes to the `AGENTS` event queue and posts
reactions back to the source system when the hub routes an event to an agent.
The Forgejo connector's reactor adds 👀 emoji reactions as a visual signal.

If your source supports reactions or status updates, implement a reactor:
- Subscribe to `agent.>` on the `AGENTS` stream.
- Parse the event and determine if it came from your source (check
  `evt.Source`).
- Call the source's API to add a reaction or status update.
- Always ack the message, even on failure — the reactor must never block
  agent runners.

If your source doesn't support reactions, skip the reactor entirely. The
connector service runs fine without one (Forgejo detects this by checking
`FORGEJO_URL` / `FORGEJO_TOKEN` env vars at startup).

Reference: the webhook-receiver handles reactions inline in
[`services/webhook-receiver/internal/webhook/handler.go`](../services/webhook-receiver/internal/webhook/handler.go).

### Bot Detection

Bot loop prevention is a two-layer defense:

1. **Connector level:** Set `Actor.IsBot = true` in the canonical event for
   known bot usernames. The connector receives bot usernames via the
   `CONNECTOR_BOT_USERNAMES` environment variable (comma-separated).

2. **Trigger level:** The Trigger CRD has `ignoreBotEvents` (default: `true`).
   The hub skips events where `Actor.IsBot == true`.

**Your connector must:**
- Accept a `botUsernames` list (from env var or CRD spec).
- Pass it to the normalizer so it can set `Actor.IsBot`.
- Document which usernames should be listed (every agent's source-system
  account).

### Wiring `main.go`

The entry point wires everything together. Key environment variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `WEBHOOK_SECRET` | Yes | HMAC secret for webhook validation |
| `NATS_URL` | No | NATS server URL (default: `nats://nats.platform.svc.cluster.local:4222`) |
| `CONNECTOR_NAME` | No | Instance name (default: source name, e.g. `"forgejo"`) |
| `CONNECTOR_PORT` | No | HTTP listen port (default: `8080`) |
| `CONNECTOR_METRICS_PORT` | No | Prometheus metrics port (default: `9090`) |
| `CONNECTOR_BOT_USERNAMES` | No | Comma-separated bot usernames |
| Source-specific auth vars | Varies | E.g. `FORGEJO_TOKEN`, `GITHUB_APP_ID` |

Pattern (simplified):

```go
func main() {
    secret       := requireEnv("WEBHOOK_SECRET")
    natsURL      := envOrDefault("NATS_URL", "nats://nats.platform.svc.cluster.local:4222")
    connectorName := envOrDefault("CONNECTOR_NAME", "jira")
    port         := envOrDefault("CONNECTOR_PORT", "8080")
    metricsPort  := envOrDefault("CONNECTOR_METRICS_PORT", "9090")

    pub, err := publisher.New(natsURL)
    if err != nil {
        log.Fatal(err)
    }
    defer pub.Close()

    // Optional: start reactor if outbound auth is configured
    // if sourceURL != "" && sourceToken != "" { ... }

    handler := webhook.New(secret, connectorName, botUsernames, pub)

    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("ok"))
    })
    mux.Handle("/webhook", handler)

    log.Fatal(http.ListenAndServe(":"+port, mux))
}
```

Reference: [`services/webhook-receiver/cmd/connector/main.go`](../services/webhook-receiver/cmd/main.go)

---

## Step 3: Implement the Operator

The operator watches your connector CRD and reconciles a Deployment, Service,
and optionally an Ingress. The pattern is:

1. Watch `<Source>Connector` CRDs.
2. On create/update: ensure the Deployment (running your connector service
   image) and Service exist and are up-to-date.
3. On delete: Kubernetes garbage-collects owned resources via owner references.

Key reconciler logic (simplified):

```go
func (r *JiraConnectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    var connector v1alpha1.JiraConnector
    if err := r.Get(ctx, req.NamespacedName, &connector); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Create/update Deployment
    if err := r.reconcileDeployment(ctx, &connector); err != nil { ... }
    // Create/update Service
    if err := r.reconcileService(ctx, &connector); err != nil { ... }
    // Create/update ServiceMonitor for Prometheus
    if err := reconcileConnectorServiceMonitor(ctx, ...); err != nil { ... }
    // Update status
    if err := r.updateStatus(ctx, &connector); err != nil { ... }

    return ctrl.Result{}, nil
}
```

The Deployment's pod template injects:
- `WEBHOOK_SECRET` — from `spec.webhookSecret`
- `NATS_URL` — auto-computed from namespace
- `CONNECTOR_NAME` — from CRD name
- Source-specific env vars — from CRD spec

Reference: [`operators/event-gateway/internal/controller/webhookconnector_controller.go`](../operators/event-gateway/internal/controller/webhookconnector_controller.go)

Register the reconciler with the manager:

```go
func main() {
    // ... manager setup ...
    if err := (&controller.JiraConnectorReconciler{
        Client: mgr.GetClient(),
        Scheme: mgr.GetScheme(),
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "JiraConnector")
        os.Exit(1)
    }
}
```

---

## Step 4: Helm Chart Integration

### CRD

Copy the generated CRD YAML to `chart/crds/jiraconnector.yaml`.

### Connector Template (Optional)

If you want a default connector instance in the Helm chart, add it to
`chart/templates/connectors/`:

```yaml
{{- if .Values.jiraConnector.enabled }}
apiVersion: ainsel.dev/v1alpha1
kind: JiraConnector
metadata:
  name: {{ .Values.jiraConnector.name | default "jira" }}
  namespace: {{ .Release.Namespace }}
spec:
  displayName: {{ .Values.jiraConnector.displayName | default "Jira" }}
  url: {{ .Values.jiraConnector.url | required "jiraConnector.url is required" }}
  webhookEndpoint: {{ .Values.jiraConnector.webhookEndpoint | required "jiraConnector.webhookEndpoint is required" }}
  webhookSecret:
    secretRef:
      name: {{ .Values.jiraConnector.webhookSecret.name }}
      key: {{ .Values.jiraConnector.webhookSecret.key | default "secret" }}
  apiToken:
    secretRef:
      name: {{ .Values.jiraConnector.apiToken.name }}
      key: {{ .Values.jiraConnector.apiToken.key | default "token" }}
  events:
    {{- toYaml .Values.jiraConnector.events | nindent 4 }}
  image:
    repository: {{ .Values.jiraConnector.image.repository }}
    tag: {{ .Values.jiraConnector.image.tag | default .Chart.AppVersion }}
{{- end }}
```

### Values

Add default values to `chart/values.yaml`:

```yaml
jiraConnector:
  enabled: false
  name: jira
  displayName: Jira
  url: ""
  webhookEndpoint: ""
  webhookSecret:
    name: jira-webhook-hmac
    key: secret
  apiToken:
    name: jira-api-token
    key: token
  events:
    - jira:issue_created
    - jira:issue_updated
    - jira:issue_deleted
  image:
    repository: ainsel/jira-event-source-gateway
    tag: latest
```

---

## Step 5: Register Event Types

Add a `JiraEventTypes()` function to
[`shared/api/event_types.go`](../shared/api/event.go):

```go
// JiraEventTypes returns the canonical event types emitted by Jira connectors.
func JiraEventTypes() []string {
    return []string{
        EventTypeIssueOpened,
        EventTypeIssueClosed,
        EventTypeIssueCommentCreated,
        // Map Jira events to canonical types as needed
    }
}
```

Also update `EventTypesForConnector()` to handle `"jira"`:

```go
func EventTypesForConnector(connectorType string) []string {
    switch connectorType {
    case ConnectorTypeForgejo:
        return ForgejoEventTypes()
    case ConnectorTypeGitHub:
        return GitHubEventTypes()
    case ConnectorTypeJira:
        return JiraEventTypes()
    default:
        return nil
    }
}
```

And add the connector type constant:

```go
const ConnectorTypeJira = "jira"
```

---

## Testing

### Unit Tests

Each package should have co-located tests:

- `normalizer/normalizer_test.go` — test that source payloads produce correct
  canonical events. Use table-driven tests with real webhook payloads.
- `webhook/handler_test.go` — test HMAC validation, error handling, unknown
  event types.
- `reactor/reactor_test.go` — test reaction logic with a mock API client.

Reference: [`services/webhook-receiver/internal/webhook/handler.go`](../services/webhook-receiver/internal/webhook/handler.go)

### Envtest for Operator Controllers

Test the operator reconciler with `envtest` so you can assert on real
Kubernetes resources without a live cluster. Follow the pattern in
`operators/event-gateway/internal/controller/suite_test.go`:

1. **Bootstrap envtest** in `internal/controller/suite_test.go`. It locates
   the `envtest` binaries via `KUBEBUILDER_ASSETS` or the `bin/k8s/` fallback
   used by `make setup-envtest`.

2. **Write a spec** that creates the CRD, calls `Reconcile` directly, and
   asserts on the resulting Deployment, Service, Ingress, and status conditions.

```go
var _ = Describe("JiraConnector Controller", func() {
    const namespace = "default"

    ctx := context.Background()
    connectorKey := types.NamespacedName{Name: "test-jira", Namespace: namespace}
    deployKey    := types.NamespacedName{Name: "connector-test-jira", Namespace: namespace}

    It("creates a Deployment and sets Ready=True", func() {
        // Create the JiraConnector CR
        conn := &v1alpha1.JiraConnector{
            ObjectMeta: metav1.ObjectMeta{Name: "test-jira", Namespace: namespace},
            Spec: v1alpha1.JiraConnectorSpec{
                URL: "https://jira.example.com",
                WebhookSecret: v1alpha1.SecretKeyRef{
                    SecretRef: v1alpha1.SecretRef{Name: "test-secret", Key: "secret"},
                },
                Image: v1alpha1.ConnectorImage{Repository: "example.com/jira-connector", Tag: "latest"},
            },
        }
        Expect(k8sClient.Create(ctx, conn)).To(Succeed())

        // Reconcile
        r := &JiraConnectorReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
        _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: connectorKey})
        Expect(err).NotTo(HaveOccurred())

        // Assert Deployment exists with one replica
        dep := &appsv1.Deployment{}
        Expect(k8sClient.Get(ctx, deployKey, dep)).To(Succeed())
        Expect(*dep.Spec.Replicas).To(Equal(int32(1)))

        // Assert Ready condition becomes True
        Eventually(func() bool {
            conn := &v1alpha1.JiraConnector{}
            if err := k8sClient.Get(ctx, connectorKey, conn); err != nil {
                return false
            }
            for _, c := range conn.Status.Conditions {
                if c.Type == v1alpha1.ConnectorConditionReady &&
                    c.Status == metav1.ConditionTrue {
                    return true
                }
            }
            return false
        }, 10*time.Second, 100*time.Millisecond).Should(BeTrue())
    })
})
```

Run with:

```bash
make setup-envtest   # downloads k8s test binaries once
go test ./internal/controller/...
```

### HMAC Signature Test Vectors

The webhook handler validates HMAC-SHA256. Here is a concrete test vector you
can drop into `handler_test.go` and adapt for your source's header format
(e.g. `X-Hub-Signature-256`, `X-Gitea-Signature`).

**Secret:** `my-secret`  
**Payload:** `{"issue":{"key":"FOO-1"}}`  
**Expected hex:** `6a9f7e401bbd0c70c48e5bf4c86a3cb0077254c4a9745b270c48085dc0b8d377`

```go
func TestValidSignature(t *testing.T) {
    h := New("jira", "my-secret", "X-Jira-Signature", nil)

    payload := []byte(`{"issue":{"key":"FOO-1"}}`)
    mac := hmac.New(sha256.New, []byte("my-secret"))
    mac.Write(payload)
    expected := hex.EncodeToString(mac.Sum(nil))

    if !h.validSignature(payload, expected) {
        t.Fatal("raw hex signature should match")
    }
    if !h.validSignature(payload, "sha256="+expected) {
        t.Fatal("GitHub-style sha256= prefix should match")
    }
    if h.validSignature(payload, "bad-signature") {
        t.Fatal("bad signature should be rejected")
    }
}
```

Compute the expected hex quickly:

```bash
echo -n '{"issue":{"key":"FOO-1"}}' | openssl dgst -sha256 -hmac "my-secret"
```

### Integration Tests

```bash
# Start NATS locally
docker run -d --name nats -p 4222:4222 nats:2 -js

# Run with a test secret
WEBHOOK_SECRET=test NATS_URL=nats://localhost:4222 CONNECTOR_NAME=jira \
    go run ./cmd/connector

# Send a test webhook (compute the HMAC first with the snippet above)
curl -X POST http://localhost:8080/webhook \
    -H "Content-Type: application/json" \
    -H "X-Jira-Event: jira:issue_created" \
    -H "X-Jira-Signature: 6a9f7e401bbd0c70c48e5bf4c86a3cb0077254c4a9745b270c48085dc0b8d377" \
    -d '{"webhookEvent":"jira:issue_created","issue":{"key":"FOO-1"}}'

# Verify the event reached the EVENTS stream
nats consumer ls EVENTS
nats stream view EVENTS --subject events.jira.issue.opened

# If your connector has a reactor, verify on the AGENTS stream
nats consumer ls AGENTS
nats stream view AGENTS
```

### Makefile Targets

Provide at least:

```makefile
.PHONY: build test lint
build:
	go build -o bin/connector ./cmd/connector
test:
	go test ./...
lint:
	golangci-lint run
```

---

## Checklist

Before submitting your connector PR, verify:

- [ ] CRD types defined in `shared/api/api/v1alpha1/<source>connector_types.go`
- [ ] CRD YAML generated and added to `chart/crds/`
- [ ] Connector service in `services/<source>-event-gateway/` with:
  - [ ] Webhook handler with HMAC/signature validation
  - [ ] Normalizer that produces canonical `Event` structs
  - [ ] NATS publisher using `shared/api` subject constants
  - [ ] Health check endpoint at `/healthz`
  - [ ] Prometheus metrics at `:9090/metrics`
  - [ ] Optional reactor for outbound reactions
  - [ ] Bot username detection via `CONNECTOR_BOT_USERNAMES`
- [ ] Operator reconciler in `operators/event-gateway/internal/controller/`
  - [ ] Creates Deployment + Service from CRD spec
  - [ ] Injects required env vars from CRD spec
  - [ ] Updates CRD status conditions
- [ ] Event types registered in `shared/api/event_types.go`
- [ ] `go.mod` uses workspace module pattern (`go.work`)
- [ ] Unit tests for normalizer, handler, and reactor
- [ ] `README.md` for the service directory
- [ ] `Dockerfile` and `Makefile` for the service
- [ ] Helm values template (optional, for default deployment)
- [ ] CI workflow (copy from an existing connector)

---

## Reference Implementation

The Forgejo connector is the canonical reference. Key files:

| What | File |
|------|------|
| Webhook handler | [`services/webhook-receiver/internal/webhook/handler.go`](../services/webhook-receiver/internal/webhook/handler.go) |
| HMAC validation | Same as above (`validSignature` method) |
| NATS publisher | [`services/webhook-receiver/internal/publisher/publisher.go`](../services/webhook-receiver/internal/publisher/publisher.go) |
| Main entry point | [`services/webhook-receiver/cmd/main.go`](../services/webhook-receiver/cmd/main.go) |
| WebhookConnector CRD | [`shared/api/api/v1alpha1/webhookconnector_types.go`](../shared/api/api/v1alpha1/webhookconnector_types.go) |
| Operator reconciler | [`operators/event-gateway/internal/controller/webhookconnector_controller.go`](../operators/event-gateway/internal/controller/webhookconnector_controller.go) |

The GitHub connector is a second implementation that follows the same pattern
### Design decisions

| Decision | Rationale |
|----------|-----------|
| **Typed CRDs per source** | Type-safe config at admission time; source-specific validation |
| **Canonical event schema** | Hub and agents are source-agnostic; new connectors need zero changes downstream |
| **Bot-loop prevention** | Two-layer defense: connector marks bot actors, triggers skip bot events by default |
| **Ingress per connector** | Each connector exposes its own webhook endpoint; the operator creates an Ingress that rewrites the path to `/publish` or `/webhook` |
| **Owner references** | Deployment and Service are owned by the CRD; deleting the CRD garbage-collects them automatically |