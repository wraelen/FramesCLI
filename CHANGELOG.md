# Changelog

## [Unreleased]

- Added CPU-aware transcription behavior in `internal/media/media.go`: GPU detection, CPU slowdown warnings for `base` and larger models, and automatic fallback from the default `base` model to `tiny` on CPU-only systems.
- Added absolute path normalization for subprocess-bound media paths so ffmpeg, ffprobe, whisper, and resume flows operate on stable filesystem paths.
- Improved default run naming by switching timestamp-based output directories to include a 6-character crypto-random hex suffix before `ensureUniquePath()` fallback.
- Added transcription timeout support with `TimeoutSec`, `ErrTranscribeTimeout`, and the CLI flag `--transcribe-timeout`, allowing extraction pipelines to skip stalled transcription without losing extracted media.
- Added `framescli transcribe-run <runDir>` for resume or transcribe-only workflows and persisted `audio_path` in `run.json` to support reliable resume behavior.
- Extended the MCP server with `transcribe_run`, periodic `notifications/message` heartbeats for long-running `extract`, `extract_batch`, and `transcribe_run` calls, and improved agent-facing recipes in `docs/AGENT_RECIPES.md`.
- Expanded `doctor` output with GPU detection, resolved Whisper model reporting, and an estimated transcription speed hint for CPU-only versus GPU-enabled systems.
- Added automatic FPS mode for `extract`: `--fps 0`, `--fps auto`, or positional `auto` now target roughly 60 frames based on video duration.
- Added a first-run Whisper cache warning before model download, including approximate model sizes for common models.
