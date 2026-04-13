# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Test Commands

### Build
```bash
make build              # Build to bin/framescli
go build -o bin/framescli ./cmd/frames
```

### Testing
```bash
make test               # Run all unit tests
make test-integration   # Run integration tests with real ffmpeg/whisper
go test ./cmd/frames    # Test main CLI package
go test ./internal/media
go test ./internal/config
go test ./internal/tui
```

### MCP-specific testing
```bash
go test ./cmd/frames -run 'TestMCPServer|TestMCPHelperProcess'
```

### Verification and smoke testing
```bash
make preflight          # Run pre-commit checks
make smoke-public       # Full smoke test with generated sample video
./scripts/public-smoke.sh --video /path/to/video.mp4
```

### Release
```bash
make release-snapshot   # Build release snapshot with goreleaser
make release-verify     # Verify release artifacts
```

## Architecture Overview

### Package Structure

- **`cmd/frames/main.go`**: CLI entry point with all command implementations. Contains the entire cobra command tree, JSON output envelopes, MCP stdio server, progress tracking, and CLI-level orchestration. This is a large file (~5000 lines) that implements all user-facing commands.

- **`internal/media/`**: Core extraction, transcription, and artifact generation logic
  - `media.go`: Video frame extraction (`ExtractMedia`), audio extraction, transcription (`TranscribeAudio`), contact sheet generation, video probing with ffprobe
  - `transcribe_chunk.go`: Chunked transcription for long recordings with resumable manifest-based progress
  - `index.go`: Run artifact indexing system that maintains `frames/index.json` with metadata for all extraction runs
  - `benchmark.go`: Performance benchmarking and history tracking

- **`internal/config/`**: Configuration management
  - `config.go`: Config loading/saving, environment variable integration, preset profiles, path normalization
  - Default config: `~/.config/framescli/config.json`
  - Legacy path support: `~/.config/frames-cli/config.json`

- **`internal/tui/`**: Terminal UI implementation using Bubbletea
  - `dashboard.go`: Full-featured TUI with import wizard, queue management, profile saving, vim-mode toggle, theme switching

### Key Data Flow

1. **CLI command** (cobra in `main.go`) → validates args, resolves "recent" video paths
2. **Media operations** (`internal/media`) → calls ffmpeg/ffprobe/whisper as subprocesses
3. **Artifact output** → writes to `frames/<RunName>/` with structured JSON metadata
4. **Index update** → refreshes `frames/index.json` after successful runs

### Extraction Pipeline

1. Resolve video path (handle "recent" keyword via config dirs)
2. Probe video with `ProbeVideoInfo()` (ffprobe subprocess)
3. Apply workflow preset defaults (laptop-safe/balanced/high-fidelity)
4. Check guardrails (frame count, disk estimates, duration, transcript cost)
5. Build ffmpeg args with hwaccel, filters, performance preset
6. Execute ffmpeg with progress tracking
7. Generate contact sheet, frame metadata JSON, optional CSV/zip
8. Extract audio if `--voice` requested
9. Transcribe with chunking if needed (`TranscribeAudioWithDetails` or `TranscribeRunWithChunking`)
10. Write `run.json`, `frames.json`, update artifact index

### MCP Integration

- MCP stdio server lives in `cmd/frames/main.go` as `mcpCmd`
- Tools: `doctor`, `preview`, `extract`, `extract_batch`, `transcribe_run`, `open_last`, `get_latest_artifacts`, `get_run_artifacts`, `prefs_get`, `prefs_set`
- Path safety: restricted to configured `AgentInputDirs` and `AgentOutputRoot` from config
- Heartbeat: emits `notifications/message` every 10 seconds during long-running operations
- Error metadata: stable top-level codes with additive recovery metadata in `error.data.*`

### Workflow Presets

Presets apply coordinated defaults for FPS, format, ffmpeg performance tuning, and transcript chunking:

- **laptop-safe**: 1fps, jpg, media preset "safe", 300s chunks
- **balanced**: 4fps, png, media preset "balanced", 600s chunks
- **high-fidelity**: 8fps, png, media preset "fast", 900s chunks

Explicit flags (`--fps`, `--format`, `--chunk-duration`) override preset values.

### Hardware Acceleration

HWAccel modes: `none`, `auto`, `cuda`, `vaapi`, `qsv`

Auto-fallback: if hwaccel fails, extraction retries with CPU-only mode and logs warning.

### Transcription Backends

- **auto**: prefers `faster-whisper` if available, falls back to `whisper`
- **whisper**: OpenAI Whisper (slower, CPU-heavy without GPU)
- **faster-whisper**: faster implementation, better for GPU workflows

Chunked transcription: for recordings >10min, use `--chunk-duration <seconds>` to split audio into resumable chunks. Manifest tracks progress at `voice/transcription-manifest.json`.

### Artifact Index

Maintained at `<frames_root>/index.json`. Auto-refreshed after `extract` and `transcribe-run`. Rebuild manually with `framescli index`.

Index schema includes: run directory, timestamps, video path, FPS, format, preset, duration, artifact paths (run.json, frames.json, transcript outputs, sheet, manifest, log, CSV, zip).

## Code Patterns

### Error Handling in CLI Commands

Commands use `failf()` and `failln()` to mark command failure and print to stderr. Exit code is set via `commandFailed` flag checked in main.

### JSON Output Envelope

All `--json` commands return:
```go
{
  "schema_version": "framescli.v1",
  "command": "extract",
  "status": "success|partial|failed",
  "started_at": "...",
  "completed_at": "...",
  "data": { /* command-specific payload */ },
  "error": { /* optional, includes code, class, recovery, retryable */ }
}
```

### Context and Cancellation

Long-running operations accept `context.Context` for cancellation support. TUI and CLI hooks use signal handling to cancel in-progress ffmpeg/whisper subprocesses.

### Progress Tracking

Progress callbacks: `func(percent float64)` passed to `ExtractMedia` and similar functions. CLI renders progress bars, TUI updates stage indicators.

### Path Normalization

WSL paths: `NormalizeVideoPath()` converts Windows-style paths (`C:\...`) to WSL mounts (`/mnt/c/...`).

### Testing Strategy

- Unit tests: mock-free logic tests in `*_test.go` files
- Integration tests: `integration_*_test.go` files with `//go:build integration` tag, require real ffmpeg/ffprobe/whisper
- MCP tests: `cmd/frames/mcp_integration_test.go` uses fake binaries and helper process pattern for deterministic stdio harness testing

## Common Development Patterns

### Adding a New CLI Command

1. Add cobra command in `cmd/frames/main.go`
2. Wire up flags and validation
3. Call `internal/media` functions or implement logic inline
4. Support `--json` output with envelope format
5. Add corresponding test in `cmd/frames/main_test.go`

### Adding a New MCP Tool

1. Add tool definition to `mcpCmd` in `cmd/frames/main.go`
2. Implement tool handler with path safety checks
3. Return structured JSON with stable error codes
4. Add test case to `mcp_integration_test.go`

### Modifying Extraction Logic

1. Update `ExtractMedia()` or related functions in `internal/media/media.go`
2. Update `ExtractMediaOptions` struct if adding new parameters
3. Update `RunMetadata` struct if persisting new metadata
4. Run integration tests: `make test-integration`
5. Test with smoke script: `./scripts/public-smoke.sh`

### Adding a New Workflow Preset

1. Update preset defaults in `internal/config/config.go` (`Default()` function)
2. Update preset normalization in `cmd/frames/main.go` (preset resolution functions)
3. Update `PresetProfile` struct if adding new preset-specific fields
4. Document in README.md

## Configuration Notes

Config precedence:
1. Explicit CLI flags (highest)
2. Environment variables (`FRAMES_CONFIG`, `WHISPER_BIN`, `WHISPER_MODEL`, etc.)
3. Config file values (`~/.config/framescli/config.json`)
4. Preset defaults (applied when user config uses stock defaults)
5. Hard-coded defaults (lowest)

Performance mode applies preset sampling/format implicitly only when `default_fps` and `default_format` are still at stock defaults. Custom config values always win.

## Dependencies

- **ffmpeg/ffprobe**: required for all video operations
- **whisper or faster-whisper**: optional, only for `--voice` transcription
- **Bubbletea/Bubbles/Lipgloss**: TUI framework
- **Cobra**: CLI framework

## Troubleshooting

- If tests fail with "ffmpeg not found", install ffmpeg: `make deps`
- Integration tests require real media tools: `make deps` or `./scripts/install-deps.sh --install`
- WSL path issues: ensure `NormalizeVideoPath()` is called before file operations
- MCP stdio issues: test with `go test ./cmd/frames -run TestMCPServer -v`
