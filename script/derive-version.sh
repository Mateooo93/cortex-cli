#!/usr/bin/env bash
# derive-version.sh — compute the next cortex-cli release tag.
#
# Policy (since v0.25):
#   • Floor at MIN_VERSION (default v0.25.0)
#   • Ignore the legacy v0.233.* issue-number tag line
#   • Ignore anything below MIN_VERSION (old v0.2.x)
#   • Bump patch on the highest eligible tag
#
# Usage:
#   ./script/derive-version.sh            # print next tag, e.g. v0.25.1
#   MIN_VERSION=v0.25.0 ./script/derive-version.sh

set -euo pipefail

MIN_VERSION="${MIN_VERSION:-v0.25.0}"
LEGACY_PREFIX="${LEGACY_PREFIX:-v0.233}"

if ! [[ "$MIN_VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "invalid MIN_VERSION: $MIN_VERSION" >&2
  exit 1
fi

LATEST="$MIN_VERSION"
while IFS= read -r tag; do
  [[ -z "$tag" ]] && continue
  [[ "$tag" == "${LEGACY_PREFIX}"* ]] && continue
  if [[ "$(printf '%s\n' "$MIN_VERSION" "$tag" | sort -V | tail -1)" == "$tag" ]]; then
    LATEST="$tag"
  fi
done < <(git tag -l 'v*' | sort -V)

VER="${LATEST#v}"
IFS=. read -r MAJOR MINOR PATCH <<< "$VER"
PATCH=$((PATCH + 1))
NEXT="v${MAJOR}.${MINOR}.${PATCH}"

if [[ "$(printf '%s\n' "$MIN_VERSION" "$NEXT" | sort -V | tail -1)" != "$NEXT" ]] || [[ "$NEXT" == "$MIN_VERSION" ]]; then
  echo "derived version $NEXT is not above floor $MIN_VERSION" >&2
  exit 1
fi

if [[ "$NEXT" == "${LEGACY_PREFIX}"* ]]; then
  echo "derived version $NEXT matches legacy prefix $LEGACY_PREFIX" >&2
  exit 1
fi

echo "$NEXT"