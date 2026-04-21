#!/usr/bin/env bash
# Generate agent-facing schema/docs artifacts from the Go contract registry.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SCHEMAS_DIR="$REPO_ROOT/docs/schemas"
GO_CACHE_DIR="${GOCACHE:-$REPO_ROOT/.cache/go-build}"

echo "Generating agent-facing schemas and docs..."
mkdir -p "$GO_CACHE_DIR"

TMP_OUT="$(mktemp)"
trap 'rm -f "$TMP_OUT"' EXIT

GOCACHE="$GO_CACHE_DIR" go run ./cmd/contractgen >"$TMP_OUT"

echo "✓ Generation complete"
echo "  Location: $SCHEMAS_DIR"
echo "  Wrote:"
sed 's/^/  - /' "$TMP_OUT"
