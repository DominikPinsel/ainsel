# Skill: Component Reference

**When to use:** Whenever you need to know which verification commands to run for a given component, or where to find specific code.

---

## Monorepo Map

| Component | Path | Language | CI Workflow |
|-----------|------|----------|-------------|
| Frontend (ops console) | `frontend/` | TypeScript/React | `ci-frontend.yml` |
| Hub (control plane) | `services/hub/` | Go | `ci-services-hub.yml` |
| Webhook Receiver | `services/webhook-receiver/` | Go | `ci-services-webhook-receiver.yml` |
| MCP Server | `services/mcp/` | Go | `ci-services-mcp.yml` |
| Chat MCP | `services/chat-mcp/` | Go | `ci-services-chat-mcp.yml` |
| Agent Operator | `operators/agent/` | Go | `ci-operators-agent.yml` |
| Event Gateway Operator | `operators/event-gateway/` | Go | `ci-operators-event-gateway.yml` |
| Shared API | `shared/api/` | Go | `ci-shared-api.yml` |
| Shared Auth (OIDC) | `shared/auth/oidc/` | Go | (covered by services) |
| Helm Chart | `chart/` | YAML | `ci-chart.yml` |
| Pi Agent Runtime | `pi/` | JS/Shell | `ci-pi.yml` |

---

## Verification Commands

### Frontend (`frontend/`)

```bash
pnpm --filter frontend lint       # ESLint (0 warnings allowed)
pnpm --filter frontend test       # Vitest with coverage
pnpm --filter frontend build      # tsc + Vite production build
```

**Notes:**
- Package manager: pnpm (workspace root)
- Test framework: Vitest + Testing Library
- StrictMode is enabled — effects run twice in dev/test

### Go Services & Operators

```bash
# From the component directory (e.g., services/hub/)
go build ./...                    # Compile check
go test ./...                     # Unit tests
golangci-lint run                 # Lint (if installed)

# Or from repo root (go workspace):
go build ./services/hub/...
go test ./services/hub/...
```

**Applies to:**
- `services/hub/`
- `services/webhook-receiver/`
- `services/mcp/`
- `services/chat-mcp/`
- `operators/agent/`
- `operators/event-gateway/`
- `shared/api/`
- `shared/auth/oidc/`

**Notes:**
- Go workspace (`go.work`) — all modules resolved together
- Run `go work sync` if you add/remove a module
- Run `go mod tidy` after adding or removing imports (prevents CI drift)
- Each component has its own `go.mod`

### Helm Chart (`chart/`)

```bash
helm lint chart/                  # Lint chart structure
helm template chart/              # Render templates (catch nil pointers)
helm template chart/ -f chart/values-example.yaml   # Render with example overlay
```

**Notes:**
- CRDs live in `chart/crds/`
- Template helpers in `chart/templates/_helpers.tpl`
- Values hierarchy: `values.yaml` → `values-{size}.yaml` (or your own overlay)

### Pi Agent Runtime (`pi/`)

```bash
# No formal lint/test — verify by inspection
node --check pi/agent-shim.js     # Syntax check
shellcheck pi/entrypoint.sh       # Shell lint (if installed)
```

---

## Cross-Component Changes

If your PR touches multiple components, run verification for **all** of them:

```bash
# Example: change touches shared/api + services/hub + frontend
go build ./shared/api/... ./services/hub/...
go test ./shared/api/... ./services/hub/...
pnpm --filter frontend lint
pnpm --filter frontend test
pnpm --filter frontend build
```

---

## CI Triggers

CI only runs for a component if its paths are touched:

| Trigger paths | CI job |
|---------------|--------|
| `frontend/**`, `pnpm-lock.yaml` | `ci-frontend.yml` |
| `services/hub/**` | `ci-services-hub.yml` |
| `services/webhook-receiver/**` | `ci-services-webhook-receiver.yml` |
| `services/mcp/**` | `ci-services-mcp.yml` |
| `services/chat-mcp/**` | `ci-services-chat-mcp.yml` |
| `operators/agent/**` | `ci-operators-agent.yml` |
| `operators/event-gateway/**` | `ci-operators-event-gateway.yml` |
| `chart/**` | `ci-chart.yml` |
| `shared/api/**` | `ci-shared-api.yml` + all Go services |

**Tip:** If you change `shared/api/`, expect multiple CI jobs to trigger (all Go services depend on it).

---

## Key Files Per Component

| Component | Entry point | Tests | Config |
|-----------|-------------|-------|--------|
| Frontend | `frontend/src/main.tsx` | `frontend/src/**/*.test.tsx` | `frontend/vite.config.ts` |
| Hub | `services/hub/cmd/main.go` | `services/hub/internal/**/*_test.go` | `services/hub/internal/config/` |
| Webhook Receiver | `services/webhook-receiver/cmd/main.go` | `*_test.go` | env vars |
| Agent Operator | `operators/agent/cmd/main.go` | `*_test.go` | CRDs in `chart/crds/` |
| Event Gateway | `operators/event-gateway/cmd/main.go` | `*_test.go` | CRDs in `chart/crds/` |
| Shared API | `shared/api/` (library) | `shared/api/**/*_test.go` | — |
| Helm Chart | `chart/Chart.yaml` | `helm lint` / `helm template` | `chart/values.yaml` |
