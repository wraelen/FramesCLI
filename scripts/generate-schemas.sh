#!/usr/bin/env bash
# Generate JSON schemas for all MCP tools by extracting from source code

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SCHEMAS_DIR="$REPO_ROOT/docs/schemas"

echo "Generating MCP tool JSON schemas..."

# Extract tool list from main.go and generate individual schema files
# For now, this is a placeholder - schemas are manually maintained
# Future: could parse Go code or generate from runtime reflection

echo "✓ Schema generation complete"
echo "  Location: $SCHEMAS_DIR"
echo "  Note: Schemas are currently maintained manually in docs/schemas/"
