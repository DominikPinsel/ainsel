# AInsel Conventions

Repo-wide conventions for commits, branches, code style, and PRs. Tooling-specific config (CI workflows, lint configs, Dockerfiles) lives next to its tool.

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

### Versioning

The repository uses [Semantic Versioning](https://semver.org/):

```
v<major>.<minor>.<patch>
```

- **Major** — breaking API/CRD changes
- **Minor** — new features, backward compatible
- **Patch** — bug fixes, documentation

Tags always include the `v` prefix: `v1.0.0`, `v0.2.1`.

### Branching

- `main` is the default branch. Never commit directly.
- Feature work happens on `<type>/<short-description>` branches
  (e.g., `feat/github-connector`, `fix/webhook-timeout`,
  `docs/architecture-update`).
- Open PRs against `main`. Squash merge is the norm.
- Always `git pull --rebase` before pushing.

### Pull Requests

- One topic per PR. Don't bundle unrelated changes — open separate PRs.
- The PR description should explain *why*, not just *what*.
- Run lints and tests locally before opening (see [Code style](#3-code-style)).

### PR Labels

The release tooling expects these labels to exist; CI creates them
automatically if missing:

| Label | Color | Purpose |
|-------|-------|---------|
| `release` | `#0e8a16` (green) | Applied to release PRs |
| `automated` | `#1d76db` (blue) | Applied to PRs opened by CI |
| `dependencies` | `#0366d6` (blue) | Applied to chart/image bump PRs |

---

## 2. Documentation Conventions

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

## 3. Code Style

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


