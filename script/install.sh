#!/usr/bin/env bash
# Install cortex-cli native binary from GitHub Releases (no npm auth).
set -euo pipefail

REPO="${CORTEX_CLI_REPO:-Mateooo93/cortex-cli}"
INSTALL_DIR="${CORTEX_INSTALL_DIR:-$HOME/.local/bin}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *)
    echo "unsupported architecture: $arch" >&2
    exit 1
    ;;
esac

asset="cortex-${os}-${arch}"
if [[ "$os" == "darwin" && "$arch" == "amd64" ]]; then
  echo "macOS amd64 is not published; use Apple Silicon (arm64) or npm." >&2
  exit 1
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

echo "==> Downloading latest ${asset} from ${REPO}"
curl -fsSL -o "$tmp" "https://github.com/${REPO}/releases/latest/download/${asset}"

mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp" "${INSTALL_DIR}/cortex"

echo "==> Installed ${INSTALL_DIR}/cortex"
"${INSTALL_DIR}/cortex" --version
echo "Ensure ${INSTALL_DIR} is on your PATH."