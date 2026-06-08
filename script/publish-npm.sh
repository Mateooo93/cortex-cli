#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 <version> [--dry-run] [--yes] [--registry github|npmjs]"
  echo "  e.g. $0 v0.25.3"
  echo ""
  echo "  Publishes @mateooo93/cortex to GitHub Packages (default)."
  echo "  Use --registry npmjs to publish legacy mateooo93-cortex to registry.npmjs.org."
  echo ""
  echo "  Auth (GitHub Packages — default):"
  echo "    CI:  GITHUB_TOKEN (auto in Actions; needs packages: write)"
  echo "    Local: NODE_AUTH_TOKEN or GITHUB_TOKEN (PAT with write:packages)"
  echo ""
  echo "  Auth (npmjs — optional):"
  echo "    NODE_AUTH_TOKEN or NPM_TOKEN (Automation token w/ bypass 2FA)"
  echo ""
  echo "  --dry-run   Prepare package.json version only; do not publish"
  echo "  --yes       Skip confirmation prompt"
  exit 1
}

VERSION=""
DRY_RUN=false
YES=false
REGISTRY="github"
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
    --registry)
      REGISTRY="${2:-}"
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

# GitHub Actions: prefer GITHUB_TOKEN for GitHub Packages publish.
if [[ "$REGISTRY" == "github" && -z "${NODE_AUTH_TOKEN:-}" && -n "${GITHUB_TOKEN:-}" ]]; then
  export NODE_AUTH_TOKEN="$GITHUB_TOKEN"
fi

configure_npm_auth() {
  if [[ -z "${NODE_AUTH_TOKEN:-}" ]]; then
    return 1
  fi
  NODE_AUTH_TOKEN="$(printf '%s' "$NODE_AUTH_TOKEN" | tr -d '[:space:]')"
  export NODE_AUTH_TOKEN
  NPM_CONFIG_USERCONFIG="$(mktemp)"
  export NPM_CONFIG_USERCONFIG
  case "$REGISTRY" in
    github)
      cat > "$NPM_CONFIG_USERCONFIG" <<EOF
@mateooo93:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=${NODE_AUTH_TOKEN}
EOF
      ;;
    npmjs)
      cat > "$NPM_CONFIG_USERCONFIG" <<EOF
registry=https://registry.npmjs.org/
//registry.npmjs.org/:_authToken=${NODE_AUTH_TOKEN}
EOF
      ;;
    *)
      echo "!! Unknown registry: $REGISTRY (use github or npmjs)"
      return 1
      ;;
  esac
}

if [[ ! -f "$NPM_DIR/package.json" ]]; then
  echo "!! Missing $NPM_DIR/package.json"
  exit 1
fi

PACKAGE_NAME="@mateooo93/cortex"
PUBLISH_REGISTRY="https://npm.pkg.github.com"
if [[ "$REGISTRY" == "npmjs" ]]; then
  PACKAGE_NAME="mateooo93-cortex"
  PUBLISH_REGISTRY="https://registry.npmjs.org"
fi

echo "==> Setting npm package version to $SEMVER ($PACKAGE_NAME → $PUBLISH_REGISTRY)"
node -e "
const fs = require('fs');
const path = process.argv[1];
const version = process.argv[2];
const name = process.argv[3];
const registry = process.argv[4];
const pkgPath = path + '/package.json';
const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));
pkg.version = version;
pkg.name = name;
if (registry === 'https://npm.pkg.github.com') {
  pkg.publishConfig = { registry };
} else {
  delete pkg.publishConfig;
}
fs.writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + '\n');
" "$NPM_DIR" "$SEMVER" "$PACKAGE_NAME" "$PUBLISH_REGISTRY"

echo "==> Validating npm wrapper"
node -e "require('$NPM_DIR/lib/platform').resolveAsset()" >/dev/null

if [[ "$DRY_RUN" == true ]]; then
  echo "==> Dry run complete (package.json updated locally)"
  exit 0
fi

if [[ "$YES" != true ]]; then
  read -r -p "Publish $PACKAGE_NAME@$SEMVER to $PUBLISH_REGISTRY? [y/N] " OK || OK=""
  if [[ ! "$OK" =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 1
  fi
fi

if ! configure_npm_auth; then
  case "$REGISTRY" in
    github)
      echo "!! NODE_AUTH_TOKEN or GITHUB_TOKEN required (PAT with write:packages, or GITHUB_TOKEN in Actions)"
      ;;
    npmjs)
      echo "!! NODE_AUTH_TOKEN or NPM_TOKEN required (npm Automation token with bypass 2FA)"
      ;;
  esac
  exit 1
fi

NPM_USER="$(npm whoami --registry "$PUBLISH_REGISTRY" 2>/dev/null || true)"
if [[ -z "$NPM_USER" ]]; then
  echo "!! npm whoami failed for $PUBLISH_REGISTRY — token is missing or invalid"
  exit 1
fi
echo "==> npm auth ($PUBLISH_REGISTRY): $NPM_USER"

echo "==> Publishing $PACKAGE_NAME@$SEMVER..."
set +e
PUBLISH_OUT="$(
  cd "$NPM_DIR"
  npm publish --access public 2>&1
)"
PUBLISH_RC=$?
set -e
if [[ $PUBLISH_RC -ne 0 ]]; then
  echo "$PUBLISH_OUT"
  if [[ "$REGISTRY" == "npmjs" ]] && echo "$PUBLISH_OUT" | grep -qE 'E403|403 Forbidden|bypass 2fa|two-factor'; then
    cat <<'EOF' >&2

!! npmjs.org publish blocked (E403). Use GitHub Packages instead (default):
  ./script/publish-npm.sh <version> --yes
  # CI uses GITHUB_TOKEN automatically — no NPM_TOKEN needed.

To publish to npmjs.org anyway:
  1. Enable 2FA: https://www.npmjs.com/settings/Mateooo93/tfa
  2. Create Automation token: https://www.npmjs.com/settings/~/tokens
  3. Set GitHub secret NPM_TOKEN

EOF
  fi
  exit "$PUBLISH_RC"
fi
echo "$PUBLISH_OUT"

echo "==> npm publish complete: $PACKAGE_NAME@$SEMVER"