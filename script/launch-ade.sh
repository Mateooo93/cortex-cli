#!/usr/bin/env bash
set -euo pipefail

# Delegates to cortex-main when repos are siblings; falls back to cortex-cli-only layout.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MAIN_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
if [[ -f "$MAIN_ROOT/Makefile" && -d "$MAIN_ROOT/cortex-ade" ]]; then
  exec "$MAIN_ROOT/script/launch-ade.sh" "$@"
fi
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
LOG_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/cortex-ade"
mkdir -p "$LOG_DIR"

export NVM_DIR="${NVM_DIR:-$HOME/.nvm}"
if [[ -s "$NVM_DIR/nvm.sh" ]]; then
  # shellcheck source=/dev/null
  source "$NVM_DIR/nvm.sh"
fi

if command -v ss >/dev/null 2>&1 && ss -ltn 2>/dev/null | grep -q ':5173'; then
  if command -v notify-send >/dev/null 2>&1; then
    notify-send "Cortex ADE" "Already running"
  fi
  exit 0
fi

cd "$ROOT"
nohup env -u ELECTRON_RUN_AS_NODE make ade-dev >>"$LOG_DIR/ade-dev.log" 2>&1 &
disown
