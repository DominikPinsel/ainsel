#!/bin/sh
# Write /usr/share/nginx/html/runtime-config.js from env so the frontend
# discovers its OIDC issuer + client at startup. One image, deploy anywhere.
set -eu

ROOT=/usr/share/nginx/html
OUT="${ROOT}/runtime-config.js"

OIDC_ISSUER="${OIDC_ISSUER:-}"
OIDC_CLIENT_ID="${OIDC_CLIENT_ID:-}"
OIDC_PROJECT_ID="${OIDC_PROJECT_ID:-}"
FORGEJO_API_BASE="${FORGEJO_API_BASE:-}"
FORGEJO_REPO="${FORGEJO_REPO:-}"

if [ -z "$OIDC_ISSUER" ] || [ -z "$OIDC_CLIENT_ID" ] || [ -z "$OIDC_PROJECT_ID" ]; then
  echo "ainsel-hub-frontend: WARNING: OIDC_ISSUER, OIDC_CLIENT_ID, or OIDC_PROJECT_ID not set, leaving public/runtime-config.js defaults" >&2
  exit 0
fi

cat > "$OUT" <<EOF
window.__AINSEL_CONFIG__ = {
  oidcIssuer: "${OIDC_ISSUER}",
  oidcClientId: "${OIDC_CLIENT_ID}",
  oidcProjectId: "${OIDC_PROJECT_ID}",
  forgejoApiBase: "${FORGEJO_API_BASE}",
  forgejoRepo: "${FORGEJO_REPO}",
};
EOF
echo "ainsel-hub-frontend: runtime-config.js written (issuer=${OIDC_ISSUER}, clientId=${OIDC_CLIENT_ID}, projectId=${OIDC_PROJECT_ID})"
