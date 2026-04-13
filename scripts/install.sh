#!/usr/bin/env sh
set -eu

REPO="withObsrvr/nebu"
BIN="nebu"
INSTALL_DIR="${INSTALL_DIR:-${HOME}/.local/bin}"
VERSION="${VERSION:-latest}"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: required command not found: $1" >&2
    exit 1
  fi
}

need curl
need tar
need uname
need mktemp

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)

case "$os" in
  linux|darwin) ;;
  *)
    echo "error: unsupported OS: $os" >&2
    exit 1
    ;;
esac

case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *)
    echo "error: unsupported architecture: $arch" >&2
    exit 1
    ;;
esac

if [ "$VERSION" = "latest" ]; then
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | \
    grep '"tag_name":' | head -n1 | sed -E 's/.*"([^"]+)".*/\1/')
fi

if [ -z "$VERSION" ]; then
  echo "error: failed to resolve release version" >&2
  exit 1
fi

archive="${BIN}_${VERSION}_${os}_${arch}.tar.gz"
url="https://github.com/${REPO}/releases/download/${VERSION}/${archive}"

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT INT TERM

echo "Installing ${BIN} ${VERSION} for ${os}/${arch}..."
echo "Downloading ${url}"

curl -fL "$url" -o "$tmpdir/$archive"
mkdir -p "$INSTALL_DIR"
tar -xzf "$tmpdir/$archive" -C "$tmpdir"
install "$tmpdir/$BIN" "$INSTALL_DIR/$BIN"

echo "Installed to $INSTALL_DIR/$BIN"
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo ""
    echo "Add this to your shell profile if needed:"
    echo "  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac

echo ""
echo "Next steps:"
echo "  nebu install token-transfer"
echo "  token-transfer --start-ledger 60200000 --end-ledger 60200001"
