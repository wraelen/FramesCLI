#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

RUN_PUBLIC_SMOKE="true"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-public-smoke)
      RUN_PUBLIC_SMOKE="false"
      shift
      ;;
    -h|--help)
      cat <<'EOF'
Usage: scripts/qa-pass.sh [--no-public-smoke]

Runs the standard FramesCLI QA lanes:
  1) scripts/preflight.sh
  2) go test ./cmd/frames -run 'TestMCPServer|TestMCPHelperProcess|TestGeneratedAgentContractArtifactsAreCurrent|TestCLIResponseEnvelopeSchemaLeavesCommandUnconstrained|TestActiveDocsDoNotReferenceRemovedTUISurface'
  3) scripts/public-smoke.sh (unless --no-public-smoke)
EOF
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

echo "[qa-pass] preflight"
./scripts/preflight.sh

echo "[qa-pass] mcp contract"
go test ./cmd/frames -run 'TestMCPServer|TestMCPHelperProcess|TestGeneratedAgentContractArtifactsAreCurrent|TestCLIResponseEnvelopeSchemaLeavesCommandUnconstrained|TestActiveDocsDoNotReferenceRemovedTUISurface'

if [[ "$RUN_PUBLIC_SMOKE" == "true" ]]; then
  echo "[qa-pass] public smoke"
  ./scripts/public-smoke.sh
fi

echo "[qa-pass] done"
