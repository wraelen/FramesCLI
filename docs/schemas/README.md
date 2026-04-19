# FramesCLI JSON Schemas

This directory documents the JSON contracts that are intentionally maintained in
the repo.

## What Is Canonical Here

- `cli-response-envelope.json`: the common automation envelope used by most
  `--json` CLI commands and by MCP tool payloads

## What Is Not Checked In Here

Individual MCP tool schemas are not generated into this directory today.
The canonical MCP input schemas live in `cmd/frames/main.go` and are exposed at
runtime through `tools/list`.

If you need the live MCP schema, inspect:

1. `framescli mcp`
2. JSON-RPC `tools/list`

Notes:

- `doctor --json` is a separate doctor report shape, not the common automation
  envelope
- `scripts/generate-schemas.sh` is currently a placeholder and does not emit
  per-tool schema files yet

## Tool Inventory

Current MCP tools:

| Tool | Description |
|------|-------------|
| `doctor` | Check local toolchain readiness |
| `preview` | Estimate extraction cost before running |
| `extract` | Extract frames and optional transcript from video |
| `extract_batch` | Process multiple videos |
| `transcribe_run` | Resume or add transcription to an existing run |
| `open_last` | Resolve a specific artifact path from the latest run |
| `get_latest_artifacts` | Return the compact latest-run artifact map |
| `get_run_artifacts` | Query indexed metadata for latest, named, or recent runs |
| `prefs_get` | Read agent path configuration |
| `prefs_set` | Persist agent path configuration |
