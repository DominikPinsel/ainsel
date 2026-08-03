# Adding Memory to Agents

This guide shows how to give an agent persistent memory — the ability to
store facts, decisions, and context between invocations — using
[mem0](https://github.com/mem0ai/mem0) as an external REST API that the
agent calls via `curl` from its `bash` tool.


## Prerequisites

- A running ainsel platform (operators, hub, frontend)
- The `bash` shell tool available on the agent image
- A mem0 REST API server reachable from the cluster (see below)

## Step 1 — Set up mem0

You need a mem0 instance with its REST API accessible from inside the
Kubernetes cluster. There are several options:

### Option A: mem0 Cloud

The simplest option — sign up at
[mem0.ai](https://mem0.ai), create a project, and note your API key.
The base URL is `https://api.mem0.ai`.

### Option B: Self-hosted mem0

Run the mem0 Python package as a REST API server. Install mem0 and launch
the built-in OpenAI-compatible server:

```bash
pip install mem0ai

# mem0 ships a REST API server
mem0 run
```

This starts a FastAPI server (default port 8080) with endpoints under
`/memories`. Point it at a vector store (Qdrant, pgvector, etc.) and an
LLM provider via the
[mem0 configuration](https://docs.mem0.ai/open-source/quickstart).

Deploy it as a Kubernetes Deployment + Service in your cluster, or run it
outside the cluster as long as agents can reach it.

### Option C: Any compatible API

Any service that implements the mem0 REST API contract works. The
endpoints ainsel agents in this documentation will call are:

| Method | Path | Body / Query | Purpose |
|--------|------|-------------|---------|
| `POST` | `/memories` | `messages`, `user_id`, `metadata` | Store a memory |
| `GET` | `/memories` | `?user_id=X&limit=N` | List memories |
| `POST` | `/search` | `query`, `user_id`, `top_k` | Semantic search |
| `DELETE` | `/memories/{id}` | — | Delete a memory |

### Verify connectivity

```bash
# From inside the cluster
kubectl run curl-test --image=curlimages/curl --rm -it --restart=Never -- \
  curl -s http://<your-mem0-host>:8080/memories?user_id=test
# Should return a JSON array (empty if no memories yet)
```

## Step 2 — Create a memory skill

The agent needs to know how to talk to mem0. That knowledge lives in a
**skill** — a markdown document the agent reads as instructions. The
skill below tells it where mem0 is, how to authenticate, and the exact
`curl` calls for storing, searching, listing, and deleting memories.

Create a skill named `memory-management` with the content below. How you
create it is up to you — the frontend **Skills** page, the hub Skills
API, or any tooling that writes to the hub. Those mechanisms are
documented in the [Administrator Guide](administrator-guide.md) and the
[API Reference](api-reference.md); this guide only covers *what* the
skill should contain.

````markdown
# Memory Management

You have persistent memory backed by a mem0 REST API. Use the `bash`
tool with `curl` to store and retrieve memories.

## Environment

- `$MEM0_API_URL` — base URL of the mem0 API
  (e.g. `http://mem0.ainsel-dev.svc.cluster.local:8080`). No trailing
  slash.
- `$MEM0_API_KEY` — (optional) bearer token, if the API requires authentication.
  May be empty.
- `$AGENT_NAME` — your agent name (e.g. `code-reviewer`). Use it as
  `user_id` so memories are scoped to you.

## Authentication

If `$MEM0_API_KEY` is set, pass it as a bearer token:

```bash
curl -s -H "Authorization: Bearer $MEM0_API_KEY" ...
```

If it is empty, omit the header.

## API Reference

### Store a memory

```bash
curl -s -X POST "$MEM0_API_URL/memories" \
  -H "Content-Type: application/json" \
  ${MEM0_API_KEY:+-H "Authorization: Bearer $MEM0_API_KEY"} \
  -d "{\"messages\": [{\"role\": \"user\", \"content\": \"The project uses Go 1.24 and k3s for development.\"}], \"user_id\": \"$AGENT_NAME\"}"
```

The `content` string is what mem0 extracts facts from. The LLM
distills the text into concise memory entries. The response contains
the created memory objects with their IDs.

### Search memories

```bash
curl -s -X POST "$MEM0_API_URL/search" \
  -H "Content-Type: application/json" \
  ${MEM0_API_KEY:+-H "Authorization: Bearer $MEM0_API_KEY"} \
  -d "{\"query\": \"what Go version does the project use\", \"user_id\": \"$AGENT_NAME\", \"top_k\": 5}"
```

Returns the top matching memories with relevance scores.

### List all memories

```bash
curl -s "$MEM0_API_URL/memories?user_id=$AGENT_NAME&limit=50"
```

Returns all stored memories for this agent. Use `limit` to control
page size.

### Delete a memory

```bash
curl -s -X DELETE "$MEM0_API_URL/memories/<memory_id>"
```

## When to use memory

- **At the start of a task**: Search for relevant context ("what do I
  know about this repo?").
- **During work**: Store important decisions ("I chose approach X over
  Y because Z").
- **At the end**: Store a summary of what was done and what was
  learned.
- **When you learn a fact**: Store it so future invocations do not
  have to rediscover it.

## Rules

- Always include `user_id` in every call. Without it, the API may
  return an empty list or error.
- Use semantic search (`/search`) rather than listing all memories
  when you need something specific.
- Keep memory content concise — mem0 extracts facts, so write
  naturally, not as raw JSON.
- Do not store sensitive data (tokens, passwords, secrets) in
  memories.
- Do not delete memories unless they are factually wrong. Prefer
  adding a corrective memory.

## Example workflow

```bash
# 1. Recall context at the start
curl -s -X POST "$MEM0_API_URL/search" \
  -H "Content-Type: application/json" \
  -d "{\"query\": \"previous work on the API\", \"user_id\": \"$AGENT_NAME\", \"top_k\": 5}"

# 2. Do your task (read code, write code, etc.)

# 3. Store what you learned
curl -s -X POST "$MEM0_API_URL/memories" \
  -H "Content-Type: application/json" \
  -d "{\"messages\": [{\"role\": \"user\", \"content\": \"Refactored auth middleware to use OIDC. The old session-based auth was removed in commit abc123. Tests are in auth_oidc_test.go.\"}], \"user_id\": \"$AGENT_NAME\"}"
```
````

Remember the skill id `memory-management` — you'll enable it in Step 3.

## Step 3 — Configure the AgentImage

An agent image needs three things for its pods to use memory: the
`bash` tool (to run `curl`), the `MEM0_API_URL` env var (to find
mem0), and the `memory-management` skill enabled (so the SKILL.md is
mounted into the pod). The steps below assume the frontend **Agent
Images** page; you can equally edit the image through the hub API or a
CRD — see the [Administrator Guide](administrator-guide.md) and the
[CRD Reference](crd-reference.md).

1. Open the agent image you want to add memory to, or create a new one.
2. Under **Tools**, enable the **bash** (shell) tool.
3. Under **Environment**, add:
   - `MEM0_API_URL` — the URL of your mem0 instance, e.g.
     `http://mem0.ainsel-dev.svc.cluster.local:8080`.
   - `MEM0_API_KEY` *(optional)* — only if your mem0 instance requires
     authentication. Mark it as a secret.
4. Under **Skills**, enable **memory-management**.
5. Save.

The hub writes the skill into the shared skills store and the agent
operator mounts it into the pod; the env vars are injected from the
image spec. Within roughly a minute, new agent pods will have the
skill available at
`/home/agent/.pi/agent/skills/memory-management/SKILL.md` and
`$MEM0_API_URL` in their environment.

## Step 4 — Configure the Agent

Point an agent at the memory-enabled image and, in its persona, tell it
to actually use memory — an agent won't reach for a skill it doesn't
know it should use. The steps below assume the frontend **Agents**
page; the same result can be achieved through the hub API or a CRD
(see the [Administrator Guide](administrator-guide.md) and
[CRD Reference](crd-reference.md)).

1. Open the agent you want to give memory, or create a new one.
2. Set **Image** to the agent image you configured in Step 3.
3. Set the **Persona** so the agent knows to use memory. Add lines
   like:

   ```text
   You have persistent memory. At the start of each task, search your
   memories for relevant context. At the end, store what you learned.
   Use the memory-management skill for the exact API calls.
   ```

4. Save.

Tune the persona lines to your agent's voice — the only requirement is
that they direct the agent to search and store memories through the
skill.

## Step 5 — Verify

Verify memory end-to-end through the agent's chat — no `kubectl` or
`curl` needed.

1. Open a chat session with the agent.
2. Tell it something worth remembering, for example: *"Remember that
   our production cluster is named `prod-us-east` and runs k3s 1.31."*
   The agent should call the memory skill and store the fact.
3. Start a **new** turn (or a new chat session) and ask something that
   relies on it, for example: *"What is the name of our production
   cluster?"*
4. The agent should search its memories and answer `prod-us-east`
   without you repeating it.

If it cannot recall, check that the persona (Step 4) actually tells the
agent to use memory, and that the skill is enabled and `MEM0_API_URL`
is set on the image (Step 3). The [Troubleshooting](#troubleshooting)
section has the deeper diagnostics.

## How it fits together

```
┌──────────────────────────────────────────────────────┐
│ Agent Pod                                             │
│                                                       │
│  ┌─────────────┐    ┌──────────────────────────┐     │
│  │ Pi Runtime   │    │ skills ConfigMap (mount) │     │
│  │              │    │                          │     │
│  │ reads SKILL  │←───│ memory-management/       │     │
│  │ .md          │    │   SKILL.md               │     │
│  │              │    └──────────────────────────┘     │
│  │ calls bash   │                                     │
│  │ tool         │    ┌──────────────────────────┐     │
│  │              │    │ env:                     │     │
│  └──────┬───────┘    │   MEM0_API_URL=…         │     │
│         │            │   MEM0_API_KEY=… (opt.)  │     │
│         ▼            │   AGENT_NAME=…           │     │
│  ┌─────────────┐     └──────────────────────────┘     │
│  │ bash / curl  │                                     │
│  └──────┬───────┘                                     │
└─────────┼─────────────────────────────────────────────┘
          │
          │ HTTP
          ▼
┌──────────────────┐     ┌──────────────────┐
│ mem0 REST API     │────→│ Vector store     │
│ (any provider)    │     │ (Qdrant,         │
│                   │     │  pgvector, …)    │
│ POST /memories    │     │                  │
│ GET  /memories    │     │ scoped by        │
│ POST /search      │     │  user_id =       │
│ DEL  /memories/:id│     │  agent name      │
└──────────────────┘     └──────────────────┘
```

1. The hub stores the skill in the database and renders it to the `skills`
   ConfigMap.
2. The agent operator mounts the ConfigMap into the pod and injects the
   `MEM0_API_URL` (and optionally `MEM0_API_KEY`) env vars from the
   AgentImage spec.
3. The Pi runtime discovers the SKILL.md and uses it as instructions.
4. When the agent needs to store or recall a memory, it calls `bash` with
   `curl` to hit the mem0 REST API directly.
5. mem0 stores and retrieves vectors in its configured vector store,
   scoped by `user_id` (set to the agent's name).

## Troubleshooting

### Agent doesn't use memory

- Check that the skill ID in `enabledSkills` matches the skill's `id`.
- Check that the skill was created:
  `curl -s ${HUB_URL}/api/v1/skills/memory-management`
- Check the pod has the file:
  `kubectl exec deployment/<agent> -- ls /home/agent/.pi/agent/skills/`
- Make sure the persona mentions memory — the agent won't use a skill it
  doesn't know it should use.

### curl fails with connection error

- Check the env var:
  `kubectl exec deployment/<agent> -- printenv MEM0_API_URL`
- Check the mem0 server is reachable from inside the cluster:
  `kubectl run curl-test --image=curlimages/curl --rm -it --restart=Never -- curl -sv http://<your-mem0-host>:8080/memories`
- If using mem0 Cloud, verify the API key is set and the URL is correct.

### Memories are empty even though the agent ran

- Make sure `user_id` is included in the API call. Without it, mem0 may
  return an empty list.
- Check which user_id the agent is using:
  `kubectl exec deployment/<agent> -- printenv AGENT_NAME`
- Query with the right user_id:
  `curl -s "http://<mem0-host>:8080/memories?user_id=<agent-name>"`

### Authentication errors (401 / 403)

- If using mem0 Cloud or an authenticated instance, make sure `MEM0_API_KEY`
  is set on the AgentImage as a secret env var.
- Check that the key hasn't expired.

## See also

- [Administrator Guide](administrator-guide.md) — agent, trigger, and MCP
  server configuration
- [CRD Reference](crd-reference.md) — Agent and AgentImage spec fields
- [mem0 documentation](https://docs.mem0.ai) — upstream mem0 setup,
  configuration, and API reference