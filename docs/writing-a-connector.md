# Writing a Connector for AInsel

A connector is how an external system's happenings become events the hub can
route. The most important fact about connectors on the current platform:
**for webhook sources, a connector is a declaration, not a program.** The
platform ships one generic webhook receiver; registering a connector is a
form POST, and wiring events to agents is a trigger. You write code only for
sources that don't fit that shape — and even then, the code is a small
standalone program that posts canonical events to one HTTP endpoint.

> **Last verified against:** `develop` (commit `274bab0a`), September 2026.
> File paths and field names should match the current codebase; if they
> drift, please open an issue or PR to update this doc.

---

## Table of Contents

1. [How Events Flow](#how-events-flow)
2. [The Canonical Event](#the-canonical-event)
3. [Path A — A Webhook Source (no code)](#path-a--a-webhook-source-no-code)
4. [Path B — Anything Else (a small poster service)](#path-b--anything-else-a-small-poster-service)
5. [Filters: How Triggers Match Your Events](#filters-how-triggers-match-your-events)
6. [Testing and Debugging](#testing-and-debugging)
7. [What a Connector Is Not](#what-a-connector-is-not)
8. [Checklist](#checklist)

---

## How Events Flow

```
your source ──webhook──▶ webhook-receiver            ┌────────────┐
                       (one Deployment per           │  hub       │
                        connector, generic image) ──▶│  ingest API│──▶ events table
                          HMAC verify, wrap raw      └────────────┘        │
                                                                           ▼
      agent ◀── long-poll /next-task ◀── agent_tasks ◀── router (2s poll) ──┘
```

The full pipeline — including channels, the trigger index, cron and chat
sources that bypass the webhook path — is described in
[architecture.md](architecture.md). What matters for connector work is three
things:

1. **The receiver is generic.** `services/webhook-receiver/` verifies the
   HMAC signature, collects headers, wraps the raw body into a canonical
   `Event`, and POSTs it to the hub's `/api/internal/events`. One image,
   one Deployment per connector, materialized by the gateway operator from
   the connector's CR.
2. **Events are stored raw.** The hub does not require you to translate
   your payload into anything. Headers and body are kept verbatim; a
   matching-time view is derived (see [Filters](#filters-how-triggers-match-your-events)).
3. **The connector label routes everything.** An event's `connector` field is
   the connector's **id** (`c-…`), which is also its CR name; the display
   name you pass at creation is a label only. The router only considers
   triggers whose `connectorRef` equals it, and the hub stamps the event's
   birth [channel](architecture.md#channels) from it.

## The Canonical Event

Defined once in [`shared/api/event.go`](../shared/api/event.go):

```json
{
  "id": "uuid",
  "version": "1",
  "connector": "c-0c4b01e3",
  "timestamp": "2026-09-24T18:44:26Z",
  "headers": { "X-GitHub-Event": "issues", "User-Agent": "GitHub-Hookshot/…" },
  "data": { "action": "opened", "issue": { "number": 42, "title": "…" } },
  "raw": "<full original webhook body as a string>"
}
```

There is no `type`/`subject`/`actor` field — earlier schema versions had one;
they were removed. The event *kind* is whatever your source's event-type
header says (`X-GitHub-Event`, `X-Forgejo-Event`), and everything else lives
in the payload. `data` is optional: if your source posts JSON, the receiver
stores the body as `data` and keeps the untouched bytes in `raw`.

## Path A — A Webhook Source (no code)

**You need this path when:** the source can send HTTP webhooks and sign them
with HMAC-SHA256. That covers GitHub (`X-Hub-Signature-256`), Forgejo
(`X-Forgejo-Signature`), and most SaaS. If it signs with a scheme the
receiver cannot verify, skip to [Path B](#path-b--anything-else-a-small-poster-service).

### 1. Register the connector

Console → **Connectors → New connector**, or:

```bash
curl -X POST $HUB/api/v1/connectors \
  -H "Authorization: token …" -H "Content-Type: application/json" \
  -d '{"name": "connector-sentry", "signatureHeader": "X-Sentry-Auth-Signature"}'
```

The hub then:

- generates an HMAC secret, stores it in a Kubernetes Secret, and returns it
  **once** in the create response (`webhookSecretValue`) — put it into your
  source's webhook signing config now;
- creates the `WebhookConnector` CR
  ([CRD reference](crd-reference.md#webhookconnector)) and the gateway
  operator (`operators/event-gateway`) rolls out the per-connector receiver
  Deployment, Service and ingress path;
- hands you `webhookEndpoint` — paste it into the source's webhook settings.

Names are labels — identity is the id (`c-…`). The create response's `id`
field is what you put in `Event.connector` (Path B) and in a trigger's
`connectorRef`; the `name` you send is display-only and never routes
anything. Follow the platform naming convention for that display name:
`connector-<platform>-<scope>`, lowercase kebab.

### 2. Send a test event

Fire a real webhook (e.g. open a test issue) and confirm arrival:

- Console → **Channels** → your connector channel shows the event; the
  event view renders headers + payload;
- or `list_recent_events` / `get_channel_events` via
  [MCP](mcp.md), filtered with the `<connector>.<eventType>` subject
  pattern.

If it doesn't arrive: the receiver logs signature failures with the header
name it looked at — the three usual causes are a mismatched signing secret,
the wrong `signatureHeader` value, or a source that sends form-encoded
`payload=` bodies (the receiver unwraps that too, but some gateways strip
the signature header on redirect).

### 3. Bind agents with triggers

A trigger says: *events from this connector whose payload matches these
filters are delivered to this agent inbox.* Create it in the console or
`POST /api/v1/triggers` — filter syntax in
[Filters](#filters-how-triggers-match-your-events).

That's the whole job for Path A. No image to build, no Deployment to write,
no stream subjects to pick.

## Path B — Anything Else (a small poster service)

**You need this path when:** the source can't do signed webhooks at all
(only polling, or push with a token in the body), or signs with something
the receiver can't verify. A connector becomes *any program* that posts
canonical events to the hub's internal ingest endpoint:

```
POST /api/internal/events
X-Internal-Token: <HUB_INTERNAL_VALIDATE_SECRET>
```

The body is the [canonical Event](#the-canonical-event) JSON; `id` and
`connector` are required. The token is the shared internal secret the hub
pods already carry (`HUB_INTERNAL_VALIDATE_SECRET` — same value the
receiver uses; see the deployment values under `hub.`/secrets). Minimal
Go example:

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	ainselapishared "github.com/DominikPinsel/ainsel/shared/api"
)

func main() {
	hub, token := os.Getenv("HUB_URL"), os.Getenv("HUB_INTERNAL_VALIDATE_SECRET")
	payload, _ := os.ReadFile("poll-result.json") // whatever your source returns

	evt := ainselapishared.Event{
		ID:        newUUID(),
		Version:   "1",
		Connector: "c-4f2a91b7", // the connector's id: the `id` field of POST /api/v1/connectors
		                       // (the `name` you send is display-only — routing uses the id)
		Timestamp: time.Now().UTC(),
		Headers:   map[string]string{"type": "build"}, // the derived event kind
		Data:      json.RawMessage(payload),
		Raw:       string(payload),
	}
	body, _ := json.Marshal(evt)

	req, _ := http.NewRequestWithContext(context.Background(),
		http.MethodPost, hub+"/api/internal/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", token)
	resp, err := http.DefaultClient.Do(req)
	// 202 = stored. Retrying the same event is safe: the insert dedupes on
	// id (ON CONFLICT DO NOTHING) — so keep the id stable per real event and
	// mint a new one for genuinely new happenings.
	_, _ = resp, err
}
```

Still create a `WebhookConnector` for the label (or have your operator do
it): the connector registry is what gives the channel a name, powers the
enable/disable switch, and is what the console lists. Events ingested for an
unknown connector label are stored and provision an *orphan-flagged*
channel — visible, but a signal the registry is out of step.

If the source is purely time-based, don't build a poller at all: a
[cron trigger](cron-triggers.md) delivers a prompt to an agent on schedule
without any connector in the picture.

## Filters: How Triggers Match Your Events

Filters are `{field, op, value}` conditions evaluated against a derived
payload built at match time ([`shared/api/filter.go`](../shared/api/filter.go),
[`services/hub/internal/trigger/index.go`](../services/hub/internal/trigger/index.go)):

- **the parsed `data` object**, at the top level (`action`, `issue.title`,
  `comment.body`, …);
- **all request headers**, under `headers.` (`headers.X-GitHub-Event`, …);
- **a derived `type`**, taken from the source's event-type header —
  `X-Forgejo-Event` / `X-GitHub-Event` — so `{"field":"type","op":"eq","value":"issues"}`
  works for both forges.

| op | meaning |
|----|---------|
| `eq`, `neq` | string equality |
| `prefix`, `suffix` | string bounds |
| `contains`, `not-contains` | substring (also matches string arrays) |
| `in`, `not-in` | value in `values` list |
| `regex` | Go regexp against stringified values |

Examples that matter in practice:

```json
[
  {"field": "type",   "op": "eq",       "value": "issue_comment"},
  {"field": "action", "op": "eq",       "value": "created"},
  {"field": "comment.body", "op": "contains", "value": "@agent-name"},
  {"field": "sender.login", "op": "not-in", "values": ["bot-user"]}
]
```

(For agent-authored events, guard loops with `not-contains` on the agent's
own comment user — the hub's own chat/cron events already bypass triggers.)

## Testing and Debugging

| Symptom | First look |
|---------|------------|
| no events on the connector channel | receiver pod logs (`kubectl logs deploy/…`): `signature invalid` names the header it read; then the source's delivery log |
| events stored, agent never runs | trigger's `connectorRef` vs event's `connector` label — they must match exactly; then filter fields against the stored payload (the console event view shows the derived match payload) |
| events stored, a second channel appears named after your connector, nothing routes | you posted the display name instead of the id — `connector` must be the `c-…` id |
| agent runs on everything | missing `type`/`action` filter — bare triggers match all events from the connector |
| duplicated deliveries | your poster minted a fresh `id` per retry — the same `id` is deduped silently, so a retry must reuse it |
| orphan-flagged channel | events arrived for a connector label nobody registered — create or rename the connector |

Useful MCP tools: `get_connector`, `list_recent_events`
(subject pattern `<connector>.<eventType>`), `get_channel_events`,
`summarize_workflows` (shows triggers pointing at missing agents),
`get_agent_logs`, `list_invocations`.

## What a Connector Is Not

**Outbound reactions.** Earlier designs had connectors subscribe to an
`AGENTS` stream and post comments back. That path is gone with the NATS
queue: agents act on forges *directly*, through their MCP tools (Forgejo /
GitHub API), using the credentials on their own Agent CR. A connector is an
inbound edge only.

**A place for business logic.** Normalizing, deduplicating, or enriching
payloads before the hub sees them belongs in your poster service (Path B) —
the hub matches on what arrives, verbatim.

## Checklist

- [ ] Connector registered with `signatureHeader` set correctly (Path A) or
      a poster service posting canonical events (Path B)
- [ ] Signing secret / `X-Internal-Token` configured at the source side
- [ ] Test event visible on the connector's channel
- [ ] Triggers bound: right `connectorRef`, `type` + `action` filters,
      bot/loop guards
- [ ] Agent replies observed in the forge (or invocation review via
      `list_invocations`)
- [ ] Naming convention followed: `connector-<platform>-<scope>`
