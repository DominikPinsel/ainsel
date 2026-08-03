# Changelog

All notable changes to this project will be documented in this file.

### Features

- Warm-minimal design foundations (9d8562b)
- Redesign navigation shell (3f64b56)


### Bug Fixes

- Resolve high-severity audit findings (postcss, brace-expansion) (e3a177a)

### Release

- Frontend/v0.2.1 (e2c2993)


### Bug Fixes

- Add @types/node, --passWithNoTests, favicon (654d124)
- Request JetBrains Mono weight 300 from Google Fonts (c2779cb)
- Bake /ainsel-dev/ base path into the dev build (84918ec)
- Rewrite base path at runtime, not build time (23c06e3)
- Runtime-config.js path goes through base-path injector (ae20afb)
- Add coverage provider and --coverage flag, restore trailing newline (8e18882)
- Add coverage provider and reporters, restore trailing newline (041b67f)
- Update pnpm-lock.yaml for @vitest/coverage-v8 (42ea025)
- Lower functions coverage threshold to 68 (9050487)
- Lower functions coverage threshold to 67% after kiosk removal (a3854d6)
- Make pnpm audit blocking and fix dependency vulnerabilities (0941609)
- Patch brace-expansion CVEs via pnpm overrides (f7333d3)
- Override js-yaml to >=4.3.0 to pass pnpm audit (5069154)

### Documentation

- Add README (0ade524)

### Features

- Scaffold vite + react + ts + vitest (c7f7f13)
- Add design tokens, fonts, base stylesheet (c6c6c83)
- Add react-oidc-context + oidc-client-ts (f319443)
- Runtime-config.js for OIDC discovery (2b6fd5e)
- OIDC code+PKCE against Zitadel + chart wiring (#67) (4dbdc36)
- Plumb oidcProjectId through runtime-config (951d52c)
- Dark theme support (b29afee)
- Add floating report error button (772c9e5)
- Render mermaid diagrams in markdown content (6f50378)

### Refactoring

- Switch to docsify-style runtime-loaded markdown (83ddaee)
- Browse the repository docs/ in-app (docsify-style) (3963e5b)

### Docker

- Copy docs/ into build context for Vite ?raw imports (#366) (8686a6b)

### Release

- Frontend/v0.2.0 (749e3bd)

