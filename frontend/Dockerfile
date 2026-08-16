# Frontend image: Vite build → nginx-served static SPA with /api proxy.
#
# Single image, deployable anywhere. The Vite base path is the placeholder
# `/__BASE_PATH__/` at build time; at container startup the entrypoint
# rewrites it to the real prefix based on the BASE_PATH env var.
#
# Runtime env vars:
#   BASE_PATH      mount prefix, e.g. "/ainsel-dev" (unset/empty = root /)
#   BACKEND_HOST   hostname of the hub backend (defaults to hub-backend)
#   BACKEND_PORT   port of the hub backend (defaults to 8080)
#
# Build context = monorepo root: `docker build -f frontend/Dockerfile .`

# ---------- builder ------------------------------------------------------
FROM node:26-bookworm AS builder
WORKDIR /workspace

# Layer caching: install deps before copying source.
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY frontend/package.json ./frontend/

# corepack was removed from Node >= 25 images; install pnpm directly.
RUN npm install -g pnpm@9.15.9 \
    && pnpm install --frozen-lockfile

COPY frontend ./frontend

# Repository docs are served as static assets by the in-app Docs page.
# The repoDocsPlugin in vite.config.ts copies docs/ into dist/docs/ at
# build time so nginx serves them alongside the SPA bundle.
COPY docs ./docs

RUN pnpm --filter frontend build

# ---------- runtime ------------------------------------------------------
FROM nginx:1.31-alpine

# envsubst comes from gettext.
RUN apk add --no-cache gettext

COPY --from=builder /workspace/frontend/dist /usr/share/nginx/html
COPY frontend/nginx.conf.template /etc/nginx/templates/default.conf.template

# Defaults; override via deployment env vars.
ENV BACKEND_HOST=hub-backend \
    BACKEND_PORT=8080 \
    BASE_PATH=""

# 05: detect DNS resolver from /etc/resolv.conf (.envsh = sourced into env)
# 15: rewrite the /__BASE_PATH__/ placeholder in built assets (.sh = executed)
COPY frontend/docker-entrypoint-resolver.envsh /docker-entrypoint.d/05-detect-resolver.envsh
# 10: write /usr/share/nginx/html/runtime-config.js from OIDC_ISSUER + OIDC_CLIENT_ID env
COPY frontend/docker-entrypoint-runtime-config.sh /docker-entrypoint.d/10-write-runtime-config.sh
COPY frontend/docker-entrypoint-base-path.sh /docker-entrypoint.d/15-inject-base-path.sh

EXPOSE 80