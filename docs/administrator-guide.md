# Administrator Guide

This guide is for the people who configure AInsel for the rest of the
organization. If you're evaluating AInsel for adoption, the root
[`README.md`](../README.md) is the better starting point. If you're
extending the platform itself, see [`../CONTRIBUTING.md`](../CONTRIBUTING.md).

## 1. Why this role exists

AI is useful inside the tools your team already uses — but you can't
make every team member an AI expert, and you don't want different teams
running different models with different prompts at uncontrolled cost.
AInsel exists so one role — yours — owns the AI specifics. You pick
the model, write the persona, choose the tools, set the triggers. Your
team uses Forgejo (or whichever connector you've enabled) as they
always have; the AI shows up where it helps: a review comment, an
issue label, a draft reply.

This guide walks through the building blocks, the end-to-end journey
from a fresh deploy to a working agent, and four worked examples you
can adapt.

## 2. The building blocks

Six concepts; read them in order.

**Connector.** The bridge between AInsel and a source system. It
receives native events (Forgejo webhooks today) and turns them into
the canonical event format AInsel uses internally (see
[`event-schema.md`](event-schema.md)). You configure the source URL,
admin credentials, webhook secret, and which event types to
subscribe to. CRD: `WebhookConnector`
(see [`crd-reference.md`](crd-reference.md#webhookconnector)). A
`GitHubConnector` CRD is scaffolded but not yet end-to-end wired —
[`roadmap.md`](roadmap.md) tracks its status. For writing your own
connector, see the
[connector developer tutorial](writing-a-connector.md).

**Agent.** One AI worker, configured with a model, persona, tool
set, and runtime settings (image, resources, scaling). You typically
run several agents — one per role (reviewer, triager, implementer)
— rather than one mega-agent. Smaller scopes are easier to reason
about and cheaper to operate. CRD: `Agent`
(see [`crd-reference.md`](crd-reference.md#agent)).

**Persona.** The prompt that defines the agent's behavior — the
locus of your AI expertise. Tone, scope, refusal patterns, output
structure, things to avoid. A good persona is specific and
constraining; a bad one is vague and lets the model improvise. Lives
either inline in `Agent.spec.persona.inline` or in a ConfigMap
referenced by `Agent.spec.persona.configMapRef`. ConfigMaps are
recommended once a persona grows beyond a paragraph — you can edit
them without touching the Agent.

**Tools and skills.** What the agent can act on — Forgejo API, git,
shell, the test runner, Kubernetes, Loki. `Agent.spec.enabledTools`
explicitly opts the agent into each tool; nothing is implicit. This
limits blast radius: a code reviewer doesn't need `shell` or
`docker-builder`, so don't grant them.

**Trigger.** Says "when an event of this type matches these filters,
invoke this agent". You configure `eventType` (exact or wildcard),
`agentRef`, `connectorRef`, optional `filters` (operators: `eq`,
`neq`, `prefix`, `suffix`, `contains`, `in`, `regex`, ANDed), and
`ignoreBotEvents` (defaults to `true`). CRD: `Trigger`
(see [`crd-reference.md`](crd-reference.md#trigger)).

**CronTrigger.** A time-based trigger: "at this cron schedule, send
this prompt to this agent". Unlike a webhook `Trigger` it has no
connector — the hub runs an internal scheduler and pushes a
synthetic event to the agent's NATS subject. Use it for anything an
agent should do on a clock rather than in reaction to a webhook
(daily digests, nightly sweeps). CRD: `CronTrigger`
(see [`crd-reference.md`](crd-reference.md#crontrigger)).

**MCP servers.** Extend an agent's tool surface via the
[Model Context Protocol](https://modelcontextprotocol.io). AInsel
registers MCP servers centrally (DB-backed, managed by the hub and
proxied through [`services/mcp/`](../services/mcp/)); each agent
opts in via `Agent.spec.enabledMCPs`. The bundled MCP server today
is configured as a generic MCP server in the frontend; you can register more.

## 3. The end-to-end admin journey

Six steps from a fresh cluster to a working agent.

1. **Deploy AInsel itself.** Follow [`deployment.md`](deployment.md) to
   install the chart. By the end of this step you have the operators,
   hub, gateway, and frontend running, but no agents and no triggers
   yet.

2. **Add a connector.** Create a `WebhookConnector` CRD pointing at
   your Forgejo instance, with admin token and webhook secret in
   Secrets. The connector operator reconciles the webhook subscription
   and brings up a connector Deployment. Verify
   `kubectl get webhookconnectors -n ainsel` shows `Ready=True`.

3. **Define an agent.** Create an `Agent` CRD with a `displayName`,
   `runtime.provider`, `llm.model`, `imageRef`, a `persona` (inline or
   ConfigMap), and an `enabledTools` list. The agent operator
   reconciles a Deployment, a NATS consumer, and a KEDA ScaledObject
   if scaling is configured. Verify `kubectl get agents -n ainsel`
   shows `Ready=True`.

4. **Define a trigger.** Create a `Trigger` CRD that maps an event
   type and filters to your agent. Verify `kubectl get triggers
   -n ainsel` shows `AgentRefValid=True` and `ConnectorRefValid=True`.

5. **Observe what happens.** Take an action in Forgejo that matches
   your trigger (open a PR, file an issue, comment). The connector
   publishes the event, the hub matches it, the agent runs, and the
   result shows up as a Forgejo artifact (a comment, a label, a draft
   PR). The frontend console at `/ainsel/` lists agents, triggers,
   and recent invocations.

6. **Iterate.** Read what the agent did. Refine the persona based on
   what's wrong (too chatty, too quiet, misses things, hallucinates).
   Re-apply the ConfigMap or Agent CRD; the operator picks up changes
   on the next reconcile. Pod restart behavior on ConfigMap edits
   depends on volume mount semantics — see "Updating agents" below.

The next section walks through complete examples you can adapt.

## 4. Cookbook

Four self-contained examples, ordered simplest to most complex. Each
includes a real persona, working YAML, and iteration tips. The
personas are starting points — tune them to your team's voice and
standards before deploying.

### Example 1: AI code review on PRs

**What it does:** Posts first-pass feedback as a comment on newly
opened PRs targeting `main`. Doesn't replace human review — catches
the things humans skim past.

**Trigger:** `pull_request.opened` with `pull_request.base eq "main"`.
**Tools:** `forgejo`, `code-review`.

**Persona:**

```text
You are a code reviewer for this repository. You read the diff of a
pull request and post a single review comment summarizing what you
see.

Focus on: bugs and likely defects (off-by-one, nil derefs, races,
missing error handling); test coverage gaps relative to the changed
code; public-API changes not reflected in the docs.

Ignore: code style and formatting (the linter handles those);
subjective preferences (naming, ordering) unless they hurt
readability; anything outside the diff.

Output: one short top-level comment (2-4 sentences) summarizing the
PR, then file-level comments only on lines actually changed. Never
comment on unchanged code. Do not invent code. If the diff exceeds
500 lines, say so and stop.
```

**YAML:** (Store the persona above in a `ConfigMap` named
`code-reviewer-persona` with key `persona.md`, then apply:)

```yaml
apiVersion: ainsel.dev/v1alpha1
kind: Agent
metadata:
  name: code-reviewer
  namespace: ainsel
spec:
  displayName: "Code Reviewer"
  description: "Posts AI feedback on newly opened PRs."
  imageRef:
    name: ainsel-ai-agent-default
  runtime:
    provider: ollama-cloud
  llm:
    model: glm-5.1:cloud
    maxTurns: 15
    temperature: 0.2
  persona:
    configMapRef:
      name: code-reviewer-persona
      key: persona.md
  enabledTools:
    - forgejo
    - code-review
  scaling:
    minReplicas: 0
    maxReplicas: 3
    cooldownPeriod: 300
    lagThreshold: 5
---
apiVersion: ainsel.dev/v1alpha1
kind: Trigger
metadata:
  name: code-reviewer-on-main-prs
  namespace: ainsel
spec:
  displayName: "Code review on PRs targeting main"
  agentRef: code-reviewer
  connectorRef: forgejo
  eventType: "pull_request.opened"
  ignoreBotEvents: true
  filters:
    - field: "pull_request.base"
      op: eq
      value: "main"
```

**What end users see:** A comment on the PR within a minute or two,
signed with the agent's Forgejo identity, containing a brief summary
and inline comments on changed lines.

**Iteration tips:**

- Comments on trivial things → tighten "Focus on:" and add more
  explicit ignores.
- Hallucinates code → repeat "Only comment on lines visible in this
  diff" near the top of the persona.
- Reviews too slow → lower `llm.maxTurns` or pick a faster model.

### Example 2: Issue triage and labeling

**What it does:** Posts a triage comment and suggested labels when an
issue is opened. Prepares the issue for a human; doesn't make
decisions.

**Trigger:** `issue.opened`. **Tools:** `forgejo`.

**Persona:**

```text
You are an issue triager for this organization's repositories. When
an issue is opened you read it once and respond with one comment.

Your comment must include:

1. A one-line summary of what the user is asking for.
2. A category — exactly one of: `bug`, `feature`, `question`,
   `docs`, `unclear`.
3. A priority guess — one of: `low`, `medium`, `high`. Use `high`
   only for data loss, security, or production-broken issues.
4. Up to three suggested labels from the repo's existing label set.
   If you're not sure a label exists, don't propose it.
5. If the report is unclear or missing information, one specific
   follow-up question.

Never close, assign, or modify labels yourself — only suggest them
in the comment. Under 10 lines of prose total.
```

**YAML:** (Persona goes in a ConfigMap named `issue-triager-persona`,
key `persona.md`.)

```yaml
apiVersion: ainsel.dev/v1alpha1
kind: Agent
metadata:
  name: issue-triager
  namespace: ainsel
spec:
  displayName: "Issue Triager"
  description: "First-pass triage on newly opened issues."
  imageRef:
    name: ainsel-ai-agent-default
  runtime:
    provider: ollama-cloud
  llm:
    model: glm-5.1:cloud
    maxTurns: 10
    temperature: 0.3
  persona:
    configMapRef:
      name: issue-triager-persona
      key: persona.md
  enabledTools:
    - forgejo
  scaling:
    minReplicas: 0
    maxReplicas: 2
    cooldownPeriod: 300
    lagThreshold: 5
---
apiVersion: ainsel.dev/v1alpha1
kind: Trigger
metadata:
  name: issue-triager-on-open
  namespace: ainsel
spec:
  displayName: "Triage newly opened issues"
  agentRef: issue-triager
  connectorRef: forgejo
  eventType: "issue.opened"
  ignoreBotEvents: true
```

**What end users see:** A triage comment on their issue within a
minute, with category, priority, and label suggestions, plus a
follow-up question if information is missing.

**Iteration tips:**

- Wrong labels → list your actual label vocabulary in the persona.
- Priority too high → give concrete `high` examples ("data
  corruption", "security disclosure").
- Agent labels the issue itself → strengthen "Never modify labels
  yourself".

### Example 3: Comment Q&A

**What it does:** Replies in-thread when an issue or PR comment
mentions the agent's handle (e.g., `@docs-helper`). Not a chatbot —
silent unless called.

**Triggers:** `issue.comment.created` and `pull_request.comment.created`
(two triggers, same agent), each filtered to comments containing
the handle. **Tools:** `forgejo`.

**Persona:**

```text
You are a helper agent that answers when explicitly mentioned in an
issue or pull request comment. You speak only when called.

When you reply:
- Quote the question being asked, then answer it directly.
- Stay scoped to the topic of the issue/PR you're commenting on.
- Link only to files under `docs/`, README files, or other documentation in
  this repository — never to external sources you cannot verify.
- If you don't know the answer, say "I don't know" and stop. Never
  guess.
- Keep replies under 8 lines unless detail was explicitly requested.

Do not reply to comments that don't mention you by handle. Do not
reply more than once per thread per question. Do not take any
action other than commenting (no labels, no closes, no pushes).
```

**YAML:** (Persona goes in a ConfigMap named `docs-helper-persona`,
key `persona.md`.)

```yaml
apiVersion: ainsel.dev/v1alpha1
kind: Agent
metadata:
  name: docs-helper
  namespace: ainsel
spec:
  displayName: "Docs Helper"
  description: "Answers questions when mentioned in comments."
  imageRef:
    name: ainsel-ai-agent-default
  runtime:
    provider: ollama-cloud
  llm:
    model: glm-5.1:cloud
    maxTurns: 10
    temperature: 0.2
  persona:
    configMapRef:
      name: docs-helper-persona
      key: persona.md
  enabledTools:
    - forgejo
  scaling:
    minReplicas: 0
    maxReplicas: 2
---
apiVersion: ainsel.dev/v1alpha1
kind: Trigger
metadata:
  name: docs-helper-on-issue-comments
  namespace: ainsel
spec:
  displayName: "Reply when mentioned in issue comments"
  agentRef: docs-helper
  connectorRef: forgejo
  eventType: "issue.comment.created"
  ignoreBotEvents: true
  filters:
    - field: "comment.body"
      op: contains
      value: "@docs-helper"
---
apiVersion: ainsel.dev/v1alpha1
kind: Trigger
metadata:
  name: docs-helper-on-pr-comments
  namespace: ainsel
spec:
  displayName: "Reply when mentioned in PR comments"
  agentRef: docs-helper
  connectorRef: forgejo
  eventType: "pull_request.comment.created"
  ignoreBotEvents: true
  filters:
    - field: "comment.body"
      op: contains
      value: "@docs-helper"
```

**What end users see:** When they write `@docs-helper how do I run
the integration tests?`, a focused reply appears under the comment
within a minute or two. Comments without the handle are ignored.

**Iteration tips:**

- Replies when not mentioned → double-check the `contains` filter
  (case sensitive) and the handle string.
- Answers off-topic → add a "stay on topic" line and list refusal
  topics.
- Hallucinates doc links → require verification via the forgejo tool
  before linking.

### Example 4: Issue → PR implementation

**What it does:** When an issue gets the `ai-please` label, the
agent clones the repo, makes the change, and opens a draft PR back
for human review. Humans still review and merge.

**Trigger:** `issue.label.added` with `label.name eq "ai-please"`.
**Tools:** `forgejo`, `git`, `shell`, `test-runner`.

**Persona:**

```text
You are an implementation agent. You take a labeled issue and
produce a draft pull request that addresses it.

Workflow:
1. Read the issue title, body, and any clarifying comments.
2. Clone the repository and create a new branch named
   `ai/issue-<NUMBER>`.
3. Make the smallest possible change that addresses the issue. Stay
   strictly inside the scope. Do not refactor, do not "improve"
   unrelated code, do not change formatting outside touched lines.
4. Run the test suite if one exists (`make test`, `go test ./...`,
   `pnpm test`). If tests fail because of your change, stop and
   explain in the PR body.
5. Open a draft pull request with a descriptive title referencing
   the issue and a body listing what you changed and why.

Hard rules:
- Never merge a PR. Always open as draft.
- Never force-push. Always create a fresh branch per issue.
- If scope is unclear or the change would touch more than ~50 lines
  across more than ~3 files, stop and post a comment on the issue
  saying the scope is too large for autonomous implementation.
- Never touch CI configuration, secrets, or
  release infrastructure.
```

**YAML:** (Persona goes in a ConfigMap named `implementer-persona`,
key `persona.md`.)

```yaml
apiVersion: ainsel.dev/v1alpha1
kind: Agent
metadata:
  name: implementer
  namespace: ainsel
spec:
  displayName: "Issue Implementer"
  description: "Opens draft PRs for issues labeled ai-please."
  imageRef:
    name: ainsel-ai-agent-default
  runtime:
    provider: ollama-cloud
    resources:
      requests:
        cpu: 500m
        memory: 1Gi
      limits:
        cpu: 2000m
        memory: 4Gi
  llm:
    model: glm-5.1:cloud
    maxTurns: 50
    temperature: 0.2
  persona:
    configMapRef:
      name: implementer-persona
      key: persona.md
  enabledTools:
    - forgejo
    - git
    - shell
    - test-runner
  scaling:
    minReplicas: 0
    maxReplicas: 2
    cooldownPeriod: 600
    lagThreshold: 1
---
apiVersion: ainsel.dev/v1alpha1
kind: Trigger
metadata:
  name: implementer-on-ai-please
  namespace: ainsel
spec:
  displayName: "Implement issues labeled ai-please"
  agentRef: implementer
  connectorRef: forgejo
  eventType: "issue.label.added"
  ignoreBotEvents: true
  filters:
    - field: "label.name"
      op: eq
      value: "ai-please"
```

**What end users see:** Within minutes of applying the `ai-please`
label, a draft PR appears linked to the issue. Reviewers review and
merge as they would any human-authored draft.

**Iteration tips:**

- Scope creep → add concrete out-of-scope examples to the persona.
- Touches files it shouldn't → list paths explicitly under
  "Never touch:".
- Broken tests in PRs → raise `llm.maxTurns` or require test output
  in the PR body.

### Example 5: Scheduled prompts (cron triggers)

**What it does:** Runs an agent on a fixed schedule rather than in
reaction to a webhook. The agent receives a prompt you define and
acts on it — e.g. a daily standup digest, a nightly stale-issue
reminder, or a periodic dependency-check sweep.

A cron trigger is a self-contained schedule: it references an agent,
a 5-field cron expression, and a prompt. The hub emits a synthetic
event on the schedule and pushes it to the agent's NATS subject, so
the existing pull-based delivery model (and invocation tracking)
applies unchanged. There is no connector.

**CronTrigger:**

```yaml
apiVersion: ainsel.dev/v1alpha1
kind: CronTrigger
metadata:
  name: daily-standup-summary
  namespace: ainsel
spec:
  displayName: "Daily standup summary"
  agentRef: standup-bot
  schedule: "0 9 * * 1-5"      # 09:00 weekdays
  prompt: |
    Summarize all pull requests opened in the last 24h and any issues
    labelled "stale". Post a short digest as a comment on the team's
    standup issue (#42).
  enabled: true
```

The agent (`standup-bot`) must already exist and reference a persona
capable of the task. The `prompt` is delivered verbatim as the user
message — it is the full instruction the model sees, with no forgejo
event wrapper.

**What end users see:** Every weekday at 09:00 the bot posts a digest
comment, unprompted.

**Iteration tips:**

- Schedule fires in the **hub's local time**. Confirm the hub pod's
  timezone (default UTC) when picking times.
- Pausing without deletion → set `spec.enabled: false`.
- Backfill missed runs is intentionally not supported — each fire is
  one invocation; if the hub is down during a slot, that slot is
  skipped.
- One `CronTrigger` per prompt/agent pair. To run the same prompt on
  multiple agents, create one `CronTrigger` per agent.

## 5. Operating the platform

### Token management

Agents emit per-invocation token metrics
(`agent_tokens_used_total{agent,repo,org,event_type,token_type,model}`)
to Prometheus. The hub's observability endpoints expose a 24h
summary, a sparkline, and a per-agent / per-repo / per-model breakdown
that the frontend renders as the "Tokens last 24h" tile. So you can *see*
what's being consumed today.

What is **not yet** wired up (see [`roadmap.md`](roadmap.md)):

- **Per-agent / per-repo budget enforcement** — no policy pauses an
  agent on budget overrun. *Planned.*
- **Pre-built Grafana dashboards** — metrics are scrapeable but no
  shipped dashboards. *Planned.*
- **PrometheusRules for cost alerts** — not bundled. *Planned.*

For today: watch the observability tile and, if an agent
misbehaves, scale it to zero (below) while you fix the persona.

### Quality guardrails

Mostly persona-driven today: refusal patterns ("never close
issues"), output-format constraints, blast-radius limits. The
platform supplements this with:

- **Per-agent tool restrictions** via `Agent.spec.enabledTools` —
  a reviewer without `git` can't push. Use this aggressively.
- **`ignoreBotEvents: true`** on triggers (default) prevents agent
  loops.
- **Filter operators** on triggers scope by repo, branch, label,
  comment body, etc.

Not yet platform-enforced (see [`roadmap.md`](roadmap.md)):
**rate limiting per agent/repo** (*Planned*);
**approval workflows for destructive actions** (*Planned*).

### Updating agents

To change a persona, edit the ConfigMap and re-apply. Pods mount it
as a volume; new contents propagate within ~60 seconds (kubelet
sync period). Pods do **not** auto-restart on ConfigMap changes —
they pick up the new persona on their next invocation. For
immediate cutover, delete the agent pods and let the Deployment
recreate them.

To change the model or runtime image, edit the `Agent` CRD. The
operator updates the Deployment, which rolls. In-flight invocations
on old pods finish on old config; new invocations land on new
config. There is no zero-downtime guarantee for invocations that
span a rollout.

### Disabling an agent quickly

Three escalating options, fastest to most thorough:

1. **Disable the trigger.** Delete the `Trigger` CRD (or set its
   `eventType` to a value that never matches), and/or set any
   `CronTrigger` referencing the agent to `spec.enabled: false`.
   Reversible; in-flight invocations finish.
2. **Scale to zero.** Set `Agent.spec.scaling.maxReplicas: 0`. KEDA
   scales the Deployment to zero; events queue in the event queue until you
   scale back up.
3. **Delete the agent.** `kubectl delete agent <name> -n ainsel`.
   Tears down Deployment consumer, and ScaledObject.

For incidents, prefer (1) — reversible, leaves config in place for
analysis.

## 6. What you do NOT manage

The whole point of AInsel is to hide AI specifics from end users.
The following are administrator-only concerns:

- **End users do NOT pick which model runs.** That's
  `Agent.spec.llm.model`, configured by you.
- **End users do NOT write or see prompts.** Personas live in
  ConfigMaps you control.
- **End users do NOT choose which tools an agent has.**
  `enabledTools` is yours.
- **End users do NOT see token usage or cost.** Metrics surface on
  the admin frontend's observability page only.
- **End users do NOT interact with Kubernetes, the hub API,
  or any internal mechanism.** They use Forgejo (or whichever
  source system the connector is reading from).
- **End users may not even know they're talking to AI.** Agents
  act through normal forge artifacts — a comment, a label, a draft
  PR — under a normal-looking user identity.

If something on this list leaks to end users — a model error
message surfaces in a forge comment, a stack trace ends up in a PR
body, an internal field name appears in an issue label — that's a
bug. File it.

## Talking to AInsel via MCP

The platform's MCP server (`services/mcp/`) exposes the AInsel control
surface to any MCP-capable AI client. The intended use is to connect it
to a **local agent** — Claude Code, Claude Desktop, an IDE plugin — and
use that agent to control AInsel and ask it questions: inspect triggers
and connectors, list and update agents, read and edit personas and
skills, watch invocations, costs, logs, and platform health.

Read tools answer questions like "what workflows do I have?", "show me
the code-reviewer's persona", "how much did agents cost today?",
"what's been failing today?". Mutation tools let the agent change
platform state directly: update an agent's model, create or delete
triggers and cron triggers, create and edit personas and skills,
register agent images. Mutations act immediately against the hub (no
pending-change staging yet), so review what the agent proposes before
it calls a write tool.

See [`mcp.md`](mcp.md) for the full guide — connecting a local agent,
authentication, the complete tool reference, and example conversations.

## 7. Where to go next

- [`crd-reference.md`](crd-reference.md) — full CRD specs (`Agent`,
  `Trigger`, `WebhookConnector`, `AgentImage`). (Known drift between
  the reference and the current schema.)
- [`event-schema.md`](event-schema.md) — canonical event format and
  the full event-type list.
- [`api-reference.md`](api-reference.md) — hub REST API (useful if
  you build admin tooling on top of AInsel).
- [`deployment.md`](deployment.md) — install / upgrade the platform.
- [`adding-memory.md`](adding-memory.md) — give agents persistent memory
  with the self-hosted mem0 REST API.
- [`architecture.md`](architecture.md) — technical deep-dive.
- [`roadmap.md`](roadmap.md) — what's wired up today vs. planned.
