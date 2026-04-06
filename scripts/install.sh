#!/usr/bin/env bash
set -euo pipefail

# Local source-build installer for contributors.
# For end users installing published releases, prefer scripts/install-release.sh.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${HOME}/.local/bin"
TARGET="${BIN_DIR}/framescli"

path_has_dir() {
  case ":${PATH}:" in
    *":$1:"*) return 0 ;;
    *) return 1 ;;
  esac
}

shell_profile_path() {
  local shell_name
  shell_name="$(basename "${SHELL:-}")"
  case "$shell_name" in
    bash) printf '%s\n' "${HOME}/.bashrc" ;;
    zsh) printf '%s\n' "${HOME}/.zshrc" ;;
    fish) printf '%s\n' "${HOME}/.config/fish/config.fish" ;;
    *) printf '%s\n' "${HOME}/.profile" ;;
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
  printf '\n%s is not on your PATH.\n' "${install_dir}"
  printf 'Add it now to %s? [Y/n] ' "$(basename "$profile")"
  read -r answer || answer=""
  answer="$(printf '%s' "$answer" | tr '[:upper:]' '[:lower:]')"
  if [[ -n "$answer" && "$answer" != "y" && "$answer" != "yes" ]]; then
    echo "Run this to use framescli without a full path:"
    echo "  ${line}"
    return 1
  fi
  mkdir -p "$(dirname "$profile")"
  if [[ ! -f "$profile" ]] || ! grep -Fq "$line" "$profile"; then
    printf '\n%s\n' "$line" >> "$profile"
    echo "Updated ${profile}"
  fi
  echo "Open a new shell or run: source ${profile}"
}

mkdir -p "${BIN_DIR}"
(
  cd "${ROOT_DIR}"
  go build -o "${TARGET}" ./cmd/frames
)

echo "installed framescli -> ${TARGET}"
ensure_install_dir_on_path "${BIN_DIR}" || true
echo "primary command: framescli"
