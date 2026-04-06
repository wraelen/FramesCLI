#!/usr/bin/env bash
set -euo pipefail

# Download and install a published FramesCLI binary from GitHub Releases.
#
# Usage:
#   ./scripts/install-release.sh
#   ./scripts/install-release.sh --version v0.1.0
#   ./scripts/install-release.sh --install-dir /usr/local/bin
#   ./scripts/install-release.sh --print-url

REPO="${FRAMESCLI_REPO:-wraelen/framescli}"
BIN_NAME="framescli"
INSTALL_DIR="${HOME}/.local/bin"
VERSION=""
PRINT_URL="false"

usage() {
  cat <<'EOF'
Usage: scripts/install-release.sh [options]

Options:
  --version <tag>         Install a specific release tag (for example: v0.1.0)
  --install-dir <dir>     Directory to place the framescli binary
  --print-url             Print the resolved release asset URL and exit
  -h, --help              Show this help message

Environment:
  FRAMESCLI_REPO          Override the GitHub repo slug (default: wraelen/framescli)

Notes:
  - Default install dir is ~/.local/bin
  - This script installs prebuilt release binaries; it does not compile from source
EOF
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1
}

fail() {
  echo "$*" >&2
  exit 1
}

detect_os() {
  case "$(uname -s)" in
    Linux) echo "linux" ;;
    Darwin) echo "darwin" ;;
    MINGW*|MSYS*|CYGWIN*) echo "windows" ;;
    *) fail "unsupported OS: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) echo "amd64" ;;
    arm64|aarch64) echo "arm64" ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
  esac
}

resolve_version() {
  if [[ -n "$VERSION" ]]; then
    echo "$VERSION"
    return
  fi
  need_cmd curl || fail "curl is required to resolve the latest release"
  local api_url="https://api.github.com/repos/${REPO}/releases/latest"
  local tag
  tag="$(
    curl -fsSL "$api_url" | \
      sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | \
      head -n1
  )"
  [[ -n "$tag" ]] || fail "failed to resolve latest release tag from ${api_url}"
  echo "$tag"
}

build_asset_name() {
  local tag="$1"
  local os="$2"
  local arch="$3"
  local version="${tag#v}"
  local ext="tar.gz"
  if [[ "$os" == "windows" ]]; then
    ext="zip"
  fi
  echo "${BIN_NAME}_${version}_${os}_${arch}.${ext}"
}

download_and_extract() {
  local asset_url="$1"
  local os="$2"
  local tmp_dir="$3"
  local archive_path="$tmp_dir/archive"

  need_cmd curl || fail "curl is required to download release artifacts"
  curl -fL "$asset_url" -o "$archive_path"

  if [[ "$os" == "windows" ]]; then
    need_cmd unzip || fail "unzip is required to extract Windows archives"
    unzip -q "$archive_path" -d "$tmp_dir"
  else
    tar -xzf "$archive_path" -C "$tmp_dir"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --install-dir)
      INSTALL_DIR="${2:-}"
      shift 2
      ;;
    --print-url)
      PRINT_URL="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

OS_NAME="$(detect_os)"
ARCH_NAME="$(detect_arch)"
TAG="$(resolve_version)"
ASSET_NAME="$(build_asset_name "$TAG" "$OS_NAME" "$ARCH_NAME")"
ASSET_URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET_NAME}"

if [[ "$PRINT_URL" == "true" ]]; then
  printf '%s\n' "$ASSET_URL"
  exit 0
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Resolving release: ${TAG}"
echo "Asset: ${ASSET_NAME}"
echo "Install dir: ${INSTALL_DIR}"

download_and_extract "$ASSET_URL" "$OS_NAME" "$TMP_DIR"

SOURCE_BIN="$(find "$TMP_DIR" -type f \( -name "${BIN_NAME}" -o -name "${BIN_NAME}.exe" \) | head -n1)"
[[ -n "$SOURCE_BIN" ]] || fail "failed to find extracted ${BIN_NAME} binary in release archive"

mkdir -p "$INSTALL_DIR"
TARGET_PATH="${INSTALL_DIR}/${BIN_NAME}"
if [[ "$OS_NAME" == "windows" ]]; then
  TARGET_PATH="${INSTALL_DIR}/${BIN_NAME}.exe"
fi

install -m 0755 "$SOURCE_BIN" "$TARGET_PATH"

echo "Installed ${BIN_NAME} -> ${TARGET_PATH}"
echo "Verify with: ${BIN_NAME} --help"
