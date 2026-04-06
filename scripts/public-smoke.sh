#!/usr/bin/env bash
set -euo pipefail

# Public beta smoke test for FramesCLI.
# Usage:
#   ./scripts/public-smoke.sh [--video /abs/path/to/video.mp4] [--no-mcp]
#
# If --video is not provided, a short synthetic sample video is generated.

VIDEO_PATH=""
RUN_MCP="true"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --video)
      VIDEO_PATH="${2:-}"
      shift 2
      ;;
    --no-mcp)
      RUN_MCP="false"
      shift
      ;;
    -h|--help)
      cat <<'EOF'
Usage: scripts/public-smoke.sh [--video /abs/path/to/video.mp4] [--no-mcp]

Runs:
  1) framescli doctor
  2) framescli preview --json
  3) framescli extract --json
  4) framescli extract-batch --json
  5) framescli open-last --artifact run
  6) scripts/mcp-smoke.sh (unless --no-mcp)
EOF
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"
export PATH="$ROOT_DIR/bin:$PATH"

BIN="${FRAMESCLI_BIN:-./bin/framescli}"
if [[ ! -x "$BIN" ]]; then
  echo "[public-smoke] building framescli"
  go build -o ./bin/framescli ./cmd/frames
fi

if ! command -v ffmpeg >/dev/null 2>&1; then
  echo "[public-smoke] ffmpeg not found. Run ./scripts/install-deps.sh --install" >&2
  exit 1
fi

if ! command -v ffprobe >/dev/null 2>&1; then
  echo "[public-smoke] ffprobe not found. Run ./scripts/install-deps.sh --install" >&2
  exit 1
fi

TMP_DIR="$ROOT_DIR/tmp/public-smoke"
mkdir -p "$TMP_DIR"

if [[ -z "${VIDEO_PATH}" ]]; then
  VIDEO_PATH="$TMP_DIR/sample-smoke.mp4"
  echo "[public-smoke] generating sample video: $VIDEO_PATH"
  ffmpeg -y \
    -f lavfi -i testsrc=size=640x360:rate=24 \
    -f lavfi -i sine=frequency=440:sample_rate=16000 \
    -t 5 \
    -c:v libx264 -pix_fmt yuv420p \
    -c:a aac \
    "$VIDEO_PATH" >/dev/null 2>&1
fi

if [[ ! -f "$VIDEO_PATH" ]]; then
  echo "[public-smoke] video not found: $VIDEO_PATH" >&2
  exit 1
fi

echo "[public-smoke] doctor"
"$BIN" doctor --json > "$TMP_DIR/doctor.json"

echo "[public-smoke] preview"
"$BIN" preview "$VIDEO_PATH" --mode both --json > "$TMP_DIR/preview.json"

echo "[public-smoke] extract"
"$BIN" extract "$VIDEO_PATH" --json > "$TMP_DIR/extract.json"

echo "[public-smoke] extract-batch"
"$BIN" extract-batch "$VIDEO_PATH" --json > "$TMP_DIR/extract-batch.json"

echo "[public-smoke] open-last"
"$BIN" open-last --artifact run > "$TMP_DIR/open-last.txt"

if [[ "$RUN_MCP" == "true" ]]; then
  if [[ -x "./scripts/mcp-smoke.sh" ]]; then
    echo "[public-smoke] mcp-smoke"
    ./scripts/mcp-smoke.sh "$BIN" > "$TMP_DIR/mcp-smoke.txt"
  else
    echo "[public-smoke] skipping mcp-smoke (scripts/mcp-smoke.sh missing)"
  fi
fi

echo ""
echo "[public-smoke] OK"
echo "Artifacts:"
echo "  $TMP_DIR/doctor.json"
echo "  $TMP_DIR/preview.json"
echo "  $TMP_DIR/extract.json"
echo "  $TMP_DIR/extract-batch.json"
echo "  $TMP_DIR/open-last.txt"
if [[ "$RUN_MCP" == "true" ]]; then
  echo "  $TMP_DIR/mcp-smoke.txt"
fi
