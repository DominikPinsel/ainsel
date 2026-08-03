# AGENTS.md

Short rule sheet for AI agents working in this repo. If you're a human, read
[`CONTRIBUTING.md`](CONTRIBUTING.md) instead.

## What this repo is

AInsel: a Kubernetes-native platform for AI agents that react to events from
code forges. Monorepo — every component lives here, deploys as a single Helm
chart into a single namespace.

Folder map (role -> path):

| Path | Role |
|---|---|
| `frontend/` | React operations console |
| `services/hub/` | Control plane (event routing, REST API) |
| `services/webhook-receiver/` | Forgejo webhook receiver / event normalizer |
| `services/mcp/` | MCP server registry |
| `operators/agent/` | K8s operator for `Agent` + `Trigger` CRDs |
| `operators/event-gateway/` | K8s operator for `WebhookConnector` CRD |
| `shared/api/` | Shared Go module (event schema, NATS constants, filter engine) |
| `chart/` | Helm chart |
| `pi/` | Pi-native agent runtime |
| `docs/` | Architecture, CRDs, deployment, conventions |

## Non-negotiable rules

- **Never commit to `main`.** Always create a branch and open a PR.
- **Conventional Commits** for every commit: `feat:`, `fix:`, `chore:`,
  `docs:`, `refactor:`, `test:`, `perf:`. Imperative, lowercase, no trailing period.
- **One topic per PR.** Don't bundle unrelated changes — open separate PRs.
- **Sync before editing:** `git fetch origin` and check for divergence on
  your branch. Multiple agents work in this repo; local state goes stale.
- **`git pull --rebase`** before push. Never force-push to shared branches.
- **Lint and test before claiming done:** `go build ./...`, `go test ./...`,
  `golangci-lint run`, `pnpm lint`, `pnpm test`, `helm lint chart/` where
  applicable.
- **Do not touch CI workflows or any `Dockerfile` without
  coordinating with the CI/Dockerfile workstream.**
- **Never read `.env` files.** Use `.env.example` for reference.

## Before you edit

1. `git fetch origin && git status` — confirm your branch is in sync with
   the remote and you're not on `main`.
2. If you're picking up an open PR, verify it's still open and unmerged
   before adding commits.
3. Run `go build ./...` and `pnpm install` once to confirm a clean baseline
   before changing anything. A failing baseline is not your bug to fix
   unless that's the task.

## Where to look for what

| If you need… | Look in… |
|---|---|
| Platform architecture, data flow, NATS streams | [`docs/architecture.md`](docs/architecture.md) |
| CRD specs (`Agent`, `Trigger`, `WebhookConnector`) | [`docs/crd-reference.md`](docs/crd-reference.md) |
| Admin-facing concepts (personas, models, tools, triggers) | [`docs/administrator-guide.md`](docs/administrator-guide.md) |
| Canonical event schema | [`docs/event-schema.md`](docs/event-schema.md) |
| Hub REST API endpoints | [`docs/api-reference.md`](docs/api-reference.md) |
| Deploying the platform | [`docs/deployment.md`](docs/deployment.md) |
| Current and planned work | [`docs/roadmap.md`](docs/roadmap.md) |
| Repo conventions (commits, branching, style) | [`docs/conventions.md`](docs/conventions.md), [`CONTRIBUTING.md`](CONTRIBUTING.md) |
| Package-specific details (how to run, env vars) | that package's `README.md` |

## Done-criteria checklist

Before reporting work as complete:

- [ ] Tests pass: `go test ./...` and `pnpm test` in any package you touched.
- [ ] Lint passes: `golangci-lint run`, `pnpm lint`.
- [ ] Build passes: `go build ./...`, `pnpm build` (frontend),
      `helm lint chart/` (if chart touched).
- [ ] Commit messages use Conventional Commits style; PR title does too.
- [ ] PR body explains *why*, not just *what*.
- [ ] Diff contains only changes relevant to the PR's one topic.
- [ ] No commented-out code, no debug prints, no `TODO` left for the
      reviewer to chase.
- [ ] Cross-links in any docs you touched still resolve to real files.
