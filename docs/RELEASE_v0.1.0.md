# FramesCLI v0.1.0 (Public Beta)

FramesCLI is now available as a standalone open-source CLI + TUI for turning screen recordings into agent-ready debugging artifacts.

## Highlights

- Extract frames from recordings with interval, time-range, and frame-range controls
- Generate contact sheets for rapid visual scanning
- Extract audio and run local Whisper transcription (`txt`, `json`, `srt`, `vtt`)
- Process batches of videos via multiple paths/globs
- Use machine-readable JSON output for automation (`extract`, `extract-batch`, `preview`)
- Launch interactive dashboard TUI (`framescli tui`) with queueing and preview
- Run MCP server (`framescli mcp`) for agent/IDE integrations
- Produce diagnostics bundles for failed extraction runs

## Stability and Automation Improvements

- Command failures now consistently return non-zero exit codes
- JSON-mode errors/partials now align with exit behavior for automation reliability
- `doctor` includes clearer recovery guidance for missing dependencies
- README and command documentation aligned to implemented command surface

## Command Surface

- `extract`, `extract-batch`, `preview`
- `open-last`, `copy-last`
- `import`, `tui`, `sheet`, `transcribe`, `clean`
- `doctor`, `setup`, `config`, `index`, `benchmark`, `telemetry`, `mcp`, `completion`

## MCP Tools

- `preview`
- `extract`
- `extract_batch`
- `doctor`
- `open_last`
- `get_latest_artifacts`
- `prefs_get`
- `prefs_set`

## Validation Snapshot

- `go test ./...` passed
- `go test -tags integration ./internal/media` passed
- `make preflight` passed

## Known Limitations

- CI currently validates Linux/macOS paths; Windows/WSL runtime coverage remains manual
- Live external MCP client handshake coverage is documentation-driven and should be expanded with integration tests

## Upgrade / Install

Build from source:

```bash
go mod tidy
go build -o bin/framescli ./cmd/frames
./bin/framescli --help
```

Install locally:

```bash
./scripts/install.sh
framescli --help
```

## Feedback

Please open issues for:

- extraction regressions or codec edge cases
- TUI UX rough edges
- MCP integration bugs
- platform-specific install/runtime issues
