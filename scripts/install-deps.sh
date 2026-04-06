#!/usr/bin/env bash
set -euo pipefail

# FramesCLI dependency bootstrapper.
# Installs required media tools and optionally whisper.
#
# Usage:
#   ./scripts/install-deps.sh --print
#   ./scripts/install-deps.sh --install
#   ./scripts/install-deps.sh --install --with-whisper

MODE="print"
WITH_WHISPER="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --print)
      MODE="print"
      shift
      ;;
    --install)
      MODE="install"
      shift
      ;;
    --with-whisper)
      WITH_WHISPER="true"
      shift
      ;;
    -h|--help)
      cat <<'EOF'
Usage: scripts/install-deps.sh [--print|--install] [--with-whisper]

  --print         Print platform-specific install commands (default)
  --install       Execute install commands when possible
  --with-whisper  Also install whisper via pipx
EOF
      exit 0
      ;;
    *)
      echo "Unknown flag: $1" >&2
      exit 2
      ;;
  esac
done

need_cmd() {
  command -v "$1" >/dev/null 2>&1
}

run_or_print() {
  local cmd="$1"
  if [[ "$MODE" == "install" ]]; then
    echo "+ $cmd"
    eval "$cmd"
  else
    echo "$cmd"
  fi
}

can_sudo() {
  if need_cmd sudo; then
    sudo -n true >/dev/null 2>&1
    return $?
  fi
  return 1
}

OS="$(uname -s)"

install_linux_ffmpeg() {
  local id_like id
  id=""
  id_like=""
  if [[ -f /etc/os-release ]]; then
    # shellcheck disable=SC1091
    source /etc/os-release
    id="${ID:-}"
    id_like="${ID_LIKE:-}"
  fi

  if [[ "$id" == "ubuntu" || "$id" == "debian" || "$id_like" == *"debian"* ]]; then
    if can_sudo; then
      run_or_print "sudo apt-get update"
      run_or_print "sudo apt-get install -y ffmpeg"
    else
      echo "sudo access not available; run manually:" >&2
      echo "  sudo apt-get update && sudo apt-get install -y ffmpeg" >&2
    fi
    return
  fi

  if [[ "$id" == "fedora" || "$id_like" == *"fedora"* || "$id_like" == *"rhel"* ]]; then
    if can_sudo; then
      run_or_print "sudo dnf install -y ffmpeg"
    else
      echo "sudo access not available; run manually:" >&2
      echo "  sudo dnf install -y ffmpeg" >&2
    fi
    return
  fi

  if [[ "$id" == "arch" || "$id_like" == *"arch"* ]]; then
    if can_sudo; then
      run_or_print "sudo pacman -S --noconfirm ffmpeg"
    else
      echo "sudo access not available; run manually:" >&2
      echo "  sudo pacman -S --noconfirm ffmpeg" >&2
    fi
    return
  fi

  if [[ "$id" == "opensuse-tumbleweed" || "$id" == "opensuse-leap" || "$id_like" == *"suse"* ]]; then
    if can_sudo; then
      run_or_print "sudo zypper install -y ffmpeg"
    else
      echo "sudo access not available; run manually:" >&2
      echo "  sudo zypper install -y ffmpeg" >&2
    fi
    return
  fi

  echo "Unsupported Linux distro for auto-install. Install ffmpeg/ffprobe manually." >&2
}

install_whisper() {
  if ! need_cmd python3; then
    echo "python3 not found; skipping whisper install" >&2
    return
  fi
  run_or_print "python3 -m pip install --user pipx"
  run_or_print "python3 -m pipx ensurepath"
  run_or_print "pipx install openai-whisper || pipx upgrade openai-whisper"
}

echo "FramesCLI dependency bootstrap ($MODE)"
echo "Target OS: $OS"

case "$OS" in
  Darwin)
    if ! need_cmd brew; then
      echo "Homebrew not found. Install from https://brew.sh first." >&2
      exit 1
    fi
    run_or_print "brew install ffmpeg"
    ;;
  Linux)
    install_linux_ffmpeg
    ;;
  MINGW*|MSYS*|CYGWIN*)
    echo "Windows shell detected; install ffmpeg manually (winget/choco/scoop), then run framescli doctor."
    ;;
  *)
    echo "Unsupported OS: $OS. Install ffmpeg and ffprobe manually, then run framescli doctor." >&2
    ;;
esac

if [[ "$WITH_WHISPER" == "true" ]]; then
  install_whisper
else
  echo "Whisper install skipped. Re-run with --with-whisper to install transcription dependency."
fi

echo "Verify:"
echo "  framescli doctor"
