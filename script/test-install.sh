#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 [--local | --tap <owner/tap>]"
  echo ""
  echo "  --local          Install from local dist/ tarballs and formula (default)"
  echo "  --tap owner/tap  Install via 'brew tap owner/tap && brew install cortex'"
  echo ""
  echo "  e.g. $0 --local"
  echo "  e.g. $0 --tap Mateooo93/cortex"
  exit 1
}

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DIST_DIR="$ROOT_DIR/dist"

MODE="local"
TAP=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --local)
      MODE="local"
      shift
      ;;
    --tap)
      MODE="tap"
      TAP="$2"
      shift 2
      ;;
    -h|--help)
      usage
      ;;
    *)
      echo "Unknown argument: $1"
      usage
      ;;
  esac
done

# Detect platform
ARCH="$(uname -m)"
if [[ "$ARCH" == "arm64" || "$ARCH" == "aarch64" ]]; then
  PLATFORM="linux/arm64"
else
  PLATFORM="linux/amd64"
fi

if [[ "$MODE" == "local" ]]; then
  if [[ ! -f "$DIST_DIR/cortex-local.rb" ]]; then
    echo "Missing $DIST_DIR/cortex-local.rb — run release.sh first."
    exit 1
  fi

  echo "==> Starting test container (local install)"
  docker run --rm -it \
    --platform "$PLATFORM" \
    -v "$DIST_DIR:/tmp/dist:ro" \
    homebrew/brew:latest \
    bash -c "
      if sudo apt-get update -qq 2>/dev/null && sudo apt-get install -y -qq dbus-x11 gnome-keyring libsecret-tools >/dev/null 2>&1; then
        eval \$(dbus-launch --sh-syntax)
        echo '' | gnome-keyring-daemon --unlock --components=secrets >/dev/null 2>&1 || true
        echo '==> Keychain tools installed (secret-tool available)'
      else
        echo '==> Skipping keychain tools (apt failed — not required for testing)'
      fi
      mkdir -p \$(brew --repo)/Library/Taps/local/homebrew-cortex/Formula
      cp /tmp/dist/cortex-local.rb \$(brew --repo)/Library/Taps/local/homebrew-cortex/Formula/cortex.rb
      echo '==> Installing cortex via Homebrew...'
      brew install cortex
      echo ''
      echo '==> Done! Type \"cortex\" to test.'
      echo ''
      exec bash
    "
else
  echo "==> Starting test container (tap install) with brew tap $TAP"
  docker run --rm -it \
    --platform "$PLATFORM" \
    homebrew/brew:latest \
    bash -c "
      if sudo apt-get update -qq 2>/dev/null && sudo apt-get install -y -qq dbus-x11 gnome-keyring libsecret-tools >/dev/null 2>&1; then
        eval \$(dbus-launch --sh-syntax)
        echo '' | gnome-keyring-daemon --unlock --components=secrets >/dev/null 2>&1 || true
        echo '==> Keychain tools installed (secret-tool available)'
      else
        echo '==> Skipping keychain tools (apt failed — not required for testing)'
      fi
      echo '==> brew tap $TAP'
      brew tap $TAP
      echo '==> brew install cortex'
      brew install cortex
      echo ''
      echo '==> Done! Type \"cortex\" to test.'
      echo ''
      exec bash
    "
fi
