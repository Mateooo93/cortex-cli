#!/usr/bin/env bash
# Set @mateooo93/cortex visibility to public on GitHub Packages.
# npm packages are not supported by GitHub GraphQL — use REST only.
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

echo "==> Checking visibility for ${PACKAGE_NAME}"

package_visibility() {
  local endpoint="$1"
  gh api "$endpoint" --jq '.visibility' 2>/dev/null || true
}

GET_ENDPOINTS=(
  "/user/packages/npm/${ENCODED_NAME}"
  "/repos/${OWNER}/${REPO}/packages/npm/${ENCODED_NAME}"
)

for endpoint in "${GET_ENDPOINTS[@]}"; do
  visibility="$(package_visibility "$endpoint")"
  if [[ "$visibility" == "public" ]]; then
    echo "==> Already public (${endpoint})"
    exit 0
  fi
  if [[ -n "$visibility" && "$visibility" != "null" ]]; then
    echo "==> Current visibility (${endpoint}): ${visibility}"
  fi
done

set_public() {
  local endpoint="$1"
  echo "==> Setting visibility PUBLIC via REST (${endpoint})"
  gh api --method POST "$endpoint" -f visibility=public
}

POST_ENDPOINTS=(
  "/user/packages/npm/${ENCODED_NAME}/visibility"
  "/repos/${OWNER}/${REPO}/packages/npm/${ENCODED_NAME}/visibility"
)

LAST_ERR=""
for endpoint in "${POST_ENDPOINTS[@]}"; do
  if err="$(set_public "$endpoint" 2>&1)"; then
    echo "==> Package visibility set to PUBLIC"
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