# ainsel-pi runtime image.
#
# Layout
# ------
# Pi is installed as a vanilla npm package. Extensions are copied verbatim
# from this repo. The whole thing is wired together by
# /usr/local/bin/entrypoint.sh which exec's `pi --mode rpc` with every
# shipped extension loaded — the pi-ainsel-runner extension provides the
# NATS consume loop, hub publishing, and per-event lifecycle.

# ---------- runtime image ------------------------------------------------
FROM node:25-bookworm-slim

# OCI metadata
LABEL org.opencontainers.image.title="ainsel-pi" \
      org.opencontainers.image.description="Pi-native ainsel agent runtime — Node.js wrapper around the pi coding agent" \
      org.opencontainers.image.source="https://github.com/DominikPinsel/ainsel"

# System packages.
# - git: needed because the LLM clones repos via the ainsel git tool.
# - ca-certificates / curl: TLS roots + general HTTPS use.
# - jq: handy for entrypoint debugging; small enough to leave in.
RUN apt-get update && apt-get install -y --no-install-recommends \
        git ca-certificates curl jq python3 \
    && rm -rf /var/lib/apt/lists/*

# Pi: pinned exact version so a release of pi cannot quietly break our
# extension API contract. Bump deliberately.
ARG PI_VERSION=latest
ARG PI_OLLAMA_CLOUD_VERSION=latest
RUN npm install -g \
        "@earendil-works/pi-coding-agent@${PI_VERSION}" \
        "pi-ollama-cloud@${PI_OLLAMA_CLOUD_VERSION}"

# Ainsel-pi runtime code.
COPY agent-shim.js   /usr/local/share/ainsel-pi/agent-shim.js
COPY entrypoint.sh   /usr/local/bin/entrypoint.sh
COPY agent           /usr/local/bin/agent

# Pi extensions shipped with the runtime. Each extension dir with a
# package.json gets its own `npm install` so pi's jiti loader can
# resolve the extension's npm deps relative to its own dir.
COPY pi-extensions/  /usr/local/share/pi-extensions/
RUN for ext in /usr/local/share/pi-extensions/*; do \
        if [ -f "$ext/package.json" ]; then \
            echo "Installing deps for extension $(basename "$ext")"; \
            (cd "$ext" && npm install --omit=dev --no-audit --no-fund) || exit 1; \
        fi; \
    done

# Workspace: pi's cwd. The LLM clones repos under here via the git tool.
RUN mkdir -p /workspace && chmod 1777 /workspace
WORKDIR /workspace

# Non-root user mirrors the operator's existing pod security context.
# node:22-bookworm-slim ships a `node` user at UID 1000; drop it first so we
# can create our own `agent` user at the same UID.
RUN userdel -r node 2>/dev/null; \
    useradd -m -u 1000 -s /bin/bash agent && \
    chown -R agent:agent /workspace /usr/local/share/ainsel-pi /usr/local/share/pi-extensions
USER 1000

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
