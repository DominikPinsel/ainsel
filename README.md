# AInsel

**AInsel adds AI agents to the systems your team already uses.**

Administrators wire AI into existing tools — code forges, ticket systems,
email, anywhere events happen — by configuring agents, triggers, and
personas. End users keep using their tools as they always have, and the
AI shows up where it helps: a review comment, an issue label, a draft
reply. The AI specifics — which model, which prompts, which tools —
stay with the administrator.

## Why this matters

AI is now table stakes for engineering productivity, but operating it
well is hard. Prompt engineering, model selection, tool design, and
token budgets are full-time concerns — you can't make every team member
an AI expert, and you don't want each team improvising independently.
Costs need central management. Quality needs central guardrails.
Existing tools shouldn't have to change.

AInsel solves this by putting AI knowledge in one place: administrators
configure the agents (model, persona, tools, triggers) once; end users
keep using their existing tools and benefit from the AI through normal
artifacts. The expertise stays centralized; the value spreads
everywhere.

## Who this is for

**End users.** People already using the systems AInsel integrates into
— Forgejo today, more connectors as they land. They don't have to know
AInsel exists. They see normal artifacts: a review comment on their PR,
a label on their issue, a draft reply on a thread. They never pick a
model, never write a prompt, never see a token count.

**Administrators.** The locus of AI expertise for the organization.
They configure agents (model, persona, tools), define triggers that
decide when agents fire, and connect AInsel to the source systems
their teams use. They own cost management, quality guardrails, and
incident response. The [administrator guide](docs/administrator-guide.md)
is written for this role.

**Platform contributors.** Engineers who extend AInsel itself — writing
new connectors, adding new tools, fixing operator behavior. Start with
[`CONTRIBUTING.md`](CONTRIBUTING.md) and [`AGENTS.md`](AGENTS.md); the
per-package READMEs under `services/`, `operators/`, `shared/`,
`pi/`, `chart/`, and `frontend/` describe each component in detail.

## What you can do with it

### Today, via the webhook connector

The generic `WebhookConnector` is the only end-to-end wired source
today (configured for Forgejo). With
it you can:

- **AI code review on PRs.** An agent comments on newly opened pull
  requests with file-level feedback on the changed lines, focused on
  the criteria you configure in the persona.
- **Issue triage and labeling.** An agent classifies incoming issues,
  applies labels (priority, area, type), and optionally summarizes or
  asks clarifying questions.
- **Comment Q&A.** An agent watches issue and PR comments for mentions
  or commands and replies with focused, scoped answers — not a
  freeform chatbot.
- **Issue → PR implementation.** An agent picks up issues with a
  trigger label, branches, writes the change, and opens a PR back for
  review.

### With your own connector

The connector is the extension point. AInsel emits a canonical event
stream regardless of source (see
[`docs/event-schema.md`](docs/event-schema.md)), so anywhere events
happen — Office365 mailboxes, Jira tickets, Slack channels, a
proprietary CI system — can become an AInsel source. The agents,
triggers, and personas stay the same; only the connector changes.

A new connector is two parts: a Kubernetes operator that watches a
new connector CRD and reconciles webhook subscriptions + a Deployment,
and a service that translates the source's native event format into
the canonical event schema before POSTing to the hub. The webhook
connector under [`services/webhook-receiver/`](services/webhook-receiver/)
and [`operators/event-gateway/`](operators/event-gateway/) is the
reference implementation. Writing one is a contributor task — see
[`CONTRIBUTING.md`](CONTRIBUTING.md) and the
[connector developer tutorial](docs/writing-a-connector.md).

Office365, Jira, and Slack are illustrative possibilities, not
commitments. The roadmap tracks what's actually scheduled.

## How it works (conceptually)

A **connector** turns external events (webhooks, polled APIs) into a
canonical event stream. The **hub** reads that stream
and matches each event against admin-defined **triggers** — a trigger
says "when an event of this type matches these filters, invoke this
agent". Matched events go to **agents**, which are AI workers
configured with a model, a persona, and a set of tools. The agent
decides what to do and acts back on the source system through its
tools (commenting on a PR, labeling an issue, opening a draft).

Administrators control the agents, triggers, and personas. End users
see only the result — a comment, a label, a reply — appearing in the
tool they were already using.

## Architecture

```mermaid
graph TD
    FORGE[Forgejo Instance] -->|webhook POST| FC[services/webhook-receiver]

    subgraph "PostgreSQL Event Queue"
        EQ[(events + agent_tasks<br/>tables)]
    end

    FC -->|POST /internal/events| HUB[services/hub]
    HUB -->|insert| EQ
    EQ -->|poll + match| HUB
    HUB -->|enqueue tasks| EQ
    EQ -->|long-poll| AR1[agent runtime<br/>code-reviewer]
    EQ -->|long-poll| AR2[agent runtime<br/>issue-triager]
    AR1 -->|ACK/NACK| HUB
    AR2 -->|ACK/NACK| HUB

    AR1 -->|API calls| FORGE
    AR2 -->|API calls| FORGE

    subgraph "Kubernetes Control Plane"
        AO[operators/agent]
        CO[operators/event-gateway]
        K8s[(Kubernetes API<br/>CRDs)]
    end

    AO -->|watches Agent + Trigger| K8s
    CO -->|watches WebhookConnector| K8s
    AO -->|manages Deployments| AR1
    AO -->|manages Deployments| AR2
    CO -->|manages Deployment| FC

    UI[frontend] -->|REST API| HUB
    HUB -->|CRUD| K8s

    subgraph "Vector Database"
        QD[(qdrant)]
    end

    AR1 .-.->|store/recall| QD
    AR2 .-.->|store/recall| QD
```

For the full data flow, NATS subjects, CRD relationships, and deployment
topology, see [`docs/architecture.md`](docs/architecture.md).

## Repository layout

| Path | Language | Purpose |
|---|---|---|
| [`frontend/`](frontend/) | TypeScript | Operations console (React + Vite) |
| [`services/hub/`](services/hub/) | Go | Control plane: event routing, REST API |
| [`services/webhook-receiver/`](services/webhook-receiver/) | Go | Generic webhook receiver, event normalizer |
| [`services/mcp/`](services/mcp/) | Go | MCP server registry |
| [`operators/agent/`](operators/agent/) | Go | K8s operator for `Agent` + `Trigger` CRDs |
| [`operators/event-gateway/`](operators/event-gateway/) | Go | K8s operator for `WebhookConnector` CRD |
| [`shared/api/`](shared/api/) | Go | Shared event schema, filter engine |
| [`chart/`](chart/) | Helm | Single chart deploying the whole platform |
| [`pi/`](pi/) | JavaScript | Pi-native agent runtime |
| [`docs/`](docs/) | — | Architecture, CRDs, deployment |

Each top-level folder has its own `README.md` with package-specific details.

## Quick start

Requires **Node.js 20+**, **pnpm 9.15+**, and **Go 1.26+**.

```bash
pnpm install                  # install frontend deps (workspace root)
pnpm --filter frontend dev    # frontend dev server
go build ./...                # build every Go module in the workspace
go test ./...                 # run every Go test
```

Go modules are joined via [`go.work`](go.work). Each Go subdirectory has its
own `go.mod`; changes in `shared/api/` are picked up by downstream modules
automatically at build time.

## Where to go next

All documentation is also published at
**[dominikpinsel.github.io/ainsel](https://dominikpinsel.github.io/ainsel/)**,
in the same style as the in-app Docs page.

**For administrators:**

- [`docs/administrator-guide.md`](docs/administrator-guide.md) — concepts, end-to-end journey, cookbook
- [`docs/crd-reference.md`](docs/crd-reference.md) — CRD specs (`Agent`, `Trigger`, `WebhookConnector`)
- [`docs/deployment.md`](docs/deployment.md) — install the platform via the Helm chart

**For evaluators:**

- This README — value prop, stakeholders, use cases
- [`docs/architecture.md`](docs/architecture.md) — full technical architecture, data flow, NATS streams

**For platform contributors:**

- [`CONTRIBUTING.md`](CONTRIBUTING.md) — contributor guide (read before opening a PR)
- [`docs/writing-a-connector.md`](docs/writing-a-connector.md) — tutorial for writing a new connector end-to-end
- [`AGENTS.md`](AGENTS.md) — short rule sheet for AI agents working in this repo

**Reference:**

- [`docs/api-reference.md`](docs/api-reference.md) — hub REST API endpoints
- [`docs/mcp.md`](docs/mcp.md) — AInsel MCP server: connect a local agent to control the platform
- [`docs/event-schema.md`](docs/event-schema.md) — canonical event format
- [`docs/roadmap.md`](docs/roadmap.md) — current and planned work

## AI full disclosure

AInsel is developed with extensive AI assistance. This project is itself
a platform for AI agents, and its own development runs on them: issues are
refined, implemented, tested, and code-reviewed by AI agents working under
the rules in [`AGENTS.md`](AGENTS.md). Humans lead the ideas, architecture,
verification, and release decisions. We say this openly because it shaped
how the project was built, and because an AI platform should be honest
about using AI.

## License

This project is licensed under the [Apache License 2.0](LICENSE).
