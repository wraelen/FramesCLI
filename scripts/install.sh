#!/usr/bin/env bash
set -euo pipefail

# Local source-build installer for contributors.
# For end users installing published releases, prefer scripts/install-release.sh.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${HOME}/.local/bin"
TARGET="${BIN_DIR}/framescli"

mkdir -p "${BIN_DIR}"
(
  cd "${ROOT_DIR}"
  go build -o "${TARGET}" ./cmd/frames
)

echo "installed framescli -> ${TARGET}"
echo "ensure ${BIN_DIR} is on your PATH"
echo "primary command: framescli"
