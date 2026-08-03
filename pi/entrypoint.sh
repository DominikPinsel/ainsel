#!/bin/sh
# ainsel-pi container entrypoint.
#
# Two responsibilities:
#   1. Honour the `agent --list-tools` contract so hub-backend's
#      AgentImage tool-sync Job can introspect this image. The Job sets
#      `command: ["agent", "--list-tools"]`, which bypasses ENTRYPOINT,
#      so this file does NOT see that invocation directly — see also
#      /usr/local/bin/agent which routes the same way for parity.
#   2. Bootstrap git credentials from the operator-injected Forgejo
#      token, so the LLM can `git clone <url>` via the ainsel `git`
#      tool without ever handling secrets.
#
# Everything else (the event loop, NATS consumption, hub publishing,
# pi lifecycle) lives in the pi-ainsel-runner extension. This script
# just configures the environment and exec's pi in RPC mode.

set -eu

# Dispatch for any caller that runs entrypoint.sh with arguments. The
# canonical caller (Kubernetes) overrides this entirely with `command:`.
case "${1:-}" in
  --list-tools|list-tools)
    exec /usr/local/bin/agent --list-tools
    ;;
esac

# Bootstrap git credentials (best-effort, never fails the container).
bootstrap_git_credentials() {
  # Single-connector path: FORGEJO_URL + FORGEJO_TOKEN env vars.
  if [ -n "${FORGEJO_URL:-}" ] && [ -n "${FORGEJO_TOKEN:-}" ]; then
    HOST=$(printf '%s\n' "$FORGEJO_URL" | sed -E 's|^https?://||; s|/.*$||')
    if [ -n "$HOST" ]; then
      mkdir -p "$HOME"
      printf 'https://x-access-token:%s@%s\n' "$FORGEJO_TOKEN" "$HOST" > "$HOME/.git-credentials"
      chmod 600 "$HOME/.git-credentials"
      git config --global credential.helper store >/dev/null 2>&1 || true
      echo "entrypoint: configured git credential helper for $HOST" >&2
    fi
  fi

  # Multi-connector path: per-connector files mounted under
  # /etc/forge-credentials/<connector>/{url,token}. We append a line per
  # connector so `git clone https://<host>/...` Just Works.
  CREDS_DIR=/etc/forge-credentials
  URLS_DIR=/etc/forgejo-urls
  if [ -d "$CREDS_DIR" ] && [ -d "$URLS_DIR" ]; then
    git config --global credential.helper store >/dev/null 2>&1 || true
    : > "$HOME/.git-credentials"
    chmod 600 "$HOME/.git-credentials"
    for connector in "$URLS_DIR"/*; do
      [ -e "$connector" ] || continue
      name=$(basename "$connector")
      case "$name" in ..*) continue ;; esac
      url=$(cat "$connector")
      token_file="$CREDS_DIR/$name/token"
      [ -f "$token_file" ] || continue
      token=$(cat "$token_file")
      host=$(printf '%s\n' "$url" | sed -E 's|^https?://||; s|/.*$||')
      [ -n "$host" ] || continue
      printf 'https://x-access-token:%s@%s\n' "$token" "$host" >> "$HOME/.git-credentials"
      echo "entrypoint: appended git credential for connector $name ($host)" >&2
    done
  fi
}

bootstrap_git_credentials

# Persona file (optional). Pi's --append-system-prompt accepts a file
# path; missing files are silently skipped. AGENT.md (platform context)
# sits next to the persona and is added with the same flag.
PERSONA_PATH="${AGENT_PERSONA_PATH:-/etc/agent/persona.md}"
AGENT_MD_PATH="$(dirname "$PERSONA_PATH")/AGENT.md"

set --
set -- --mode rpc --no-session
[ -r "$PERSONA_PATH" ] && set -- "$@" --append-system-prompt "$PERSONA_PATH"
[ -r "$AGENT_MD_PATH" ] && set -- "$@" --append-system-prompt "$AGENT_MD_PATH"

# Load every shipped pi extension. Order does not matter — pi finishes
# loading them all before any session_start handler fires.
for ext in /usr/local/share/pi-extensions/*/index.ts; do
  [ -r "$ext" ] || continue
  set -- "$@" --extension "$ext"
done

# Provider + model: the custom provider is defined in
# /home/agent/.pi/agent/models.json (mounted by the operator's
# setup-pi-models init container). PI_PROVIDER selects which provider
# entry pi uses; defaults to ollama-api-key for backwards compatibility.
PI_PROVIDER="${PI_PROVIDER:-ollama-api-key}"
set -- "$@" --provider "$PI_PROVIDER" --model "${OLLAMA_CLOUD_MODEL:-glm-5.1:cloud}"

echo "entrypoint: exec pi $*" >&2

# Hold stdin open with `tail -f /dev/null` so RPC mode does not exit on
# EOF. We don't actually send RPC commands — the ainsel-runner
# extension drives the agent via pi.sendUserMessage().
#
# SIGTERM handling: when sh is PID 1 in the container, the kernel silently
# discards SIGTERM unless an explicit handler is installed (the kernel
# will never default-kill PID 1). Without the trap below, Kubernetes'
# pod termination SIGTERM is lost — pi never receives it, never fires
# session_shutdown, and the ainsel-runner NATS consumer stays connected
# until the 1800s grace period expires and SIGKILL fires.
#
# The pipeline MUST run in the background with `wait`. dash (our
# /bin/sh) is POSIX-compliant and defers trapped signals while waiting
# for a foreground pipeline to complete. Since `tail -f | pi` never
# completes, a foreground-pipeline trap is never reached — SIGTERM is
# queued indefinitely and the pod hangs for 1800s until SIGKILL fires.
# Running the pipeline in the background and using `wait` instead works
# because `wait` IS interruptible by trapped signals per POSIX.
#
# The trap sends SIGTERM to the process group (`kill -TERM 0`) so pi
# receives it and can drain gracefully (ainsel-runner's
# session_shutdown handler calls nc.drain()). `exit 0` ensures sh
# exits even if the signal re-delivers to PID 1 during trap execution.
# stdout redirect: the RPC mode JSON event stream on stdout is not
# consumed by anything -- event delivery goes through NATS via the
# ainsel-runner extension. Without the redirect, large agent_end
# events (1MB+) can fill the pipe buffer and block
# waitForRawStdoutBackpressure() indefinitely, leaving the agent
# stuck in a permanent "is processing" state. All logInfo/logError
# output goes to stderr (via takeOverStdout) and is still captured
# by Kubernetes.
#
# Redirect to a tmp file instead of /dev/null so the raw event stream
# is still accessible for troubleshooting via kubectl exec/cp.
PI_STDOUT_LOG="${PI_STDOUT_LOG:-/tmp/pi-stdout.log}"
export PI_STDOUT_LOG
exec sh -c '
  trap "kill -TERM 0; exit 0" TERM
  tail -f /dev/null | pi "$@" >"$PI_STDOUT_LOG" &
  wait
' _ "$@"
