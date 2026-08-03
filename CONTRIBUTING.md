# Contributing to AInsel

Thanks for your interest in contributing. This guide walks through setting up
the repo, the conventions we follow, and how to land a change.

If you only need a quick orientation, the top-level [`README.md`](README.md)
has the architecture diagram and folder map. AI agents working in this repo
should read [`AGENTS.md`](AGENTS.md) instead — it's the terse, pointer-heavy
version of this guide.

## Code of Conduct

Please read [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md) before participating.
The short version: be kind, assume good faith, and keep disagreements focused
on code and design rather than people.

## Reporting issues

Open an issue in the Forgejo issue tracker for the
[AInsel/ainsel](https://github.com/DominikPinsel/ainsel/issues)
repo. Useful issues include:

- What you expected to happen.
- What actually happened (logs, error messages, screenshots where relevant).
- A minimal reproduction if you have one.

For security-sensitive reports, do not open a public issue — contact the
maintainers directly.

## Development environment

You'll need:

- **Node.js 20+** — pinned in [`.nvmrc`](.nvmrc) and `package.json#engines`.
- **pnpm 9.15+** — pinned in `package.json#packageManager`.
- **Go 1.26+** — pinned in each module's `go.mod`.
- **Helm 3** — for working with the chart.
- A Kubernetes cluster if you want to deploy locally (kind, k3s, minikube).

Clone and install:

```bash
git clone https://github.com/DominikPinsel/ainsel.git
cd ainsel

pnpm install                  # frontend workspace deps
go build ./...                # builds every Go module via go.work
go test ./...                 # runs every Go test
pnpm --filter frontend dev    # frontend dev server (Vite picks the port)
```

If a build fails on a stale checkout, try `pnpm install --frozen-lockfile`
and `go mod download` from a fresh clone before assuming a real regression.

## Repository layout

This is a monorepo organized by *role*, not by language:

- `frontend/` — operations console (React + Vite + TypeScript).
- `services/` — long-running HTTP/NATS services.
  - `services/hub/` — control plane: event routing and REST API.
  - `services/webhook-receiver/` — Forgejo webhook receiver and event normalizer.
  - `services/mcp/` — MCP server registry.
- `operators/` — Kubernetes operators.
  - `operators/agent/` — reconciles `Agent` and `Trigger` CRDs.
  - `operators/event-gateway/` — reconciles `WebhookConnector` CRDs.
- `shared/api/` — Go module imported by every Go service for the canonical
  event schema, filter engine, and NATS subject constants.
- `pi/` — pi-native agent runtime.
- `chart/` — single Helm chart that deploys the whole platform.
- `docs/` — architecture, CRD reference, deployment guide, conventions.

Each top-level folder has its own `README.md` with package-specific details.
See [`docs/architecture.md`](docs/architecture.md) for the full data flow.

## Working with the Go workspace

Go modules are joined via [`go.work`](go.work). When you change a file in
`shared/api/`, downstream modules pick it up automatically at build time — no
`replace` directive needed inside the workspace.

Adding a new Go module:

```bash
mkdir services/new-thing
cd services/new-thing
go mod init github.com/DominikPinsel/ainsel/services/new-thing
cd ../..
# Add the new path under `use ( … )` in go.work.
go work sync
```

Bumping dependencies: do it per-module (`cd services/hub && go get -u …`),
then run `go work sync` from the root.

## Working with the Helm chart

```bash
helm lint chart/                            # validate templates and values
helm template chart/ -f your-values.yaml    # render manifests locally
```

The chart references image tags via `values.yaml`. Image builds and tag
updates are managed by CI.
Avoid editing the workflows or Dockerfiles without coordinating — see
[Releases](#releases) for how versions and tags are produced.

## CI runners

All Forgejo Actions jobs in this repo use the standard **`ubuntu-latest`**
runner label so that any Act runner can pick them up — no custom runner images
required.

Jobs that need a specific toolchain declare a public **`container:`** image:

- **Go jobs** (lint, test, build) use `container: node:20` (for JS action
  support) with `actions/setup-go@v5` to install Go, plus an explicit
  `golangci-lint` install step.
- **Frontend and pi jobs** (lint, test, build) use `container: node:20`.
- **Docker build jobs** (dev-image, release) run without a container so the
  runner's Docker daemon is available for `docker/build-push-action`.
- **Release-tools jobs** (chart-update, rollout, deploy) run without a
  container and install `kubectl`, `helm`, or `yq` in explicit setup steps.

When writing a new workflow, pick the appropriate public container image or
add install steps for the tools you need. Keep `runs-on: ubuntu-latest`.

## Commit conventions

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>: <description>

[optional body]
```

Types: `feat`, `fix`, `chore`, `docs`, `refactor`, `test`, `perf`.

- Lowercase type, lowercase description, no trailing period.
- Imperative mood ("add login page", not "added login page").
- Body is optional, separated by a blank line; explain *why* the change is
  needed, not just *what* changed.

The full convention lives in [`docs/conventions.md`](docs/conventions.md).

## Releases

Releases are automated per component with a release-please style flow
(release-please + release-tag CI workflows). Each push to
`main` makes the workflow compute the next version for every component from
conventional commits (via `git-cliff`) and open — or refresh — a single
release PR titled `release: <component>/vX.Y.Z` containing a generated
`CHANGELOG.md`. Merging that PR cuts the release: `release-tag` creates the
git tag, which triggers `release-<component>` to build and push
`<version>` + `latest` to Docker Hub and open a chart-update PR.

Version bumps follow the commit type: `feat` → minor,
`fix`/`perf`/`refactor`/`docs`/`test` → patch, a `BREAKING CHANGE` footer →
major; `chore`/`ci` are ignored. Components with no prior tag get an initial
`v0.1.0` release PR the first time they change on `main`.

Merge release PRs with **squash** (the norm) or a merge commit — never
rebase-merge; the post-merge guard reads the PR title from the resulting
commit subject.

## Branching

- `main` is the production branch. Merges here trigger release-please and,
  via tags, versioned image builds. **Never commit directly to `main`.**
- `dev` is the development branch. Merges here build `:dev` images and roll
  them out to the `ainsel-dev` environment. Cut `dev` from `main` and keep
  it fed with work in progress.
- Feature work happens on `type/short-description` branches
  (`feat/agent-scaling`, `fix/webhook-timeout`, `docs/architecture-update`)
  and is merged via PR.
- Base new branches on the latest `origin/main`:
  `git fetch origin && git checkout -b feat/my-thing origin/main`.
- Always `git pull --rebase` before pushing to catch remote changes.
- Delete branches after merging.

## Pull requests

- **One topic per PR.** Don't bundle unrelated changes — open separate PRs.
- Open against `main`. Squash merge is the norm.
- The PR description should explain *why*, not just *what*.
- Reference the issue you're addressing if there is one.
- Rebase your branch on `main` regularly:
  `git fetch origin && git rebase origin/main`.

Before opening a PR, run everything in [Testing & quality](#testing--quality)
locally. CI will catch regressions, but your reviewers shouldn't have to.

## Testing & quality

Run these before pushing:

```bash
go build ./...                # all Go modules compile
go test ./...                 # all Go tests pass
golangci-lint run             # Go lint clean
pnpm lint                     # frontend lint clean
pnpm test                     # frontend tests pass
pnpm build                    # frontend builds
helm lint chart/              # only if you touched the chart
```

Tests live next to source:

- Go: `*_test.go` files in the same package.
- TypeScript: `*.test.ts` or `*.test.tsx` next to source, or in a `__tests__/`
  directory.

Add or update tests for behavior you change. Especially: edge cases, error
paths, and anything that touches the canonical event schema.

## Code style

- **Go**: `gofmt`, `golangci-lint` with the config at repo root. Keep
  packages small and named for what they do. Tests live next to source.
- **TypeScript**: Prettier + ESLint with the configs at repo root. Prefer
  composition over inheritance; co-locate component, styles, and tests.
- **Markdown**: keep lines under ~100 chars where reasonable; use fenced
  code blocks with language tags; link targets relative to the file.
- **Editor**: an [`.editorconfig`](.editorconfig) at the repo root pins
  line endings, indent style, and trim-trailing-whitespace.

Full conventions in [`docs/conventions.md`](docs/conventions.md).

## Writing a connector

To add support for a new event source (Jira, Slack, Office365, etc.),
see [`docs/writing-a-connector.md`](docs/writing-a-connector.md) — a
full tutorial covering the CRD, service, operator, and chart integration.

## Where to ask for help

- [Issues](https://github.com/DominikPinsel/ainsel/issues)
  for bugs and feature requests.
- For architectural questions, start by skimming
  [`docs/architecture.md`](docs/architecture.md).

There is no chat, Discord, Matrix, or discussion forum — Forgejo issues
are the only support surface. Open an issue for anything not covered above.
