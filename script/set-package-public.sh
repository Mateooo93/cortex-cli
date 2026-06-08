#!/usr/bin/env bash
# Set @mateooo93/cortex visibility to public on GitHub Packages.
# Requires GITHUB_TOKEN (or gh auth) with read:packages + write:packages.
set -euo pipefail

OWNER="${CORTEX_PACKAGE_OWNER:-Mateooo93}"
REPO="${CORTEX_PACKAGE_REPO:-cortex-cli}"
PACKAGE_NAME="${CORTEX_NPM_PACKAGE:-@mateooo93/cortex}"
ENCODED_NAME="$(python3 -c "import urllib.parse; print(urllib.parse.quote('${PACKAGE_NAME}', safe=''))")"

if ! command -v gh >/dev/null 2>&1; then
  echo "!! gh CLI required" >&2
  exit 1
fi

export GH_TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
if [[ -z "${GH_TOKEN}" ]]; then
  echo "!! Set GITHUB_TOKEN or GH_TOKEN (PAT with read:packages + write:packages)" >&2
  exit 1
fi

echo "==> Resolving package id for ${PACKAGE_NAME}"

PACKAGE_ID="$(gh api graphql -f query='
query($owner: String!, $name: String!, $pkg: String!) {
  repository(owner: $owner, name: $name) {
    packages(first: 20, names: [$pkg], packageType: NPM) {
      nodes { id name }
    }
  }
}' -f owner="$OWNER" -f name="$REPO" -f pkg="$PACKAGE_NAME" \
  --jq '.data.repository.packages.nodes[0].id' 2>/dev/null || true)"

if [[ -z "$PACKAGE_ID" || "$PACKAGE_ID" == "null" ]]; then
  PACKAGE_ID="$(gh api graphql -f query='
query($login: String!, $pkg: String!) {
  user(login: $login) {
    packages(first: 20, names: [$pkg], packageType: NPM) {
      nodes { id name }
    }
  }
}' -f login="$OWNER" -f pkg="$PACKAGE_NAME" \
    --jq '.data.user.packages.nodes[0].id' 2>/dev/null || true)"
fi

if [[ -n "$PACKAGE_ID" && "$PACKAGE_ID" != "null" ]]; then
  echo "==> Setting visibility PUBLIC via GraphQL (id=${PACKAGE_ID})"
  gh api graphql -f query='
mutation($packageId: ID!) {
  updatePackagesSettings(input: {packageId: $packageId, visibility: PUBLIC}) {
    package { id name }
  }
}' -f packageId="$PACKAGE_ID" >/dev/null
  echo "==> Package visibility set to PUBLIC"
  exit 0
fi

echo "==> GraphQL lookup failed; trying REST visibility endpoint"
if gh api --method POST \
  -H "Accept: application/vnd.github+json" \
  "/user/packages/npm/${ENCODED_NAME}/visibility" \
  -f visibility=public >/dev/null 2>&1; then
  echo "==> Package visibility set to PUBLIC (REST)"
  exit 0
fi

echo "!! Could not change package visibility automatically." >&2
echo "   Open: https://github.com/users/${OWNER}/packages/npm/package/${ENCODED_NAME}" >&2
echo "   → Package settings → Change visibility → Public" >&2
exit 1