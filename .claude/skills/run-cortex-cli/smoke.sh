#!/usr/bin/env bash
# cortex-cli smoke harness — build, launch, drive, screenshot.
# Prerequisites: tmux, Go 1.26+
# Usage: smoke.sh {build|headless|tui|all}
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
SMOKE_DIR="${CORTEX_SMOKE_DIR:-/tmp/cortex-smoke}"
BINARY="$REPO_ROOT/cortex"
FAKE_KEY="${CORTEX_FAKE_KEY:-sk-test-cortex-smoke-fake-key}"
TMUX_SESSION="cortex-smoke-$$"

mkdir -p "$SMOKE_DIR"

build() {
  echo "=== Building cortex ==="
  cd "$REPO_ROOT"
  go build -o "$BINARY" ./cmd/cortex/
  echo "Binary: $BINARY"
  "$BINARY" -version 2>&1 || true
}

headless() {
  echo "=== Headless: list-models ==="
  "$BINARY" -list-models 2>&1
  echo ""
  echo "=== Headless: version ==="
  "$BINARY" -version 2>&1 || true
  echo ""
  echo "=== Headless: prompt (expects API error, not crash) ==="
  timeout 5 "$BINARY" -p "hello" 2>&1 || true
}

tui() {
  echo "=== Launching TUI in test mode ==="
  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true

  OPENAI_API_KEY="$FAKE_KEY" \
    TERM=xterm-256color \
    tmux new-session -d -s "$TMUX_SESSION" -x 120 -y 40 "$BINARY -test 2>&1"

  sleep 3

  tmux capture-pane -t "$TMUX_SESSION" -p > "$SMOKE_DIR/chat-tab.txt"
  echo "  -> $SMOKE_DIR/chat-tab.txt"

  tmux send-keys -t "$TMUX_SESSION" F1
  sleep 0.5
  tmux capture-pane -t "$TMUX_SESSION" -p > "$SMOKE_DIR/sessions-tab.txt"
  echo "  -> $SMOKE_DIR/sessions-tab.txt"

  tmux send-keys -t "$TMUX_SESSION" F3
  sleep 0.5
  tmux capture-pane -t "$TMUX_SESSION" -p > "$SMOKE_DIR/settings-tab.txt"
  echo "  -> $SMOKE_DIR/settings-tab.txt"

  tmux send-keys -t "$TMUX_SESSION" F2
  sleep 0.3
  tmux send-keys -t "$TMUX_SESSION" '/'
  sleep 0.3
  tmux capture-pane -t "$TMUX_SESSION" -p > "$SMOKE_DIR/slash-menu.txt"
  echo "  -> $SMOKE_DIR/slash-menu.txt"

  tmux kill-session -t "$TMUX_SESSION" 2>/dev/null || true
  echo "  TUI smoke complete."
}

case "${1:-all}" in
  build)    build ;;
  headless) headless ;;
  tui)      tui ;;
  all)      build; headless; tui; echo "All checks passed. Captures: $SMOKE_DIR/"; ls -la "$SMOKE_DIR/" ;;
  *)        echo "Usage: $0 {build|headless|tui|all}"; exit 1 ;;
esac
