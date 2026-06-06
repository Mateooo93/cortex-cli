#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 <version> --repo <owner/repo> [--tap <owner/tap>] [--force] [--changelog <path>]"
  echo "  e.g. $0 v0.1.0 --repo Mateooo93/cortex-cli"
  echo ""
  echo "  --tap <owner/tap>    Homebrew tap repo (default: derives owner/homebrew-cortex from --repo)"
  echo "  --force              Replace an existing release"
  echo "  --changelog <path>   Use contents of this file as the release changelog"
  echo "                       (skips the git-log derivation; confirmation prompt still shown)"
  exit 1
}

# Parse arguments
VERSION=""
REPO=""
TAP=""
FORCE=false
CHANGELOG_FILE=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --repo)
      REPO="$2"
      shift 2
      ;;
    --tap)
      TAP="$2"
      shift 2
      ;;
    --force)
      FORCE=true
      shift
      ;;
    --changelog)
      CHANGELOG_FILE="$2"
      shift 2
      ;;
    -h|--help)
      usage
      ;;
    *)
      if [[ -z "$VERSION" ]]; then
        VERSION="$1"
      else
        echo "Unknown argument: $1"
        usage
      fi
      shift
      ;;
  esac
done

if [[ -z "$VERSION" || -z "$REPO" ]]; then
  usage
fi

if [[ -n "$CHANGELOG_FILE" ]]; then
  if [[ ! -f "$CHANGELOG_FILE" ]]; then
    echo "!! --changelog file not found: $CHANGELOG_FILE"
    exit 1
  fi
  # Resolve to an absolute path so publish.sh can still find it regardless of cwd.
  CHANGELOG_FILE="$(cd "$(dirname "$CHANGELOG_FILE")" && pwd)/$(basename "$CHANGELOG_FILE")"
fi

# Derive tap from repo owner if not specified
if [[ -z "$TAP" ]]; then
  OWNER="${REPO%%/*}"
  TAP="${OWNER}/homebrew-cortex"
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

# Load DISCORD_WEBHOOK_URL from .env if not already set. Release must have a webhook.
if [[ -z "${DISCORD_WEBHOOK_URL:-}" ]]; then
  ENV_FILE="$ROOT_DIR/.env"
  if [[ -f "$ENV_FILE" ]]; then
    DISCORD_WEBHOOK_URL="$(grep '^DISCORD_WEBHOOK_URL=' "$ENV_FILE" | head -n1 | cut -d= -f2-)"
    export DISCORD_WEBHOOK_URL
  fi
fi

# Note: DISCORD_WEBHOOK_URL is optional. If absent, the release is still
# created — the Discord announcement step in publish.sh just becomes a no-op.
# if [[ -z "${DISCORD_WEBHOOK_URL:-}" ]]; then
#   echo "!! DISCORD_WEBHOOK_URL is not set. Configure it in your environment or in .env before releasing."
#   exit 1
# fi

# --- Step 0: Ensure git working tree is clean ---
echo "==> Checking git status..."
if [[ -n "$(git status --porcelain)" ]]; then
  echo "!! Git working tree is not clean. Please commit or stash your changes before releasing."
  git status --short
  exit 1
fi
echo "==> Git working tree is clean."
echo ""

# --- Step 1: Build ---
# build.sh produces loose binaries in $ROOT_DIR/bin/. publish.sh picks them
# up and owns tarballs, SHA256, GPG, formula.
"$SCRIPT_DIR/build.sh" --version "$VERSION" --force

# --- Step 2: Publish ---
PUBLISH_ARGS=("$VERSION" --repo "$REPO")
if [[ "$FORCE" == true ]]; then
  PUBLISH_ARGS+=(--force)
fi
if [[ -n "$CHANGELOG_FILE" ]]; then
  PUBLISH_ARGS+=(--changelog "$CHANGELOG_FILE")
fi
"$SCRIPT_DIR/publish.sh" "${PUBLISH_ARGS[@]}"

# --- Step 3: Update tap (skipped silently if the user has not set up a tap) ---
if [[ -n "$TAP" ]]; then
  "$SCRIPT_DIR/update-tap.sh" "$VERSION" --tap "$TAP" || \
    echo "!! Tap update failed; the GitHub release is still published."
fi

# --- Step 4: Tag the release commit (GPG-signed if configured) ---
echo ""
echo "==> Tagging commit as $VERSION..."
if [[ "$FORCE" == true ]]; then
  git tag -d "$VERSION" 2>/dev/null || true
fi
if git config --get user.signingkey >/dev/null 2>&1; then
  echo "→  Signing tag (will prompt for YubiKey / GPG key touch if needed)"
  git tag -s "$VERSION" -m "Release $VERSION" || git tag -a "$VERSION" -m "Release $VERSION"
else
  git tag -a "$VERSION" -m "Release $VERSION"
fi
echo "==> Tagged $VERSION."

# --- Done ---
echo ""
echo "================================================"
echo "  cortex-cli $VERSION released successfully!"
echo ""
echo "  Install with:"
echo "    brew tap ${TAP%%/*}/cortex"
echo "    brew install cortex"
echo "================================================"
