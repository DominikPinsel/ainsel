# Quick Start

AInsel adds AI agents to the systems your team already uses. Administrators configure agents (model, persona, tools, triggers) once; end users keep using their existing tools and benefit from the AI through normal artifacts — a review comment, an issue label, a draft reply.

This page is a 5-minute orientation. For the full technical reference, see the [Architecture](architecture) doc; for the admin journey with cookbook examples, see the [Administrator Guide](administrator-guide).

## What AInsel does

A connector turns webhook deliveries (from Forgejo or GitHub) into a canonical
event stream. The hub matches events against triggers and routes them to agents
via the event queue. Agents act on the forge — commenting, opening PRs,
pushing code. Agents, connector gateways, and runtime images are Kubernetes
CRDs managed by a single Helm chart; triggers, cron triggers, personas, skills,
MCP servers, and the channel registry live in the hub's PostgreSQL database.

## The core building blocks

| Concept | What it is | Where in the UI |
|---|---|---|
| **Agent** | One AI worker: its own persona, model, and runtime, plus per-agent overrides for tools, skills, MCP servers and environment. | **Fleet → Agents** (`/agents`) |
| **Persona** | The versioned prompt that defines an agent's behaviour. Shared templates live in the library; an agent can fork its own private copy. | **Library → Personas** (`/personas`) |
| **Agent Image** | The runtime profile: container image with tools, env vars, skills and MCP servers that agents inherit from. | **Library → Images** (`/agent-images`) |
| **Connector** | Bridge between AInsel and an external system (Forgejo today). | **Admin → Connectors** (`/connectors`) |
| **Trigger** | Rule that decides when an agent fires (event type + filter). | Agent detail page |
| **Skill** | A reusable capability an agent can invoke (e.g. forgejo, git, shell). | **Library → Skills** (`/skills`) |
| **MCP server** | A registry entry for a remote MCP server agents can connect to. | **Library → MCPs** (`/settings`) |

The sidebar mirrors this: **Fleet** holds the agents you operate, **Library** holds the shared building blocks they draw from, and **Admin** holds users, groups and connectors. Anything an agent owns outright is edited on that agent's detail tabs, not in the library.

Under **Agents**, the sidebar also lists up to five shortcuts: the agents you last opened, filled out with the most recently updated ones if you have opened fewer. That list is per account and per browser — it is not synced when you switch machines, and it only ever shows agents the hub lets you read.

## How to get started

1. **Deploy** — Install the Helm chart into a Kubernetes namespace. See the [Deployment Guide](deployment).

2. **Connect a source** — Register a connector (Forgejo webhook today) so events start flowing. See `/connectors` in the UI.

3. **Create a persona** — Write the prompt that defines your agent's behaviour. Personas are versioned — you can roll back. See `/personas`.

4. **Build an agent image** — Define the container image with the tools, environment variables, and MCP servers your agent needs. See `/agent-images`.

5. **Create an agent** — **Fleet → Agents → New Agent** walks you through five steps: identity and group, runtime image, model and provider, persona, then a review of what will be created. Each step validates on its own, so nothing is submitted half-filled.

6. **Tune the agent** — After creation, the agent's detail tabs hold what belongs to that agent alone: **Persona** (fork an agent-owned copy of a shared template), **Runtime** (switch image, override environment variables), **Tools** (tool selection and MCP servers) and **Skills**. Overrides start out inherited from the image; the first change pins them to the agent.

7. **Add triggers** — Decide when the agent fires from its **Triggers** and **Schedule** tabs.

8. **Watch it work** — Monitor activity on the dashboard (`/dashboard`), drill into invocations (`/activity`), and check token usage and errors (`/observability`).

## Connect a local agent via MCP

AInsel exposes an MCP server so you can connect a local agent (Claude Code, Cursor, etc.) to inspect and control the platform. See the [MCP guide](mcp) for OAuth discovery URLs and a ready-to-paste config snippet.

## Where to go next

- [Architecture](architecture) — full system architecture and data flow
- [Administrator Guide](administrator-guide) — end-to-end admin journey with cookbook examples
- [CRD Reference](crd-reference) — CRD specifications
- [API Reference](api-reference) — hub REST API