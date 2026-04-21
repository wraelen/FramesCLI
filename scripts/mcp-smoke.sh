#!/usr/bin/env bash
set -euo pipefail

# Basic MCP stdio smoke test for FramesCLI.
# Usage:
#   ./scripts/mcp-smoke.sh [framescli-binary]
#
# Exit non-zero if any MCP roundtrip fails expected checks.

BIN="${1:-./bin/framescli}"

if [[ ! -x "$BIN" ]]; then
  echo "framescli binary not found or not executable at: $BIN" >&2
  echo "Build first: go build -o bin/framescli ./cmd/frames" >&2
  exit 1
fi

send_req() {
  local init='{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2024-11-05","clientInfo":{"name":"framescli-mcp-smoke","version":"1.0"},"capabilities":{}}}'
  local ready='{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}'
  {
    printf '%s\n' "$init"
    printf '%s\n' "$ready"
    while [[ $# -gt 0 ]]; do
      printf '%s\n' "$1"
      shift
    done
    # Keep stdin open briefly so async tools/call responses can flush.
    sleep 1
  } | "$BIN" mcp
}

check_contains() {
  local haystack="$1"
  local needle="$2"
  local label="$3"
  if ! printf '%s' "$haystack" | grep -Eq "$needle"; then
    echo "FAIL: $label (missing pattern: $needle)" >&2
    echo "Response was:" >&2
    printf '%s\n' "$haystack" >&2
    exit 1
  fi
}

echo "[mcp-smoke] tools/list"
REQ_TOOLS='{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'
RESP_TOOLS="$(send_req "$REQ_TOOLS")"
check_contains "$RESP_TOOLS" '"tools"' "tools/list should return tools"
check_contains "$RESP_TOOLS" '"extract"' "tools/list should include extract"

echo "[mcp-smoke] doctor"
REQ_DOCTOR='{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"doctor","arguments":{}}}'
RESP_DOCTOR="$(send_req "$REQ_DOCTOR")"
check_contains "$RESP_DOCTOR" '"status"[[:space:]]*:[[:space:]]*"success"' "doctor should succeed"

echo "[mcp-smoke] prefs_get"
REQ_PREFS='{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"prefs_get","arguments":{}}}'
RESP_PREFS="$(send_req "$REQ_PREFS")"
check_contains "$RESP_PREFS" '"input_dirs"' "prefs_get should include input_dirs"
check_contains "$RESP_PREFS" '"output_root"' "prefs_get should include output_root"

echo "[mcp-smoke] done"
