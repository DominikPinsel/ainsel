# Observability

This document describes how to observe a running ainsel platform: where logs come from, which metrics are exported, and how the optional Loki and Prometheus backends integrate.

## Logs

All components write **structured JSON logs** using Go's `log/slog` package. Every log line is a JSON object written to stdout, which Kubernetes collects and forwards to your log aggregation backend.

### Log levels

| Level | When used |
|-------|-----------|
| `info` | Normal operation — startup, shutdown, successful events |
| `warn` | Recoverable issues — retries, degraded state |
| `error` | Failures that require attention |

### Querying logs via the hub API

When Loki is configured (see [Required vs optional backends](#required-vs-optional-backends)), logs from any component are queryable through the hub API:

```
GET /api/v1/observability/logs?app=<component>&namespace=<namespace>
```

Replace `<namespace>` with the Kubernetes namespace where ainsel is deployed.

### Component log labels

| Component | `app` label |
|-----------|-------------|
| Hub backend | `ainsel-hub` |
| Webhook receiver (connector) | `connector-<name>` |
| Agent pod | `<agent-name>` |

### Example LogQL patterns

```logql
# All hub logs in the last hour
{namespace="<namespace>", app="ainsel-hub"} | json

# Hub errors only
{namespace="<namespace>", app="ainsel-hub"} | json | level="error"

# Connector logs for a specific connector
{namespace="<namespace>", app="connector-<name>"} | json

# Agent logs for a specific agent
{namespace="<namespace>", app="<agent-name>"} | json

# Routing errors across the hub
{namespace="<namespace>", app="ainsel-hub"} | json | msg=~"routing error.*"
```

## Event detail & conversation transcript

Activity events (see [`GET /api/v1/events`](api-reference.md)) are the entry point for tracing what happened for a given event. From the **Activity** page or **Observability → Events**, every event row shows an always-visible `open →` link (and a **View full event** link when the row is expanded). Either opens the event detail view at `/observability/events/<id>`.

The event detail view lists each invocation matched to that event with its agent, trigger, status, duration, and total token usage. Below that it renders the full agent conversation transcript for the invocation: the user prompt, assistant thinking and text, tool calls, and tool results. These messages are served by [`GET /api/v1/observability/conversations`](api-reference.md).

Transcripts are populated by the agent runtime, which reports its messages back to the hub when a task completes. If an invocation has no reported messages, the event detail view says so explicitly rather than rendering an empty transcript.

## Metrics

Metrics are exposed by the hub on a dedicated metrics port (default `9090`) at `/metrics` and are proxied through the hub API at:

```
GET /api/v1/observability/metrics/summary
```

### Hub metrics

The following counters are exported by the hub. No other ainsel components export custom metrics today (see [Operator metrics](#operator-metrics) for controller-runtime defaults, and the note below for future work).

| Metric | Type | Description |
|--------|------|-------------|
| `hub_events_consumed_total` | counter | Events received from the NATS EVENTS stream |
| `hub_triggers_matched_total` | counter | Events that matched at least one trigger rule |
| `hub_events_routed_total` | counter | Events successfully dispatched to an agent |
| `hub_routing_errors_total` | counter | Events that failed to route to an agent |

> **Note:** The webhook-receiver, MCP service, and agent pods do not export custom metrics today. Adding per-component metrics is follow-up work.

### Enabling Prometheus scraping

Set `observability.prometheus.url` in `values.yaml` to the URL of your Prometheus instance. The hub uses this URL to proxy metric queries through the `/api/v1/observability/metrics/*` endpoints.

To have Prometheus scrape the hub's own `/metrics` endpoint, enable the ServiceMonitor or PodMonitor resources in `values.yaml`:

```yaml
observability:
  prometheus:
    url: "http://prometheus.monitoring.svc.cluster.local:9090"
  serviceMonitor:
    enabled: true
    labels:
      release: prometheus   # match your Prometheus Operator selector
    interval: "30s"
  podMonitor:
    enabled: false
```

## Required vs optional backends

Both Loki and Prometheus are **optional**. The platform continues to function fully without them; only the observability query endpoints are affected.

| Backend | Effect when absent |
|---------|--------------------|
| **Loki** | `GET /api/v1/observability/logs` returns an error. All other platform functionality works normally. |
| **Prometheus** | `GET /api/v1/observability/metrics/*` returns an error. All other platform functionality works normally. |

The platform health endpoint reports the status of all configured backends:

```
GET /api/v1/platform/health
```

Check this endpoint to confirm whether Loki and Prometheus are reachable before troubleshooting missing log or metric data.

## Operator metrics

Both the agent operator and the connector operator are built on [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime), which automatically exports the following metrics on port `8080` of each operator pod (path: `/metrics`):

- Reconcile duration histogram (`controller_runtime_reconcile_time_seconds`)
- Work queue depth gauge (`workqueue_depth`)
- Reconcile error counter (`controller_runtime_reconcile_errors_total`)

To scrape these, create a `ServiceMonitor` (or `PodMonitor`) pointing at the operator's metrics port. The Helm chart does not currently ship ServiceMonitor resources for the operators — this is planned as future work.

## Tracing

Distributed tracing is **not implemented**. Trace context propagation across hub, operators, and agent invocations is future work.
