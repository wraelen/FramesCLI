#!/usr/bin/env bash
set -euo pipefail

# Download and install a published FramesCLI binary from GitHub Releases.
#
# Usage:
#   ./scripts/install-release.sh
#   ./scripts/install-release.sh --version v0.1.0
#   ./scripts/install-release.sh --install-dir /usr/local/bin
#   ./scripts/install-release.sh --with-deps --with-whisper --yes
#   ./scripts/install-release.sh --print-url

REPO="${FRAMESCLI_REPO:-wraelen/framescli}"
BIN_NAME="framescli"
INSTALL_DIR="${HOME}/.local/bin"
VERSION=""
PRINT_URL="false"
AUTO_YES="false"
INSTALL_DEPS="false"
INSTALL_WHISPER="false"
RUN_DOCTOR="true"
CHECKSUM_FILE="checksums.txt"

usage() {
  cat <<'EOF'
Usage: scripts/install-release.sh [options]

Options:
  --version <tag>         Install a specific release tag (for example: v0.1.0)
  --install-dir <dir>     Directory to place the framescli binary
  --with-deps             Install required media dependencies when possible
  --with-whisper          Install whisper via pipx
  --skip-doctor           Skip the post-install doctor check
  --yes                   Non-interactive install with defaults
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

path_has_dir() {
  case ":${PATH}:" in
    *":$1:"*) return 0 ;;
    *) return 1 ;;
  esac
}

fail() {
  echo "$*" >&2
  exit 1
}

has_tty() {
  [[ -r /dev/tty && -w /dev/tty ]]
}

prompt_yes_no() {
  local question="$1"
  local default_answer="$2"
  local prompt suffix reply

  if [[ "$AUTO_YES" == "true" ]]; then
    [[ "$default_answer" == "y" ]]
    return
  fi
  if ! has_tty; then
    [[ "$default_answer" == "y" ]]
    return
  fi

  if [[ "$default_answer" == "y" ]]; then
    suffix="[Y/n]"
  else
    suffix="[y/N]"
  fi
  prompt="${question} ${suffix} "
  printf "%s" "$prompt" > /dev/tty
  IFS= read -r reply < /dev/tty || reply=""
  reply="$(printf '%s' "$reply" | tr '[:upper:]' '[:lower:]')"
  if [[ -z "$reply" ]]; then
    reply="$default_answer"
  fi
  [[ "$reply" == "y" || "$reply" == "yes" ]]
}

shell_profile_path() {
  local shell_name
  shell_name="$(basename "${SHELL:-}")"
  case "$shell_name" in
    bash)
      if [[ -n "${BASH_VERSION:-}" && -f "${HOME}/.bashrc" ]]; then
        printf '%s\n' "${HOME}/.bashrc"
        return
      fi
      printf '%s\n' "${HOME}/.bashrc"
      ;;
    zsh)
      printf '%s\n' "${HOME}/.zshrc"
      ;;
    fish)
      printf '%s\n' "${HOME}/.config/fish/config.fish"
      ;;
    *)
      printf '%s\n' "${HOME}/.profile"
      ;;
  esac
}

ensure_install_dir_on_path() {
  local install_dir="$1"
  local profile line

  if path_has_dir "$install_dir"; then
    return 0
  fi

  profile="$(shell_profile_path)"
  if [[ "$(basename "$profile")" == "config.fish" ]]; then
    line="fish_add_path ${install_dir}"
  else
    line="export PATH=\"${install_dir}:\$PATH\""
  fi

  echo ""
  echo "${install_dir} is not on your PATH."
  if ! prompt_yes_no "Add it to $(basename "$profile") now?" "y"; then
    echo "Run this after install to use framescli without a full path:"
    echo "  ${line}"
    return 1
  fi

  mkdir -p "$(dirname "$profile")"
  if [[ -f "$profile" ]] && grep -Fq "$line" "$profile"; then
    echo "PATH entry already present in ${profile}"
  else
    printf '\n%s\n' "$line" >> "$profile"
    echo "Updated ${profile}"
  fi

  if [[ "$(basename "$profile")" == "config.fish" ]]; then
    echo "Open a new shell or run: source ${profile}"
  else
    echo "Open a new shell or run: source ${profile}"
  fi
  return 0
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
  local checksum_url="$2"
  local asset_name="$3"
  local os="$4"
  local tmp_dir="$5"
  local archive_path="$tmp_dir/archive"
  local checksum_path="$tmp_dir/${CHECKSUM_FILE}"

  need_cmd curl || fail "curl is required to download release artifacts"
  curl -fL "$asset_url" -o "$archive_path"
  curl -fL "$checksum_url" -o "$checksum_path"
  verify_download_checksum "$asset_name" "$archive_path" "$checksum_path"

  if [[ "$os" == "windows" ]]; then
    need_cmd unzip || fail "unzip is required to extract Windows archives"
    unzip -q "$archive_path" -d "$tmp_dir"
  else
    tar -xzf "$archive_path" -C "$tmp_dir"
  fi
}

verify_download_checksum() {
  local asset_name="$1"
  local archive_path="$2"
  local checksum_path="$3"
  local expected actual

  expected="$(awk -v asset="$asset_name" '$2 == asset { print $1 }' "$checksum_path" | head -n1)"
  [[ -n "$expected" ]] || fail "failed to find checksum for ${asset_name} in ${CHECKSUM_FILE}"

  if need_cmd sha256sum; then
    actual="$(sha256sum "$archive_path" | awk '{print $1}')"
  elif need_cmd shasum; then
    actual="$(shasum -a 256 "$archive_path" | awk '{print $1}')"
  else
    fail "sha256sum or shasum is required to verify release artifacts"
  fi
  [[ "$actual" == "$expected" ]] || fail "checksum mismatch for ${asset_name}"
}

can_sudo() {
  if need_cmd sudo; then
    sudo -n true >/dev/null 2>&1
    return $?
  fi
  return 1
}

run_step() {
  local cmd="$1"
  echo "+ $cmd"
  eval "$cmd"
}

install_media_deps() {
  local os id id_like
  os="$(uname -s)"
  case "$os" in
    Darwin)
      need_cmd brew || fail "Homebrew not found. Install it first from https://brew.sh"
      run_step "brew install ffmpeg"
      return
      ;;
    Linux)
      id=""
      id_like=""
      if [[ -f /etc/os-release ]]; then
        # shellcheck disable=SC1091
        source /etc/os-release
        id="${ID:-}"
        id_like="${ID_LIKE:-}"
      fi
      if [[ "$id" == "ubuntu" || "$id" == "debian" || "$id_like" == *"debian"* ]]; then
        can_sudo || fail "sudo access not available; install ffmpeg manually"
        run_step "sudo apt-get update"
        run_step "sudo apt-get install -y ffmpeg"
        return
      fi
      if [[ "$id" == "fedora" || "$id_like" == *"fedora"* || "$id_like" == *"rhel"* ]]; then
        can_sudo || fail "sudo access not available; install ffmpeg manually"
        run_step "sudo dnf install -y ffmpeg"
        return
      fi
      if [[ "$id" == "arch" || "$id_like" == *"arch"* ]]; then
        can_sudo || fail "sudo access not available; install ffmpeg manually"
        run_step "sudo pacman -S --noconfirm ffmpeg"
        return
      fi
      if [[ "$id" == "opensuse-tumbleweed" || "$id" == "opensuse-leap" || "$id_like" == *"suse"* ]]; then
        can_sudo || fail "sudo access not available; install ffmpeg manually"
        run_step "sudo zypper install -y ffmpeg"
        return
      fi
      fail "unsupported Linux distro for auto-install; install ffmpeg and ffprobe manually"
      ;;
    *)
      fail "auto-install not supported on $(uname -s); install ffmpeg and ffprobe manually"
      ;;
  esac
}

install_whisper_dep() {
  need_cmd python3 || fail "python3 is required to install whisper"
  run_step "python3 -m pip install --user pipx"
  run_step "python3 -m pipx ensurepath"
  run_step "pipx install openai-whisper || pipx upgrade openai-whisper"
}

run_post_install() {
  local target="$1"

  echo ""
  echo "FramesCLI installed."
  echo "Binary: ${target}"
  ensure_install_dir_on_path "${INSTALL_DIR}" || true

  if [[ "$INSTALL_DEPS" != "true" ]] && prompt_yes_no "Install required media dependencies (ffmpeg/ffprobe) now?" "y"; then
    INSTALL_DEPS="true"
  fi
  if [[ "$INSTALL_DEPS" == "true" ]]; then
    install_media_deps
  fi

  if [[ "$INSTALL_WHISPER" != "true" ]] && prompt_yes_no "Install whisper for transcription workflows?" "n"; then
    INSTALL_WHISPER="true"
  fi
  if [[ "$INSTALL_WHISPER" == "true" ]]; then
    install_whisper_dep
  fi

  if [[ "$RUN_DOCTOR" == "true" ]]; then
    echo ""
    echo "Running framescli doctor..."
    "$target" doctor < /dev/tty > /dev/tty 2>&1 || true
  fi

  echo ""
  echo "Next: framescli extract <video.mp4>"
  echo "Help: framescli --help"
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
    --with-deps)
      INSTALL_DEPS="true"
      shift
      ;;
    --with-whisper)
      INSTALL_WHISPER="true"
      shift
      ;;
    --skip-doctor)
      RUN_DOCTOR="false"
      shift
      ;;
    --yes)
      AUTO_YES="true"
      shift
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
CHECKSUM_URL="https://github.com/${REPO}/releases/download/${TAG}/${CHECKSUM_FILE}"

if [[ "$PRINT_URL" == "true" ]]; then
  printf '%s\n' "$ASSET_URL"
  exit 0
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo "Resolving release: ${TAG}"
echo "Asset: ${ASSET_NAME}"
echo "Install dir: ${INSTALL_DIR}"

download_and_extract "$ASSET_URL" "$CHECKSUM_URL" "$ASSET_NAME" "$OS_NAME" "$TMP_DIR"

SOURCE_BIN="$(find "$TMP_DIR" -type f \( -name "${BIN_NAME}" -o -name "${BIN_NAME}.exe" \) | head -n1)"
[[ -n "$SOURCE_BIN" ]] || fail "failed to find extracted ${BIN_NAME} binary in release archive"

mkdir -p "$INSTALL_DIR"
TARGET_PATH="${INSTALL_DIR}/${BIN_NAME}"
if [[ "$OS_NAME" == "windows" ]]; then
  TARGET_PATH="${INSTALL_DIR}/${BIN_NAME}.exe"
fi

install -m 0755 "$SOURCE_BIN" "$TARGET_PATH"

echo "Installed ${BIN_NAME} -> ${TARGET_PATH}"
if [[ "$AUTO_YES" == "true" ]]; then
  echo "Verify with: ${BIN_NAME} --help"
  if ! path_has_dir "${INSTALL_DIR}"; then
    echo "Note: ${INSTALL_DIR} is not on your PATH in this shell."
  fi
  exit 0
fi

if has_tty; then
  run_post_install "$TARGET_PATH"
else
  echo "Verify with: ${BIN_NAME} --help"
  echo "Then run: ${BIN_NAME} doctor"
  echo "Then run: ${BIN_NAME} setup"
fi
