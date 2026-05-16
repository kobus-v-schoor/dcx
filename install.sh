#!/usr/bin/env bash
set -euo pipefail

REPO="kobus-v-schoor/dcx"
BINARY="dcx"
INSTALL_DIR="${DCX_INSTALL_DIR:-$HOME/.local/bin}"

GITHUB_TOKEN="${GITHUB_TOKEN:-}"

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

github_api() {
    local url="$1"
    shift
    if [ -n "$GITHUB_TOKEN" ]; then
        curl -fsSL -H "Authorization: Bearer ${GITHUB_TOKEN}" -H "Accept: application/vnd.github+json" "$url" "$@"
    else
        curl -fsSL -H "Accept: application/vnd.github+json" "$url" "$@"
    fi
}

github_download() {
    local url="$1"
    shift
    if [ -n "$GITHUB_TOKEN" ]; then
        curl -fsSL -H "Authorization: Bearer ${GITHUB_TOKEN}" -H "Accept: application/octet-stream" "$url" "$@"
    else
        curl -fsSL "$url" "$@"
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

msg "detecting latest release for ${OS}/${ARCH}"

RELEASE_JSON=$(github_api "https://api.github.com/repos/${REPO}/releases/latest")

TAG=$(printf '%s' "$RELEASE_JSON" | grep '"tag_name":' | head -1 | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$TAG" ]; then
    err "could not determine latest release tag"
    exit 1
fi

VERSION="${TAG#v}"
ARCHIVE="${BINARY}_${VERSION}_${OS}_${ARCH}.tar.gz"

ASSET_ID=$(printf '%s' "$RELEASE_JSON" | python3 -c "
import sys, json
release = json.load(sys.stdin)
for asset in release.get('assets', []):
    if asset['name'] == '${ARCHIVE}':
        print(asset['id'])
        break
" 2>/dev/null || true)

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

if [ -n "$ASSET_ID" ]; then
    ASSET_URL="https://api.github.com/repos/${REPO}/releases/assets/${ASSET_ID}"
    CHECKSUM_ID=$(printf '%s' "$RELEASE_JSON" | python3 -c "
import sys, json
release = json.load(sys.stdin)
for asset in release.get('assets', []):
    if asset['name'] == 'checksums.txt':
        print(asset['id'])
        break
" 2>/dev/null || true)
    CHECKSUM_URL="https://api.github.com/repos/${REPO}/releases/assets/${CHECKSUM_ID}"

    msg "downloading ${ARCHIVE} (via API)"
    github_download -o "${TMPDIR}/${ARCHIVE}" "$ASSET_URL"

    msg "downloading checksums (via API)"
    github_download -o "${TMPDIR}/checksums.txt" "$CHECKSUM_URL"
else
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/${ARCHIVE}"
    CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${TAG}/checksums.txt"

    msg "downloading ${ARCHIVE}"
    github_download -o "${TMPDIR}/${ARCHIVE}" "$DOWNLOAD_URL"

    msg "downloading checksums"
    github_download -o "${TMPDIR}/checksums.txt" "$CHECKSUMS_URL"
fi

EXPECTED=$(grep " ${ARCHIVE}$" "${TMPDIR}/checksums.txt" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
    err "no checksum entry found for ${ARCHIVE}"
    exit 1
fi

ACTUAL=$(shasum -a 256 "${TMPDIR}/${ARCHIVE}" | awk '{print $1}')
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
