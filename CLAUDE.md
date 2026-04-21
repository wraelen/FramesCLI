# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## QA entrypoint

`scripts/qa-pass.sh` — canonical QA entrypoint. Fast lane: `./scripts/qa-pass.sh --no-public-smoke`. Full lane: `./scripts/qa-pass.sh`. CI and release both call it.

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

GPU is reported per-subsystem, not as a single global bool:
- **Hardware GPU** — `detectGPU()` / `doctorHasGPU()` — describes what silicon is on the machine. Drives extraction hwaccel recommendations.
- **FFmpeg GPU** — `ffmpegSupportsHWAccel()` — checks whether the installed ffmpeg binary actually exposes the recommended hwaccel. Drives hwaccel messaging in doctor.
- **Transcription GPU** — `probeTranscribeAccel(cfg, backendOverride, hwGPUPresent)` returns `{UsesGPU, Reason}`. Rule-based: openai-whisper always reports CPU (typical pip installs don't ship CUDA torch); faster-whisper reports GPU when hardware GPU is present. Drives `transcriptRealtimeRange`, `buildTranscriptPlan`, extract transcribe header, and the doctor Accel line.

Never pass `doctorHasGPU()` directly into transcription speed estimates — use `probeTranscribeAccel(...).UsesGPU` so the claim matches the backend that will actually run.

### Transcription Backends

- **auto**: prefers `faster-whisper` if available, falls back to `whisper`
- **whisper**: OpenAI Whisper (slower, CPU-heavy without GPU)
- **faster-whisper**: faster implementation, better for GPU workflows

Chunked transcription: use `--chunk-duration <seconds>` to split long audio into resumable chunks. Manifest tracks progress at `voice/transcription-manifest.json`. Clips whose audio fits in one chunk (with 10% overhang) auto-skip the chunked pipeline and use a faster single-shot path — no manifest, no split/merge overhead. Existing manifests always resume via the chunked path regardless of duration, so interrupted long runs aren't downgraded.

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
  "status": "success|partial|error",
  "started_at": "...",
  "ended_at": "...",
  "duration_ms": 123,
  "data": { /* command-specific payload */ },
  "error": { /* optional, includes code, class, recovery, retryable */ }
}
```

### Context and Cancellation

Long-running operations accept `context.Context` for cancellation support. CLI hooks use signal handling to cancel in-progress ffmpeg/whisper subprocesses.

### Progress Tracking

Progress callbacks: `func(percent float64)` passed to `ExtractMedia`; `func(stage string, pct float64)` on `TranscribeOptions.ProgressFn`. CLI renders progress bars on stdout.

Transcription emits a per-second heartbeat (`startTranscribeHeartbeat` in `internal/media/transcribe_chunk.go`) during otherwise-quiet whisper runs. The heartbeat interpolates pct inside the current chunk's span (capped at 90% so the bar doesn't claim completion before whisper returns) and augments the stage string with elapsed time, e.g. `chunk 1/2 · 00:47 elapsed`. Passing `pct < 0` to the progress callback means "leave pct unchanged" — use it for stage-only updates.

The CLI renderer (`transcribeProgressRenderer.run` in `cmd/frames/main.go`) checks `isStdoutTTY()` and switches between carriage-return animation (TTY, 200ms tick) and newline-per-change cadence (non-TTY, 5s tick) so piped output stays readable.

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
- **Cobra**: CLI framework

## gstack Integration

**IMPORTANT: For all web browsing tasks, use the `/browse` skill from gstack. NEVER use `mcp__claude-in-chrome__*` tools.**

gstack provides specialized workflow skills for development, design, and quality assurance. Available skills:

### Planning & Review Skills
- `/office-hours` - Executive office hours workflow
- `/plan-ceo-review` - Plan CEO review sessions
- `/plan-eng-review` - Plan engineering reviews
- `/plan-design-review` - Plan design reviews
- `/plan-devex-review` - Plan developer experience reviews

### Design & Consultation
- `/design-consultation` - Design consultation workflow
- `/design-shotgun` - Quick design iterations
- `/design-html` - HTML/CSS design work
- `/design-review` - Comprehensive design reviews

### Development & Shipping
- `/review` - Code review workflow
- `/ship` - Ship features to production
- `/land-and-deploy` - Land code and deploy
- `/canary` - Canary deployment workflow
- `/autoplan` - Automated planning

### Quality & Testing
- `/qa` - Quality assurance workflow
- `/qa-only` - QA-only mode
- `/benchmark` - Performance benchmarking

### Browser & Investigation
- `/browse` - Web browsing with playwright (use this instead of MCP chrome tools)
- `/connect-chrome` - Connect to Chrome instance
- `/investigate` - Investigation workflow
- `/setup-browser-cookies` - Configure browser authentication

### Documentation & Deployment
- `/document-release` - Document release notes
- `/setup-deploy` - Setup deployment configuration
- `/retro` - Retrospective workflow
- `/codex` - Documentation generation

### Safety & Controls
- `/careful` - Enable careful mode
- `/freeze` - Freeze current state
- `/guard` - Enable guard mode
- `/unfreeze` - Unfreeze state
- `/cso` - Chief Security Officer workflow

### Utilities
- `/learn` - Learning workflow
- `/gstack-upgrade` - Upgrade gstack skills

## Troubleshooting

- If tests fail with "ffmpeg not found", install ffmpeg: `make deps`
- Integration tests require real media tools: `make deps` or `./scripts/install-deps.sh --install`
- WSL path issues: ensure `NormalizeVideoPath()` is called before file operations
- MCP stdio issues: test with `go test ./cmd/frames -run TestMCPServer -v`

## Skill routing

When the user's request matches an available skill, ALWAYS invoke it using the Skill
tool as your FIRST action. Do NOT answer directly, do NOT use other tools first.
The skill has specialized workflows that produce better results than ad-hoc answers.

Key routing rules:
- Product ideas, "is this worth building", brainstorming → invoke office-hours
- Bugs, errors, "why is this broken", 500 errors → invoke investigate
- Ship, deploy, push, create PR → invoke ship
- QA, test the site, find bugs → invoke qa
- Code review, check my diff → invoke review
- Update docs after shipping → invoke document-release
- Weekly retro → invoke retro
- Design system, brand → invoke design-consultation
- Visual audit, design polish → invoke design-review
- Architecture review → invoke plan-eng-review
- Save progress, checkpoint, resume → invoke checkpoint
- Code quality, health check → invoke health
