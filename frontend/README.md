# Frontend

React + Vite + TypeScript operations console for the AInsel platform.

Built with React 18, React Router 6, TanStack Query 5, and React Hook
Form + Zod. The console lists and edits `Agent`, `Trigger`, and
`WebhookConnector` resources, streams agent invocations, and surfaces
the hub's observability endpoints.

## Role in the platform

See [`../docs/architecture.md`](../docs/architecture.md).

Talks to [`../services/hub/`](../services/hub/) over its REST API
(`/api/v1/*`) for all data access. The Vite dev server proxies `/api`
to the configured hub URL; in-cluster the `Ingress` serves the SPA at
`/ainsel` and the API at `/ainsel/api`.

## Local development

```bash
pnpm install                  # from repo root (workspace install)
pnpm --filter frontend dev    # Vite dev server on :5173
```

Pointing at a local hub backend:

```bash
# Proxy /api requests through Vite to a local hub
VITE_API_TARGET=http://localhost:8080 pnpm --filter frontend dev

# Or override the API base URL the SPA fetches directly
VITE_API_URL=http://localhost:8080/api/v1 pnpm --filter frontend dev
```

`VITE_API_URL` defaults to `/api/v1` (served through the Vite proxy in
dev, through the `Ingress` rewrite in cluster). `VITE_API_TARGET`
controls the dev-only proxy target (default `http://localhost:8080`).

## Testing

```bash
pnpm --filter frontend test         # vitest (passes with no tests)
pnpm --filter frontend lint         # eslint, zero warnings
```

## Build

```bash
pnpm --filter frontend build        # tsc -b && vite build
```

Output goes to `frontend/dist/`. Production serving is done by the
nginx image in `Dockerfile`, which substitutes runtime config from
`docker-entrypoint-resolver.envsh` into `nginx.conf.template`.

## Reference

- [Hub REST API](../docs/api-reference.md)
- [Architecture overview](../docs/architecture.md)
