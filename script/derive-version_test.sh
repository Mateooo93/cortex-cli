#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

fail() { echo "FAIL: $1" >&2; exit 1; }
pass() { echo "PASS: $1"; }

worktree="$(mktemp -d)"
trap 'rm -rf "$worktree"' EXIT
git clone -q --bare --no-tags . "$worktree/repo.git"
export GIT_DIR="$worktree/repo.git"

git tag v0.2.17
git tag v0.25.0
git tag v0.233.25

got="$(MIN_VERSION=v0.25.0 LEGACY_PREFIX=v0.233 "$ROOT/script/derive-version.sh")"
[[ "$got" == "v0.25.1" ]] || fail "expected v0.25.1 from v0.25.0 floor, got $got"
pass "bumps v0.25.0 to v0.25.1 and ignores v0.233.25"

git tag v0.25.3
got="$(MIN_VERSION=v0.25.0 LEGACY_PREFIX=v0.233 "$ROOT/script/derive-version.sh")"
[[ "$got" == "v0.25.4" ]] || fail "expected v0.25.4, got $got"
pass "bumps highest v0.25.x tag"

echo "All derive-version tests passed."