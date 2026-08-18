#!/bin/sh
# Write /usr/share/nginx/html/runtime-config.js from env so the frontend
# discovers its auth mode + OIDC settings at startup. One image, deploy
# anywhere.
#
# AUTH_MODE controls what the SPA does at load time:
#   oidc  - OIDC redirect flow (requires OIDC_ISSUER/CLIENT_ID/PROJECT_ID)
#   local - hub username/password login
#   none  - no auth (port-forward or edge-proxy installs)
#
# When AUTH_MODE is empty, behavior is backward compatible: config is only
# written when all three OIDC vars are set, otherwise the baked defaults
# (all-empty = no-auth) stay in place.
set -eu

ROOT=/usr/share/nginx/html
OUT="${ROOT}/runtime-config.js"

AUTH_MODE="${AUTH_MODE:-}"
OIDC_ISSUER="${OIDC_ISSUER:-}"
OIDC_CLIENT_ID="${OIDC_CLIENT_ID:-}"
OIDC_PROJECT_ID="${OIDC_PROJECT_ID:-}"
FORGEJO_API_BASE="${FORGEJO_API_BASE:-}"
FORGEJO_REPO="${FORGEJO_REPO:-}"

write_config() {
  auth_mode="$1"
  cat > "$OUT" <<EOF
window.__AINSEL_CONFIG__ = {
  authMode: "${auth_mode}",
  oidcIssuer: "${OIDC_ISSUER}",
  oidcClientId: "${OIDC_CLIENT_ID}",
  oidcProjectId: "${OIDC_PROJECT_ID}",
  forgejoApiBase: "${FORGEJO_API_BASE}",
  forgejoRepo: "${FORGEJO_REPO}",
};
EOF
}

case "$AUTH_MODE" in
  local|none)
    write_config "$AUTH_MODE"
    echo "ainsel-hub-frontend: runtime-config.js written (authMode=${AUTH_MODE})"
    exit 0
    ;;
  oidc)
    if [ -z "$OIDC_ISSUER" ] || [ -z "$OIDC_CLIENT_ID" ] || [ -z "$OIDC_PROJECT_ID" ]; then
      echo "ainsel-hub-frontend: ERROR: AUTH_MODE=oidc requires OIDC_ISSUER, OIDC_CLIENT_ID, and OIDC_PROJECT_ID" >&2
      exit 1
    fi
    write_config "oidc"
    echo "ainsel-hub-frontend: runtime-config.js written (authMode=oidc, issuer=${OIDC_ISSUER})"
    exit 0
    ;;
  "")
    # Backward-compatible path: write config only when OIDC is fully set.
    if [ -z "$OIDC_ISSUER" ] || [ -z "$OIDC_CLIENT_ID" ] || [ -z "$OIDC_PROJECT_ID" ]; then
      echo "ainsel-hub-frontend: WARNING: AUTH_MODE not set and OIDC vars incomplete; keeping baked defaults (no-auth)" >&2
      exit 0
    fi
    write_config "oidc"
    echo "ainsel-hub-frontend: runtime-config.js written (authMode=oidc, issuer=${OIDC_ISSUER})"
    exit 0
    ;;
  *)
    echo "ainsel-hub-frontend: ERROR: invalid AUTH_MODE '${AUTH_MODE}' (want oidc, local, or none)" >&2
    exit 1
    ;;
esac
