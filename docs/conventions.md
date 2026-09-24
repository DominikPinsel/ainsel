# AInsel Conventions

Repo-wide conventions for commits, branches, entity naming, code style, and PRs. Tooling-specific config (CI workflows, lint configs, Dockerfiles) lives next to its tool.

---

## 1. Git Conventions

### Commit Messages

All commits follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/):

```
<type>: <description>

[optional body]
```

**Types:**

- `feat:` — new feature or functionality
- `fix:` — bug fix
- `chore:` — maintenance, dependency updates, CI changes
- `docs:` — documentation only
- `refactor:` — code restructuring without behavior change
- `test:` — adding or updating tests
- `perf:` — performance improvement

**Rules:**

- Lowercase type prefix, no capital first letter in description
- Imperative mood: "add login page", not "added login page"
- No period at the end
- Body is optional, separated by a blank line; use it to explain the *why*

Breaking changes need a marker: append `!` to the type
(`feat(api)!: drop enabledMCPs`) or add a `BREAKING CHANGE: <what>` footer.

**Why the format matters mechanically:** [release-please](https://github.com/googleapis/release-please)
parses these commits on `main` to compute the next version and to write
`CHANGELOG.md`. The subject becomes the changelog entry verbatim, so write it
for someone reading a changelog, not for someone reviewing a diff. A commit
whose subject does not parse is left out of the changelog and does not bump the
version.

### Versioning

The repository uses [Semantic Versioning](https://semver.org/):

```
v<major>.<minor>.<patch>
```

Versions are computed by release-please from conventional commits, not chosen
by hand. `.release-please-manifest.json` records the last released version and
`release-please-config.json` sets the rules:

- **Major** — breaking changes (a `!` marker or a `BREAKING CHANGE:` footer),
  once the project reaches `1.0.0`
- **Minor** — `feat:` commits. While the version is below `1.0.0`,
  `bump-minor-pre-major` maps breaking changes here too, which is what semver
  prescribes before a stable API is promised
- **Patch** — `fix:` commits

Tags always include the `v` prefix and carry no component name (`v1.0.0`, not
`ainsel-v1.0.0`), because one version covers the whole product: the Helm chart
pins a single image tag per component.

### Branching

- `main` is the default branch. Never commit directly.
- Feature work happens on `<type>/<short-description>` branches
  (e.g., `feat/github-connector`, `fix/webhook-timeout`,
  `docs/architecture-update`).
- Open PRs against `develop` and squash-merge them, so the PR title becomes
  the commit subject.
- **Promote `develop` into `main` with a merge commit, never a squash.**
  release-please derives the version and `CHANGELOG.md` from the conventional
  commits reachable on `main`. A squash promotion collapses all of them into a
  single `release:` commit, which is not a conventional type, so the changelog
  would omit everything the release actually ships.
- Always `git pull --rebase` before pushing.

### Pull Requests

- One topic per PR. Don't bundle unrelated changes — open separate PRs.
- The PR description should explain *why*, not just *what*.
- Run lints and tests locally before opening (see [Code style](#4-code-style)).

### PR Labels

Labels are a human convention here - no workflow reads them, and nothing
creates them automatically. Only `dependencies` exists in the repository today
(Dependabot applies it to its own PRs); add the others with `gh label create`
if you want them.

| Label | Color | Purpose |
|-------|-------|---------|
| `dependencies` | `#0366d6` (blue) | Dependency bumps (applied by Dependabot) |

---

## 2. Naming Conventions

Every named entity — agents, connectors, channels, triggers, cron triggers,
personas, agent images — follows the platform naming scheme: kind prefix,
kebab-case, role word, fixed scope tokens. See
[Naming Convention](naming.md) for the rules, the scope vocabulary, and how
to rename things safely.

## 3. Documentation Conventions

- Every top-level package has a `README.md`. Follow the per-package template
  described in [`CONTRIBUTING.md`](../CONTRIBUTING.md).
- Cross-package links use relative folder paths (e.g., `../services/hub/`),
  not external URLs.
- Architecture lives in [`docs/architecture.md`](architecture.md), not in
  per-package READMEs. Per-package READMEs may include a *focused* internal
  diagram if it helps, but the system-level view stays in one place.
- API/CRD reference content lives in [`docs/crd-reference.md`](crd-reference.md)
  and [`docs/api-reference.md`](api-reference.md). Per-package docs link
  outward instead of duplicating.
- Markdown lines: aim for ~100 chars where reasonable. Use fenced code blocks
  with language tags.

---

## 4. Code Style

### Go

- `gofmt` is mandatory; CI fails on unformatted code.
- `golangci-lint` runs with the config at the repo root.
- Tests live next to source as `*_test.go`. Prefer table-driven tests.
- `go.work` joins every Go module; when you change a file in `shared/api/`,
  downstream modules pick it up automatically — no `replace` directives
  needed inside the workspace.

### TypeScript / Frontend

- Prettier + ESLint, configs at the repo root.
- Tests live next to source as `*.test.ts(x)` or under `__tests__/`.
- Frontend code lives under [`frontend/`](../frontend/); follow its package
  README for build/dev commands.

### Helm

- `helm lint chart/` must pass.
- `helm template chart/ -f your-values.yaml` should render without errors.
- Keep values keys lowercase-camelCase to match the rest of the chart.


