#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 <version> --tap <owner/homebrew-repo>"
  echo "  e.g. $0 v0.1.0 --tap Mateooo93/homebrew-cortex"
  exit 1
}

# Parse arguments
VERSION=""
TAP=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --tap)
      TAP="$2"
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

if [[ -z "$VERSION" || -z "$TAP" ]]; then
  usage
fi

# Ensure version starts with v
if [[ "$VERSION" != v* ]]; then
  VERSION="v$VERSION"
fi

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DIST_DIR="$ROOT_DIR/dist"

if [[ ! -f "$DIST_DIR/cortex.rb" ]]; then
  echo "Missing dist/cortex.rb — run build.sh first."
  exit 1
fi

# Clone tap repo to temp dir
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "==> Cloning tap repo $TAP..."
git clone "git@github.com:${TAP}.git" "$TMPDIR/tap"

# Copy formula
mkdir -p "$TMPDIR/tap/Formula"
cp "$DIST_DIR/cortex.rb" "$TMPDIR/tap/Formula/cortex.rb"

# Commit and push
cd "$TMPDIR/tap"
git add Formula/cortex.rb
git commit -m "cortex $VERSION"
git push

echo ""
echo "==> Homebrew tap $TAP updated with cortex $VERSION"
