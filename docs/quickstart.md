# Quick Start

AInsel adds AI agents to the systems your team already uses. Administrators configure agents (model, persona, tools, triggers) once; end users keep using their existing tools and benefit from the AI through normal artifacts — a review comment, an issue label, a draft reply.

This page is a 5-minute orientation. For the full technical reference, see the [Architecture](architecture) doc; for the admin journey with cookbook examples, see the [Administrator Guide](administrator-guide).

## What AInsel does

A connector turns webhook deliveries (from Forgejo today) into a canonical event stream. The hub matches events against triggers and routes them to agents via the event queue. Agents act on the forge — commenting, opening PRs, pushing code. Everything is a Kubernetes CRD, managed by a single Helm chart.

## The core building blocks

| Concept | What it is | Where in the UI |
|---|---|---|
| **Agent** | One AI worker: binds a persona, an image, an LLM model, and a tool set. | `/agents` |
| **Persona** | The versioned prompt that defines the agent's behaviour. | `/personas` |
| **Agent Image** | The container image with tools, env vars, and MCP servers. | `/agent-images` |
| **Connector** | Bridge between AInsel and an external system (Forgejo today). | `/connectors` |
| **Trigger** | Rule that decides when an agent fires (event type + filter). | Agent detail page |
| **Skill** | A reusable capability an agent can invoke (e.g. forgejo, git, shell). | `/skills` |

## How to get started

1. **Deploy** — Install the Helm chart into a Kubernetes namespace. See the [Deployment Guide](deployment).

2. **Connect a source** — Register a connector (Forgejo webhook today) so events start flowing. See `/connectors` in the UI.

3. **Create a persona** — Write the prompt that defines your agent's behaviour. Personas are versioned — you can roll back. See `/personas`.

4. **Build an agent image** — Define the container image with the tools, environment variables, and MCP servers your agent needs. See `/agent-images`.

5. **Create an agent** — Bind a persona, an image, a model, and a tool set together. Add triggers to decide when it fires. See `/agents`.

6. **Watch it work** — Monitor activity on the dashboard (`/dashboard`), drill into invocations (`/activity`), and check token usage and errors (`/observability`).

## Connect a local agent via MCP

AInsel exposes an MCP server so you can connect a local agent (Claude Code, Cursor, etc.) to inspect and control the platform. See the [MCP guide](mcp) for OAuth discovery URLs and a ready-to-paste config snippet.

## Where to go next

- [Architecture](architecture) — full system architecture and data flow
- [Administrator Guide](administrator-guide) — end-to-end admin journey with cookbook examples
- [CRD Reference](crd-reference) — CRD specifications
- [API Reference](api-reference) — hub REST API