#!/bin/sh
set -eu

REPO="rybkr/gitvista"
BINARY="gitvista"
INSTALL_NAME="git-vista"
INSTALL_DIR="${GITVISTA_INSTALL_DIR:-/usr/local/bin}"

info()  { printf '  \033[1;34m[>]\033[0m %s\n' "$*"; }
err()   { printf '  \033[1;31m[!]\033[0m %s\n' "$*" >&2; exit 1; }
ok()    { printf '  \033[1;32m[+]\033[0m %s\n' "$*"; }

need() {
    command -v "$1" >/dev/null 2>&1 || err "Required command not found: $1"
}

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux"  ;;
        Darwin*) echo "darwin" ;;
        *)       err "Unsupported OS: $(uname -s)" ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "amd64"  ;;
        aarch64|arm64) echo "arm64"  ;;
        *)             err "Unsupported architecture: $(uname -m)" ;;
    esac
}

latest_version() {
    need curl
    url="https://api.github.com/repos/${REPO}/releases/latest"
    version=$(curl -fSsL "$url" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"//;s/".*//')
    [ -n "$version" ] || err "Could not determine latest version"
    echo "$version"
}

install() {
    OS=$(detect_os)
    ARCH=$(detect_arch)

    info "Detected platform: ${OS}/${ARCH}"

    VERSION="${GITVISTA_VERSION:-$(latest_version)}"
    info "Installing ${BINARY} ${VERSION}"

    TARBALL="${BINARY}_${VERSION#v}_${OS}_${ARCH}.tar.gz"
    URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"

    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"' EXIT

    info "Downloading ${URL}"
    need curl
    curl -fSsL "$URL" -o "${TMPDIR}/${TARBALL}" || err "Download failed — does release ${VERSION} exist?"

    # Checksum verification
    if [ "${GITVISTA_SKIP_CHECKSUM:-}" != "true" ]; then
        CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"
        info "Downloading checksums"
        curl -fSsL "$CHECKSUMS_URL" -o "${TMPDIR}/checksums.txt" || err "Failed to download checksums file"

        EXPECTED=$(grep "${TARBALL}" "${TMPDIR}/checksums.txt" | awk '{print $1}')
        [ -n "$EXPECTED" ] || err "Checksum not found for ${TARBALL} in checksums.txt"

        if command -v sha256sum >/dev/null 2>&1; then
            ACTUAL=$(sha256sum "${TMPDIR}/${TARBALL}" | awk '{print $1}')
        elif command -v shasum >/dev/null 2>&1; then
            ACTUAL=$(shasum -a 256 "${TMPDIR}/${TARBALL}" | awk '{print $1}')
        else
            err "No SHA-256 tool found (need sha256sum or shasum)"
        fi

        if [ "$EXPECTED" != "$ACTUAL" ]; then
            err "Checksum mismatch for ${TARBALL}\n  expected: ${EXPECTED}\n  actual:   ${ACTUAL}"
        fi
        ok "Checksum verified"
    else
        info "Skipping checksum verification (GITVISTA_SKIP_CHECKSUM=true)"
    fi

    need tar
    tar -xzf "${TMPDIR}/${TARBALL}" -C "$TMPDIR"

    # Find the binary inside the extracted archive
    BIN=$(find "$TMPDIR" -name "$BINARY" -type f | head -1)
    [ -n "$BIN" ] || err "Binary not found in archive"

    chmod +x "$BIN"

    if [ -w "$INSTALL_DIR" ]; then
        mv "$BIN" "${INSTALL_DIR}/${INSTALL_NAME}"
    else
        info "Elevated permissions required to install to ${INSTALL_DIR}"
        sudo mv "$BIN" "${INSTALL_DIR}/${INSTALL_NAME}"
    fi

    ok "Installed ${INSTALL_NAME} ${VERSION} to ${INSTALL_DIR}/${INSTALL_NAME}"
    echo ""
    echo "  Run 'git vista' inside any repository to visualize it."
    echo ""
}

install
