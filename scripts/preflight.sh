#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

echo "[preflight] tidy"
go mod tidy

echo "[preflight] fmt-check"
test -z "$(gofmt -l ./cmd ./internal)"

echo "[preflight] unit tests"
go test ./...

echo "[preflight] build"
go build ./...

echo "[preflight] cli smoke"
go run ./cmd/frames --help >/dev/null
go run ./cmd/frames doctor --help >/dev/null
go run ./cmd/frames benchmark --help >/dev/null
go run ./cmd/frames setup --help >/dev/null
go run ./cmd/frames mcp --help >/dev/null
go run ./cmd/frames transcribe-run --help >/dev/null

echo "[preflight] done"
