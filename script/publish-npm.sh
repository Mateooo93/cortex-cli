#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 <version> [--dry-run] [--yes]"
  echo "  e.g. $0 v0.25.3"
  echo ""
  echo "  Publishes mateooo93-cortex to the npm registry."
  echo "  Requires NODE_AUTH_TOKEN or npm login."
  echo "  --dry-run   Prepare package.json version only; do not publish"
  echo "  --yes       Skip confirmation prompt"
  exit 1
}

VERSION=""
DRY_RUN=false
YES=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --yes)
      YES=true
      shift
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

if [[ -z "$VERSION" ]]; then
  usage
fi

if [[ "$VERSION" != v* ]]; then
  VERSION="v$VERSION"
fi
SEMVER="${VERSION#v}"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
NPM_DIR="$ROOT_DIR/packaging/npm"

# Load NPM_TOKEN from .env when NODE_AUTH_TOKEN is not already set.
if [[ -z "${NODE_AUTH_TOKEN:-}" && -f "$ROOT_DIR/.env" ]]; then
  NPM_TOKEN="$(grep -E '^NPM_TOKEN=' "$ROOT_DIR/.env" | head -n1 | cut -d= -f2- | tr -d '"' | tr -d "'")"
  if [[ -n "$NPM_TOKEN" ]]; then
    export NODE_AUTH_TOKEN="$NPM_TOKEN"
  fi
fi

configure_npm_auth() {
  if [[ -z "${NODE_AUTH_TOKEN:-}" ]]; then
    return 1
  fi
  # GitHub Actions secrets (and some .env files) may include stray whitespace.
  NODE_AUTH_TOKEN="$(printf '%s' "$NODE_AUTH_TOKEN" | tr -d '[:space:]')"
  export NODE_AUTH_TOKEN
  NPM_CONFIG_USERCONFIG="$(mktemp)"
  export NPM_CONFIG_USERCONFIG
  cat > "$NPM_CONFIG_USERCONFIG" <<EOF
registry=https://registry.npmjs.org/
//registry.npmjs.org/:_authToken=${NODE_AUTH_TOKEN}
EOF
}

if [[ ! -f "$NPM_DIR/package.json" ]]; then
  echo "!! Missing $NPM_DIR/package.json"
  exit 1
fi

echo "==> Setting npm package version to $SEMVER"
node -e "
const fs = require('fs');
const path = process.argv[1];
const version = process.argv[2];
const pkgPath = path + '/package.json';
const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));
pkg.version = version;
fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + '\n');
" "$NPM_DIR" "$SEMVER"

echo "==> Validating npm wrapper"
node -e "require('$NPM_DIR/lib/platform').resolveAsset()" >/dev/null

if [[ "$DRY_RUN" == true ]]; then
  echo "==> Dry run complete (package.json updated locally)"
  exit 0
fi

if [[ "$YES" != true ]]; then
  read -r -p "Publish mateooo93-cortex@$SEMVER to npm? [y/N] " OK || OK=""
  if [[ ! "$OK" =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 1
  fi
fi

if ! configure_npm_auth; then
  echo "!! NODE_AUTH_TOKEN is not set (npm login or NPM_TOKEN secret required)"
  exit 1
fi

echo "==> npm auth: $(npm whoami --registry https://registry.npmjs.org)"

echo "==> Publishing mateooo93-cortex@$SEMVER to npm..."
(
  cd "$NPM_DIR"
  npm publish --access public
)

echo "==> npm publish complete: mateooo93-cortex@$SEMVER"