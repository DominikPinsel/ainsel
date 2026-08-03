#!/bin/sh
# Rewrite the build-time placeholder `/__BASE_PATH__/` in all served assets to
# the real mount path provided via the BASE_PATH env var. This lets one built
# image be deployable at any subpath (or at the root) without a rebuild.
#
# BASE_PATH conventions accepted:
#   unset / ""           → mount at root, URLs become "/..."
#   "/ainsel-dev"        → URLs become "/ainsel-dev/..."
#   "ainsel-dev"         → same as above
#   "/ainsel-dev/"       → same as above (trailing slash normalized)
set -eu

ROOT=/usr/share/nginx/html
PLACEHOLDER="/__BASE_PATH__/"

# Normalize: strip both ends of slashes, then re-add for a clean replacement.
BP="${BASE_PATH:-}"
BP="${BP#/}"
BP="${BP%/}"

if [ -z "$BP" ]; then
  REPLACEMENT="/"
else
  REPLACEMENT="/${BP}/"
fi

echo "ainsel-hub-frontend: rewriting ${PLACEHOLDER} → ${REPLACEMENT}"

find "$ROOT" -type f \( -name '*.html' -o -name '*.js' -o -name '*.css' \
                       -o -name '*.svg' -o -name '*.json' \) \
  -exec sed -i "s|${PLACEHOLDER}|${REPLACEMENT}|g" {} +
