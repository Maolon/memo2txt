#!/usr/bin/env bash
# memos2txt installer script
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Maolon/memo2txt/main/install.sh | bash

set -euo pipefail

REPO="Maolon/memo2txt"
BINARY_NAME="memos2txt"
INSTALL_DIR="/usr/local/bin"

echo "==> Detecting platform architecture..."
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64|amd64)
    ARCH="amd64"
    ;;
  arm64|aarch64)
    ARCH="arm64"
    ;;
  *)
    echo "Error: Unsupported architecture $ARCH" >&2
    exit 1
    ;;
esac

case "$OS" in
  darwin|linux)
    ;;
  *)
    echo "Error: Unsupported operating system $OS. For Windows, download from https://github.com/${REPO}/releases/latest" >&2
    exit 1
    ;;
esac

TARBALL="memos2txt-${OS}-${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${TARBALL}"

echo "==> Downloading ${BINARY_NAME} (${OS}/${ARCH})..."
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

# 1. Try gh CLI if available (supports private repos automatically)
if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
  echo "==> Using GitHub CLI to fetch release asset from ${REPO}..."
  gh release download --repo "${REPO}" --pattern "${TARBALL}" --dir "${TMP_DIR}" --clobber 2>/dev/null || true
fi

# 2. Try direct curl download (with optional token for private repos)
if [ ! -f "${TMP_DIR}/${TARBALL}" ]; then
  AUTH_HEADER=()
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    AUTH_HEADER=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
  elif [ -n "${GH_TOKEN:-}" ]; then
    AUTH_HEADER=(-H "Authorization: Bearer ${GH_TOKEN}")
  fi
  curl -fsSL "${AUTH_HEADER[@]}" -H "Accept: application/octet-stream" "${DOWNLOAD_URL}" -o "${TMP_DIR}/${TARBALL}" 2>/dev/null || true
fi

if [ -f "${TMP_DIR}/${TARBALL}" ]; then
  tar -xzf "${TMP_DIR}/${TARBALL}" -C "${TMP_DIR}"
  EXTRACTED_BIN="${TMP_DIR}/${BINARY_NAME}"
  if [ ! -f "$EXTRACTED_BIN" ]; then
    EXTRACTED_BIN="${TMP_DIR}/memos2txt-${OS}-${ARCH}"
  fi
elif [ -f "./cmd/memos2txt/main.go" ]; then
  echo "==> Building directly from local source directory..."
  go build -trimpath -ldflags="-s -w" -o "${TMP_DIR}/${BINARY_NAME}" ./cmd/memos2txt
  EXTRACTED_BIN="${TMP_DIR}/${BINARY_NAME}"
else
  echo "==> Release binary not found, attempting fallback to build from source via Go..."
  if command -v go >/dev/null 2>&1; then
    go install "github.com/${REPO}/cmd/memos2txt@latest"
    echo "✓ Installed via go install to $(go env GOPATH)/bin/${BINARY_NAME}"
    exit 0
  else
    echo "Error: Could not download release binary and Go is not installed." >&2
    exit 1
  fi
fi

INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Check install location write permissions
if [ ! -w "$INSTALL_DIR" ]; then
  if [ -t 0 ] && command -v sudo >/dev/null 2>&1; then
    echo "==> Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo cp "$EXTRACTED_BIN" "${INSTALL_DIR}/${BINARY_NAME}"
    sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
  else
    INSTALL_DIR="${HOME}/.local/bin"
    mkdir -p "$INSTALL_DIR"
    cp "$EXTRACTED_BIN" "${INSTALL_DIR}/${BINARY_NAME}"
    chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
    echo "==> Installed to ${INSTALL_DIR}/${BINARY_NAME}"
    echo "Note: Ensure ${INSTALL_DIR} is in your PATH."
  fi
else
  cp "$EXTRACTED_BIN" "${INSTALL_DIR}/${BINARY_NAME}"
  chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
fi

echo "✓ Successfully installed ${BINARY_NAME} to ${INSTALL_DIR}/${BINARY_NAME}"
"${INSTALL_DIR}/${BINARY_NAME}" -h | head -n 3

echo ""
echo "Next steps:"
echo "  1. Check adapter status: ${BINARY_NAME} auth --list"
echo "  2. Configure API key:    ${BINARY_NAME} auth groq"
echo "  3. Transcribe audio:     ${BINARY_NAME} --provider groq --file path/to/audio.m4a --json"
