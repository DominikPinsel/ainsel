# docs-site

Static publication of the repository documentation at
<https://dominikpinsel.github.io/ainsel/>, built from the same `docs/`
directory the in-app Docs page serves.

## How it works

This is a minimal Vite + React app that **reuses the frontend's `DocsPage`
component and design CSS by importing them directly** (`../frontend/src/…`).
Navigation is driven by `docs/_sidebar.md`; each page is fetched from
`docs/` at runtime (the build copies `docs/` into `dist/docs/`). Because
components and styles are shared with the frontend, the published docs look
exactly like the in-app docs and stay in sync automatically.

Routing uses `HashRouter` so links work on GitHub Pages without SPA
fallback rewrites. The Vite base path defaults to `/ainsel/` (the GitHub
Pages project subpath); override with `DOCS_SITE_BASE_PATH` when hosting
elsewhere.

## Local development

```sh
pnpm install
pnpm --filter docs-site dev      # http://localhost:5174/ainsel/
```

## Build & publish

```sh
pnpm --filter docs-site build    # output: docs-site/dist/
```

Publishing is automated: `.github/workflows/deploy-docs-pages.yml` builds
this package and deploys it to GitHub Pages on every push to `main` that
touches `docs/` or `docs-site/`.

## Adding a doc

Drop a markdown file into `docs/` and add a `- [Title](slug)` line to
`docs/_sidebar.md` — no changes here required.
