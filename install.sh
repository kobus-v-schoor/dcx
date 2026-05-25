#!/usr/bin/env bash
set -euo pipefail

REPO="kobus-v-schoor/dcx"
BINARY="dcx"
INSTALL_DIR="${DCX_INSTALL_DIR:-$HOME/.local/bin}"

msg() { printf ">>> %s\n" "$*" >&2; }
err() { printf "!!! %s\n" "$*" >&2; }

need() {
    if ! command -v "$1" &>/dev/null; then
        err "'$1' is required but not found in PATH"
        exit 1
    fi
}

need curl
need tar
need uname
need mktemp

# sha256 returns the SHA-256 hash of a file, portable across macOS and Linux.
sha256() {
    if command -v sha256sum &>/dev/null; then
        sha256sum "$1" | awk '{print $1}'
    elif command -v shasum &>/dev/null; then
        shasum -a 256 "$1" | awk '{print $1}'
    else
        err "sha256sum or shasum is required but not found in PATH"
        exit 1
    fi
}

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) err "unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
    linux|darwin) ;;
    *) err "unsupported OS: $OS"; exit 1 ;;
esac

msg "fetching latest release tag"

TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name":' \
      | head -1 \
      | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$TAG" ]; then
    err "could not determine latest release tag"
    exit 1
fi

VERSION="${TAG#v}"
ARCHIVE="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"

DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${ARCHIVE}"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${TAG}/checksums.txt"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

msg "downloading ${ARCHIVE}"
curl -fsSL -o "${TMPDIR}/${ARCHIVE}" "$DOWNLOAD_URL"

msg "downloading checksums"
curl -fsSL -o "${TMPDIR}/checksums.txt" "$CHECKSUMS_URL"

EXPECTED=$(grep " ${ARCHIVE}$" "${TMPDIR}/checksums.txt" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
    err "no checksum entry found for ${ARCHIVE}"
    exit 1
fi

ACTUAL=$(sha256 "${TMPDIR}/${ARCHIVE}")
if [ "$ACTUAL" != "$EXPECTED" ]; then
    err "checksum mismatch for ${ARCHIVE}"
    err "  expected: ${EXPECTED}"
    err "  actual:   ${ACTUAL}"
    exit 1
fi

msg "checksum verified"

tar -xzf "${TMPDIR}/${ARCHIVE}" -C "${TMPDIR}" "${BINARY}"

mkdir -p "$INSTALL_DIR"
install -m 755 "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"

msg "installed ${BINARY} ${TAG} to ${INSTALL_DIR}/${BINARY}"

if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
    msg "note: '${INSTALL_DIR}' is not in your PATH"
    msg "  add it with: export PATH=\"${INSTALL_DIR}:\$PATH\""
fi
