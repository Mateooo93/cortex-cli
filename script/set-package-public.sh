#!/usr/bin/env bash
# Set @mateooo93/cortex visibility to public on GitHub Packages.
# npm packages are not supported by GitHub GraphQL — use REST only.
# GitHub Packages REST API uses the unscoped name (e.g. "cortex"), not "@scope/name".
# Requires GITHUB_TOKEN (or gh auth) with read:packages + write:packages.
set -euo pipefail

OWNER="${CORTEX_PACKAGE_OWNER:-Mateooo93}"
REPO="${CORTEX_PACKAGE_REPO:-cortex-cli}"
NPM_NAME="${CORTEX_NPM_PACKAGE:-@mateooo93/cortex}"
# GitHub Packages REST name (see repo Packages tab — usually unscoped).
GH_NAME="${CORTEX_GH_PACKAGE_NAME:-cortex}"
ENCODED_NAME="$(python3 -c "import urllib.parse; print(urllib.parse.quote('${GH_NAME}', safe=''))")"

if ! command -v gh >/dev/null 2>&1; then
  echo "!! gh CLI required" >&2
  exit 1
fi

export GH_TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"
if [[ -z "${GH_TOKEN}" ]]; then
  echo "!! Set GITHUB_TOKEN or GH_TOKEN (PAT with read:packages + write:packages)" >&2
  exit 1
fi

echo "==> Checking visibility for ${NPM_NAME} (GitHub Packages name: ${GH_NAME})"

package_visibility() {
  local endpoint="$1"
  local out
  if ! out="$(gh api "$endpoint" --jq '.visibility' 2>/dev/null)"; then
    return 1
  fi
  if [[ "$out" == "public" || "$out" == "private" ]]; then
    echo "$out"
    return 0
  fi
  return 1
}

GET_ENDPOINTS=(
  "/repos/${OWNER}/${REPO}/packages/npm/${ENCODED_NAME}"
  "/user/packages/npm/${ENCODED_NAME}"
)

for endpoint in "${GET_ENDPOINTS[@]}"; do
  if visibility="$(package_visibility "$endpoint")"; then
    echo "==> Current visibility (${endpoint}): ${visibility}"
    if [[ "$visibility" == "public" ]]; then
      echo "==> Already public"
      exit 0
    fi
  fi
done

set_public() {
  local endpoint="$1"
  echo "==> Setting visibility PUBLIC via REST (${endpoint})"
  gh api --method POST "$endpoint" -f visibility=public
}

POST_ENDPOINTS=(
  "/repos/${OWNER}/${REPO}/packages/npm/${ENCODED_NAME}/visibility"
  "/user/packages/npm/${ENCODED_NAME}/visibility"
)

LAST_ERR=""
for endpoint in "${POST_ENDPOINTS[@]}"; do
  if err="$(set_public "$endpoint" 2>&1)"; then
    echo "==> Package visibility set to PUBLIC (${NPM_NAME})"
    exit 0
  fi
  LAST_ERR="$err"
  echo "!! ${endpoint} failed: ${err}" >&2
done

echo "!! Could not change package visibility automatically." >&2
if [[ -n "$LAST_ERR" ]]; then
  echo "   Last error: ${LAST_ERR}" >&2
fi
echo "   Open: https://github.com/users/${OWNER}/packages/npm/package/${ENCODED_NAME}" >&2
echo "   → Package settings → Change visibility → Public" >&2
exit 1