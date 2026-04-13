#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO="${FRAMESCLI_REPO:-wraelen/framescli}"
SOURCE_MODE="dist"
DIST_DIR="${ROOT_DIR}/dist"
VERSION=""
TMP_DIR=""
BIN_NAME="framescli"
CHECKSUM_FILE="checksums.txt"

usage() {
  cat <<'EOF'
Usage: scripts/release-verify.sh [options]

Validate FramesCLI release artifacts either from local GoReleaser output or from a
published GitHub release.

Options:
  --version <tag-or-version>  Release version to verify. Required for --source github.
  --source <dist|github>      Artifact source to verify (default: dist)
  --dist-dir <dir>            Local dist directory for --source dist (default: ./dist)
  -h, --help                  Show this help message

Examples:
  ./scripts/release-verify.sh --source dist --dist-dir ./dist
  ./scripts/release-verify.sh --source github --version v0.1.0
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

archive_ext() {
  local os_name="$1"
  if [[ "$os_name" == "windows" ]]; then
    printf '%s\n' "zip"
  else
    printf '%s\n' "tar.gz"
  fi
}

build_asset_name() {
  local version="$1"
  local os_name="$2"
  local arch_name="$3"
  printf '%s_%s_%s_%s.%s\n' \
    "$BIN_NAME" \
    "${version#v}" \
    "$os_name" \
    "$arch_name" \
    "$(archive_ext "$os_name")"
}

collect_expected_assets() {
  local version="$1"
  local os_name arch_name
  for os_name in linux darwin windows; do
    for arch_name in amd64 arm64; do
      build_asset_name "$version" "$os_name" "$arch_name"
    done
  done
}

infer_local_version() {
  local first_asset
  first_asset="$(find "$DIST_DIR" -maxdepth 1 -type f -name "${BIN_NAME}_*" | sort | head -n1)"
  [[ -n "$first_asset" ]] || fail "failed to infer version from ${DIST_DIR}; pass --version"
  first_asset="$(basename "$first_asset")"
  first_asset="${first_asset#${BIN_NAME}_}"
  first_asset="${first_asset%.*}"
  first_asset="${first_asset%.*}"
  printf '%s\n' "${first_asset%_*_*}"
}

download_release_assets() {
  local version="$1"
  local asset_name

  need_cmd curl || fail "curl is required for --source github"
  TMP_DIR="$(mktemp -d)"
  trap 'rm -rf "$TMP_DIR"' EXIT

  curl -fsSL \
    "https://github.com/${REPO}/releases/download/${version}/${CHECKSUM_FILE}" \
    -o "${TMP_DIR}/${CHECKSUM_FILE}"

  while IFS= read -r asset_name; do
    curl -fsSL \
      "https://github.com/${REPO}/releases/download/${version}/${asset_name}" \
      -o "${TMP_DIR}/${asset_name}"
  done < <(collect_expected_assets "$version")
}

verify_checksums() {
  local dir="$1"
  (
    cd "$dir"
    if need_cmd sha256sum; then
      sha256sum -c "$CHECKSUM_FILE"
    elif need_cmd shasum; then
      shasum -a 256 -c "$CHECKSUM_FILE"
    else
      fail "sha256sum or shasum is required to verify release checksums"
    fi
  )
}

list_zip_archive() {
  local archive_path="$1"
  if need_cmd unzip; then
    unzip -Z1 "$archive_path"
  elif need_cmd python3; then
    python3 -m zipfile -l "$archive_path" | awk 'NR > 1 {print $1}'
  else
    fail "unzip or python3 is required to inspect Windows archives"
  fi
}

extract_zip_archive() {
  local archive_path="$1"
  local out_dir="$2"
  if need_cmd unzip; then
    unzip -q "$archive_path" -d "$out_dir"
  elif need_cmd python3; then
    python3 -m zipfile -e "$archive_path" "$out_dir" >/dev/null
  else
    fail "unzip or python3 is required to extract Windows archives"
  fi
}

assert_archive_contains() {
  local archive_path="$1"
  local pattern="$2"
  local kind="$3"

  if [[ "$archive_path" == *.zip ]]; then
    list_zip_archive "$archive_path" | grep -Eq "$pattern" || fail "${archive_path} missing ${kind}"
  else
    tar -tzf "$archive_path" | grep -Eq "$pattern" || fail "${archive_path} missing ${kind}"
  fi
}

verify_archive_layout() {
  local dir="$1"
  local asset_name asset_path

  while IFS= read -r asset_name; do
    asset_path="${dir}/${asset_name}"
    [[ -f "$asset_path" ]] || fail "missing artifact: ${asset_path}"
    assert_archive_contains "$asset_path" '(^|/|\\)README\.md$' "README.md"
    assert_archive_contains "$asset_path" '(^|/|\\)LICENSE$' "LICENSE"
    if [[ "$asset_name" == *.zip ]]; then
      assert_archive_contains "$asset_path" '(^|/|\\)framescli\.exe$' "framescli.exe"
    else
      assert_archive_contains "$asset_path" '(^|/|\\)framescli$' "framescli"
    fi
  done < <(collect_expected_assets "$VERSION")
}

verify_installer_target() {
  local os_name arch_name expected_url actual_url
  os_name="$(detect_os)"
  arch_name="$(detect_arch)"
  expected_url="https://github.com/${REPO}/releases/download/${VERSION}/$(build_asset_name "$VERSION" "$os_name" "$arch_name")"
  actual_url="$(bash "${ROOT_DIR}/scripts/install-release.sh" --version "$VERSION" --print-url)"
  [[ "$actual_url" == "$expected_url" ]] || fail "install-release.sh resolved ${actual_url}, expected ${expected_url}"
}

extract_current_platform_binary() {
  local dir="$1"
  local os_name arch_name asset_name asset_path out_dir

  os_name="$(detect_os)"
  arch_name="$(detect_arch)"
  asset_name="$(build_asset_name "$VERSION" "$os_name" "$arch_name")"
  asset_path="${dir}/${asset_name}"
  out_dir="${dir}/runtime-check"
  rm -rf "$out_dir"
  mkdir -p "$out_dir"

  if [[ "$asset_name" == *.zip ]]; then
    extract_zip_archive "$asset_path" "$out_dir"
    find "$out_dir" -type f -name "${BIN_NAME}.exe" | head -n1
  else
    tar -xzf "$asset_path" -C "$out_dir"
    find "$out_dir" -type f -name "${BIN_NAME}" | head -n1
  fi
}

verify_runtime_smoke() {
  local dir="$1"
  local binary_path

  binary_path="$(extract_current_platform_binary "$dir")"
  [[ -n "$binary_path" ]] || fail "failed to extract current-platform binary"
  chmod +x "$binary_path"

  "$binary_path" --help >/dev/null
  "$binary_path" doctor --help >/dev/null
  "$binary_path" completion bash >/dev/null
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --source)
      SOURCE_MODE="${2:-}"
      shift 2
      ;;
    --dist-dir)
      DIST_DIR="${2:-}"
      shift 2
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

case "$SOURCE_MODE" in
  dist|github) ;;
  *) fail "unsupported --source value: ${SOURCE_MODE}" ;;
esac

if [[ "$SOURCE_MODE" == "dist" ]]; then
  [[ -d "$DIST_DIR" ]] || fail "dist directory not found: ${DIST_DIR}"
  if [[ -z "$VERSION" ]]; then
    VERSION="$(infer_local_version)"
  fi
  verify_checksums "$DIST_DIR"
  verify_archive_layout "$DIST_DIR"
  verify_installer_target
  verify_runtime_smoke "$DIST_DIR"
else
  [[ -n "$VERSION" ]] || fail "--version is required for --source github"
  download_release_assets "$VERSION"
  verify_checksums "$TMP_DIR"
  verify_archive_layout "$TMP_DIR"
  verify_installer_target
  verify_runtime_smoke "$TMP_DIR"
fi

echo "[release-verify] OK"
echo "[release-verify] source=${SOURCE_MODE} version=${VERSION}"
