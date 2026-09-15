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

- **Node.js 24+** — pinned in [`.nvmrc`](.nvmrc) and `package.json#engines`.
  The floor is set by the toolchain, not by preference: jsdom 30 needs
  `^22.22.2 || ^24.15.0 || >=26.0.0` (its `undici` dependency calls
  `worker_threads.markAsUncloneable`, which Node 20 does not have) and
  mermaid 12 needs `>=22.12.0`. `frontend/Dockerfile` builds on `node:26`.
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

Everything in `.github/workflows/` runs on **GitHub Actions** with the standard
`ubuntu-latest` label, so any runner can pick the jobs up. The dev-cluster
deployment is the one exception: it lives in the `AInsel/ainsel-deployment`
repository on Forgejo, not here.

| workflow | trigger | does |
| --- | --- | --- |
| `ci-<component>.yml` (10) | PR based on `main` or `develop`, path-filtered | build, test, lint that component |
| `ci-chart.yml` | same, for `chart/**` and `operators/*/config/crd/**` | helm lint, template with default/example/medium/large values, CRD sync check |
| `dev-image-<component>.yml` (8) | push to `main` or `develop`, path-filtered | build and push images (see tags below) |
| `gitleaks.yml` | push and PR on `main`/`develop` | secret scanning |
| `deploy-docs-pages.yml` | push to `main` on docs paths | publish the docs site |
| `release.yml` | `v*.*.*` tag, or dispatch | cut a product release |

Toolchains come from `actions/setup-go@v7` and `actions/setup-node@v7` after
`actions/checkout@v7`. Docker build jobs run without a `container:` so the
runner's own Docker daemon is available to `docker/build-push-action`; the
chart jobs install Helm with the official `get-helm-3` script.

`dev-image-*` tags are deliberately asymmetric, and the guard lives in each
workflow:

- push to `develop` → `:dev` (mutable, what the dev cluster runs) and `:<short-sha>`
- push to `main` → `:<short-sha>` only

Nothing on `main` writes `:dev`. Main builds used to overwrite it on every
push, which took the dev cluster down by replacing its images with code that
lacked migrations develop had already applied - see PR #182.

## Commit conventions

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>: <description>

[optional body]
```

Types: `feat`, `fix`, `chore`, `docs`, `refactor`, `test`, `perf`, `revert`.

- Lowercase type, lowercase description, no trailing period.
- Imperative mood ("add login page", not "added login page").
- Body is optional, separated by a blank line; explain *why* the change is
  needed, not just *what* changed.

Breaking changes need a marker, because the release tooling reads it: either
append `!` to the type (`feat(api)!: drop enabledMCPs`) or add a
`BREAKING CHANGE: <what>` footer. Without one, the change still ships - it
just won't appear under *Breaking changes* in the release notes, and the
version won't bump for it.

The full convention lives in [`docs/conventions.md`](docs/conventions.md).

## Releases

One version for the whole product, cut from `main`.
`.github/workflows/release.yml` publishes everything from a single `vX.Y.Z` tag:

| artifact | where | tags |
| --- | --- | --- |
| 8 images + 2 pi variants | Docker Hub, `dpinsel/ainsel-*` | `:vX.Y.Z`, `:X.Y.Z`, and `:latest` for stable releases |
| Helm chart | GHCR, `oci://ghcr.io/dominikpinsel/charts/ainsel` | `X.Y.Z` |
| chart `.tgz` + release notes | the GitHub Release | `vX.Y.Z` |

The product is versioned as a unit because `chart/values.yaml` pins one image
tag per component: the chart cannot express per-component versions without a
redesign, so a release tags them all the same.

### Cutting a release

1. **Prepare.** Open a PR to `main` that bumps both `version` and `appVersion`
   in `chart/Chart.yaml` to the version you intend to publish. To find out what
   that should be:

   ```bash
   python3 scripts/next-version.py --explain
   ```

   It takes the highest bump among conventional commits since the last tag:
   `feat!` or a `BREAKING CHANGE` footer → major, `feat` → minor, `fix` →
   patch, `chore`/`ci`/`deps` → nothing. On a `0.x` base a breaking change
   bumps *minor*, which is what semver prescribes before 1.0; pass
   `--strict-breaking` to bump major instead. With no tag yet it returns the
   chart version, since that is the only version the project has ever carried.

2. **Tag**, once that PR is merged:

   ```bash
   git fetch origin && git checkout origin/main
   git tag -a v0.2.0 -m "Release v0.2.0" && git push origin v0.2.0
   ```

   Or run **Actions → Release → Run workflow** with `version: auto`, which
   computes the version, pushes the tag and continues in the same run. Set
   `dry_run: true` first to see every gate and the generated notes without
   publishing anything.

3. **The workflow gates before it publishes anything:**
   - the tagged commit is contained in `origin/main`
   - `chart/Chart.yaml` `version` *and* `appVersion` equal the release
   - no failed check runs on that commit
   - the tag does not already exist

   A gate failure means nothing was published, so fixing it and re-running is
   safe.

4. **Then it publishes.** The two pi variants build with
   `BASE_TAG=<release>` against the pi base built in the same run, which is what
   makes a release reproducible. A final job pulls every tag back from Docker
   Hub and GHCR and checks the Release has the chart attached, so a green run
   means the artifacts exist.

### Changelogs

Release notes are generated from conventional commits by
`scripts/release-notes.py`, grouped into breaking changes, features, fixes,
dependency updates and everything else, with each entry linked to its PR.
Commits whose subject does not match `<type>(<scope>): <subject>` land under
*Other changes* - another reason to keep the format.

```bash
python3 scripts/release-notes.py --from v0.1.0 --to v0.2.0   # between two tags
python3 scripts/release-notes.py --from -        --to v0.2.0 # everything so far
```

Prereleases (`v0.3.0-rc.1`) are supported: they are marked as such on GitHub
and skip the `:latest` tag, so `latest` always means the newest stable release.

### Releasing is not deploying

The dev cluster keeps running mutable `:dev` images built from `develop`; the
`AInsel/ainsel-deployment` workflow on Forgejo rolls them out on a schedule.
A release does not touch it. Pointing an environment at a release means setting
the chart version (or the component `image.tag` values) explicitly.

## Branching

- `main` is the release branch. Merges here build `:<short-sha>` images only,
  and releases are cut by tagging - see [Releases](#releases).
  **Never commit directly to `main`.**
- `develop` is the development branch. Pushes here build `:dev` images that
  the `ainsel-dev` environment runs. Cut `develop` from `main` and keep
  it fed with work in progress.
- Feature work happens on `type/short-description` branches
  (`feat/agent-scaling`, `fix/webhook-timeout`, `docs/architecture-update`)
  and is merged via PR into `develop`.
- Base new branches on the latest `origin/develop`:
  `git fetch origin && git checkout -b feat/my-thing origin/develop`.
- Always `git pull --rebase` before pushing to catch remote changes.
- Delete branches after merging.

## Pull requests

- **One topic per PR.** Don't bundle unrelated changes — open separate PRs.
- Open against `develop`. Squash merge is the norm - it is the only merge type
  the repository allows, so PR titles become commit subjects on both branches.
- The PR description should explain *why*, not just *what*.
- Reference the issue you're addressing if there is one.
- Rebase your branch on `develop` regularly:
  `git fetch origin && git rebase origin/develop`.
- CI runs only for PRs based on `main` or `develop`. A PR stacked on another
  feature branch gets no CI, so retarget it if you need the checks.

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
