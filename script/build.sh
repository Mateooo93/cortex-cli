#!/usr/bin/env bash
# build.sh — compile cortex for darwin-arm64 + linux-amd64 + linux-arm64
# + windows-amd64 + windows-arm64 and drop the loose binaries into
# ./bin/ (or -o <dir>).
#
# cortex-cli is a single binary (no separate daemon), so we only need
# to produce one cortex-<platform> binary per architecture.
#
# This is the single build entry point. Callers:
#   • script/release.sh       — runs this with --version, then hands off to publish.sh
#
# Tarballs, checksums, GPG signing, and Homebrew formula generation are NOT
# done here. They live in script/publish.sh, which reads from this script's
# output dir.
#
# Usage:
#   ./build.sh                       # version=dev, output=<repo>/bin
#   ./build.sh --version v0.2.0      # embed v0.2.0 into the binary (-X main.Version)
#   ./build.sh --force               # rebuild even if .build-commit matches HEAD
#   ./build.sh -o /tmp/cortex-out    # override output dir
#
# Output:
#   <out>/cortex-darwin-arm64
#   <out>/cortex-linux-amd64
#   <out>/cortex-linux-arm64
#   <out>/cortex-windows-amd64.exe
#   <out>/cortex-windows-arm64.exe
#   <out>/.build-commit       # git HEAD of the cortex-cli repo at build time

set -euo pipefail

# ── Parse args ────────────────────────────────────────────────────────────────
VERSION="dev"
FORCE=0
OUT_DIR=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="$2"
      shift 2
      ;;
    --force)
      FORCE=1
      shift
      ;;
    -o)
      OUT_DIR="$2"
      shift 2
      ;;
    -h|--help)
      sed -n '/^# Usage:/,/^$/p' "$0" | sed 's/^# \?//'
      exit 0
      ;;
    *)
      echo "Error: unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
if [[ -z "$OUT_DIR" ]]; then
  OUT_DIR="$ROOT_DIR/bin"
fi

# ── Colors (disabled when stdout is not a tty) ───────────────────────────────
if [ -t 1 ]; then
  C_RESET=$'\033[0m'; C_BOLD=$'\033[1m'
  C_BLUE=$'\033[34m'; C_GREEN=$'\033[32m'
  C_YELLOW=$'\033[33m'; C_RED=$'\033[31m'; C_DIM=$'\033[2m'
else
  C_RESET=""; C_BOLD=""; C_BLUE=""; C_GREEN=""; C_YELLOW=""; C_RED=""; C_DIM=""
fi

# ── Dirty-tree check ─────────────────────────────────────────────────────────
if [[ -n "$(git -C "$ROOT_DIR" status --porcelain 2>/dev/null)" ]]; then
  if [ -t 0 ]; then
    echo "${C_YELLOW}!!${C_RESET} ${C_BOLD}$ROOT_DIR${C_RESET} has uncommitted changes:"
    git -C "$ROOT_DIR" status --short
    read -r -p "Continue anyway? [y/N] " ans
    if [[ ! "$ans" =~ ^[Yy]$ ]]; then
      echo "Aborted."
      exit 1
    fi
  else
    echo "${C_YELLOW}warning:${C_RESET} $ROOT_DIR has uncommitted changes, proceeding (non-interactive)." >&2
  fi
fi

CURRENT_COMMIT="$(git -C "$ROOT_DIR" rev-parse HEAD 2>/dev/null || echo "unknown")"
STAMP_FILE="$OUT_DIR/.build-commit"

# ── Staleness check ──────────────────────────────────────────────────────────
if [[ $FORCE -eq 0 && "$VERSION" == "dev" \
    && -f "$STAMP_FILE" \
    && -f "$OUT_DIR/cortex-darwin-arm64" \
    && -f "$OUT_DIR/cortex-linux-amd64"  \
    && -f "$OUT_DIR/cortex-linux-arm64"  \
    && -f "$OUT_DIR/cortex-windows-amd64.exe" \
    && -f "$OUT_DIR/cortex-windows-arm64.exe" \
    && "$(cat "$STAMP_FILE")" == "$CURRENT_COMMIT" ]]; then
  echo "${C_GREEN}==>${C_RESET} cortex binaries up to date (commit ${C_BOLD}${CURRENT_COMMIT:0:12}${C_RESET}), skipping."
  echo "    ${C_DIM}Run with --force to rebuild anyway.${C_RESET}"
  exit 0
fi

mkdir -p "$OUT_DIR"

echo "${C_BLUE}==>${C_RESET} ${C_BOLD}Building cortex${C_RESET} (darwin-arm64 + linux-amd64 + linux-arm64 + windows-amd64 + windows-arm64), version ${C_BOLD}${VERSION}${C_RESET}, commit ${C_BOLD}${CURRENT_COMMIT:0:12}${C_RESET}"

# ── Launch all three builds in parallel ──────────────────────────────────────
# darwin-arm64 runs natively (no docker). Each linux arch runs in its own
# `docker build` pipeline on golang:1.26-alpine — CGO on for tree-sitter,
# netgo+osusergo tags + static extldflags so the binary is fully
# self-contained (no glibc/NSS runtime deps). The Dockerfile is a heredoc so
# BuildKit caches `go mod download` in its own layer (module graph survives
# between releases) with a 3x retry for flaky proxy.

darwin_log="$(mktemp)"
amd64_log="$(mktemp)"
arm64_log="$(mktemp)"
windows_amd64_log="$(mktemp)"
windows_arm64_log="$(mktemp)"

# darwin-arm64 — native build
(
  CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
    go build -C "$ROOT_DIR" -trimpath \
      -ldflags="-s -w -X main.Version=${VERSION}" \
      -o "$OUT_DIR/cortex-darwin-arm64" .
) >"$darwin_log" 2>&1 &
darwin_pid=$!

# Windows — cross-compiled from Linux. CGO_ENABLED=0 because the
# static Go runtime doesn't need the Microsoft C runtime and
# builds produce a self-contained .exe. TAGS are the same as
# the linux static-build path.
(
  CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
    go build -C "$ROOT_DIR" -trimpath \
      -tags 'netgo osusergo' \
      -ldflags="-s -w -X main.Version=${VERSION}" \
      -o "$OUT_DIR/cortex-windows-amd64.exe" .
) >"$windows_amd64_log" 2>&1 &
windows_amd64_pid=$!
(
  CGO_ENABLED=0 GOOS=windows GOARCH=arm64 \
    go build -C "$ROOT_DIR" -trimpath \
      -tags 'netgo osusergo' \
      -ldflags="-s -w -X main.Version=${VERSION}" \
      -o "$OUT_DIR/cortex-windows-arm64.exe" .
) >"$windows_arm64_log" 2>&1 &
windows_arm64_pid=$!

# linux-* — docker build
build_linux_docker() {
  local arch="$1" label="$2" logfile="$3"
  local tag="cortex-build-${label}"
  local create_name="cortex-extract-${label}-$$"
  # Unquoted heredoc — ${VERSION} expanded by the shell before docker sees it.
  docker build --platform "linux/${arch}" -f - -t "$tag" "$ROOT_DIR" <<DOCKERFILE >"$logfile" 2>&1
FROM golang:1.26-alpine
RUN apk add --no-cache build-base
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download \
 || (sleep 5  && go mod download) \
 || (sleep 15 && go mod download)
COPY . .
RUN mkdir -p /out \
    && go build -trimpath -tags 'netgo osusergo' \
         -ldflags="-s -w -linkmode external -extldflags '-static' -X main.Version=${VERSION}" \
         -o /out/cortex .
DOCKERFILE
  docker create --name "$create_name" "$tag" true >>"$logfile" 2>&1
  docker cp "$create_name":/out/cortex "$OUT_DIR/cortex-${label}"  >>"$logfile" 2>&1
  docker rm "$create_name" >>"$logfile" 2>&1
}

build_linux_docker amd64 linux-amd64 "$amd64_log" &
amd64_pid=$!
build_linux_docker arm64 linux-arm64 "$arm64_log" &
arm64_pid=$!

# ── Live 5-column spinner ────────────────────────────────────────────────────
parallel_start=$SECONDS
frames=('⠋' '⠙' '⠹' '⠸' '⠼' '⠴' '⠦' '⠧' '⠇' '⠏')
darwin_done=0; amd64_done=0; arm64_done=0
windows_amd64_done=0; windows_arm64_done=0
darwin_elapsed=0; amd64_elapsed=0; arm64_elapsed=0
windows_amd64_elapsed=0; windows_arm64_elapsed=0
i=0

if [ -t 1 ]; then
  printf '\033[?25l'  # hide cursor
  while [ $darwin_done -eq 0 ] || [ $amd64_done -eq 0 ] || [ $arm64_done -eq 0 ] \
        || [ $windows_amd64_done -eq 0 ] || [ $windows_arm64_done -eq 0 ]; do
    frame="${frames[i++ % ${#frames[@]}]}"

    if [ $darwin_done -eq 0 ] && ! kill -0 "$darwin_pid" 2>/dev/null; then
      darwin_done=1; darwin_elapsed=$((SECONDS - parallel_start))
    fi
    if [ $amd64_done -eq 0 ] && ! kill -0 "$amd64_pid" 2>/dev/null; then
      amd64_done=1; amd64_elapsed=$((SECONDS - parallel_start))
    fi
    if [ $arm64_done -eq 0 ] && ! kill -0 "$arm64_pid" 2>/dev/null; then
      arm64_done=1; arm64_elapsed=$((SECONDS - parallel_start))
    fi
    if [ $windows_amd64_done -eq 0 ] && ! kill -0 "$windows_amd64_pid" 2>/dev/null; then
      windows_amd64_done=1; windows_amd64_elapsed=$((SECONDS - parallel_start))
    fi
    if [ $windows_arm64_done -eq 0 ] && ! kill -0 "$windows_arm64_pid" 2>/dev/null; then
      windows_arm64_done=1; windows_arm64_elapsed=$((SECONDS - parallel_start))
    fi

    fmt_status() {
      local done="$1" elapsed="$2" label="$3"
      if [ "$done" -eq 0 ]; then
        printf "%s%s%s %s %s(%ss)%s" "$C_BLUE" "$frame" "$C_RESET" "$label" "$C_DIM" "$((SECONDS - parallel_start))" "$C_RESET"
      else
        printf "%s✓%s %s %s(%ss)%s" "$C_GREEN" "$C_RESET" "$label" "$C_DIM" "$elapsed" "$C_RESET"
      fi
    }

    printf "\r\033[K  %s    %s    %s    %s    %s" \
      "$(fmt_status $darwin_done $darwin_elapsed darwin-arm64)" \
      "$(fmt_status $amd64_done $amd64_elapsed linux-amd64)" \
      "$(fmt_status $arm64_done $arm64_elapsed linux-arm64)" \
      "$(fmt_status $windows_amd64_done $windows_amd64_elapsed windows-amd64)" \
      "$(fmt_status $windows_arm64_done $windows_arm64_elapsed windows-arm64)"
    sleep 0.1
  done
  printf '\033[?25h\r\033[K'  # restore cursor, clear line
else
  while [ $darwin_done -eq 0 ] || [ $amd64_done -eq 0 ] || [ $arm64_done -eq 0 ] \
        || [ $windows_amd64_done -eq 0 ] || [ $windows_arm64_done -eq 0 ]; do
    if [ $darwin_done -eq 0 ] && ! kill -0 "$darwin_pid" 2>/dev/null; then darwin_done=1; fi
    if [ $amd64_done -eq 0 ] && ! kill -0 "$amd64_pid" 2>/dev/null; then amd64_done=1; fi
    if [ $arm64_done -eq 0 ] && ! kill -0 "$arm64_pid" 2>/dev/null; then arm64_done=1; fi
    if [ $windows_amd64_done -eq 0 ] && ! kill -0 "$windows_amd64_pid" 2>/dev/null; then windows_amd64_done=1; fi
    if [ $windows_arm64_done -eq 0 ] && ! kill -0 "$windows_arm64_pid" 2>/dev/null; then windows_arm64_done=1; fi
    sleep 1
  done
fi

# ── Reap exit codes ──────────────────────────────────────────────────────────
wait "$darwin_pid" || { echo "${C_RED}✗${C_RESET} darwin-arm64 build failed:"; cat "$darwin_log"; exit 1; }
wait "$amd64_pid"  || { echo "${C_RED}✗${C_RESET} linux-amd64 build failed:";  cat "$amd64_log";  exit 1; }
wait "$arm64_pid"  || { echo "${C_RED}✗${C_RESET} linux-arm64 build failed:";  cat "$arm64_log";  exit 1; }
wait "$windows_amd64_pid"  || { echo "${C_RED}✗${C_RESET} windows-amd64 build failed:";  cat "$windows_amd64_log";  exit 1; }
wait "$windows_arm64_pid"  || { echo "${C_RED}✗${C_RESET} windows-arm64 build failed:";  cat "$windows_arm64_log";  exit 1; }

echo "${C_GREEN}==>${C_RESET} All builds complete in $((SECONDS - parallel_start))s."
ls -lh "$OUT_DIR"/cortex-* 2>/dev/null | awk '{print "    " $9 " (" $5 ")"}'

echo "$CURRENT_COMMIT" > "$STAMP_FILE"
