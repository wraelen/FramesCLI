# Changelog

## [0.2.3] - 2026-04-17

Polish release driven by an external live-test debug pack. Closes the gaps that made v0.2.2 feel "more like a power-user tool than a polished public-facing product."

### Fixed
- **URL passed as a positional arg now auto-routes to the URL flow.** Previously `framescli extract https://youtu.be/...` failed with "invalid video input"; now it transparently downloads via yt-dlp.
- **Reruns into existing output dirs skip already-completed work.** Frame and audio extraction is skipped when artifacts match the planned output; transcription is skipped when `transcript.txt` + `transcript.json` already exist. Use `--force` to override. Cancel-then-resume now finishes the second run in milliseconds instead of redoing everything.
- **Transcription progress is no longer an opaque spinner.** A new progress renderer shows live chunk progress (`chunk 3/12 (25%)`) plus a one-time header announcing backend, model, and an honest CPU vs GPU expectation. Single-writer design eliminates the spinner-vs-progress collision.
- **Friendly missing-file errors.** `framescli extract /no/such/file.mp4` now says `file not found: ...` with a recovery hint instead of `video probe failed: exit status 1`. New `file_not_found` error class surfaces in JSON and MCP envelopes.
- **`-vsync` deprecation cleaned up.** Replaced with `-fps_mode vfr` to match ffmpeg 5.0+. No more deprecation warnings in stderr.
- **Doctor surfaces the missing-cuda-in-ffmpeg trap.** When a GPU is detected but the local ffmpeg lacks support for the recommended hwaccel (common with static ffmpeg builds), doctor warns and the auto-config no longer sets `hwaccel=cuda` — preventing the constant "cuda failed; falling back to CPU" notes on every run.
- **CPU users get a loud faster-whisper hint.** Doctor now prints a high-visibility recommendation block on CPU-only systems with whisper but without faster-whisper (3-5× speedup unlocked).
- **Doctor recommendations point to real commands.** Replaced fake `framescli prefs set …` references with the actual `framescli setup --non-interactive --hwaccel …` form.

### Added
- **`--version` flag and version subcommand** with goreleaser-populated `main.version`/`main.commit`/`main.date` via ldflags. The Homebrew formula's `system "framescli", "--version"` test now actually works.
- **`--force` flag on `extract`** to opt out of resume behavior and re-extract from scratch.
- **`ProgressFn` callback on `TranscribeOptions`** for embedding transcription in long-running pipelines that want progress events.
- **Major test additions:** `TestEvaluateResumeStateForceBypasses`, `TestEvaluateResumeStateMatchingFramesSkips`, `TestEvaluateResumeStateMissingAudioRunsFFmpeg`, `TestEvaluateResumeStateFrameCountMismatchRunsFFmpeg`, `TestLooksLikeURL`, `TestExistingTranscriptArtifacts`, `TestFFmpegSupportsHWAccel`, plus `file_not_found` and `path_is_directory` error classification cases.

### Documentation
- **README rewritten** from 944 → 353 lines. Install-first structure, removed "AI slop" marketing copy, added hero-demo placeholder, concrete real-world examples instead of abstract feature lists.

### Credits
Thanks to the external live-test agent who produced the comprehensive debug pack against v0.2.2 — this release is built directly from those findings.

## [0.2.2] - 2026-04-15

- Hardened published-release installs by verifying downloaded archives against GoReleaser `checksums.txt` before extraction, and added `scripts/release-verify.sh` plus a `release-verify` workflow for packaged-artifact validation.
- Added richer `preview` workload estimation with approximate frame counts, disk-size ranges, common sampling/format profiles, transcript runtime class hints, CPU/long-form warnings, and optional benchmark-derived machine-speed notes in both text and JSON output.
- Added CPU-aware transcription behavior in `internal/media/media.go`: GPU detection, CPU slowdown warnings for `base` and larger models, and automatic fallback from the default `base` model to `tiny` on CPU-only systems.
- Added absolute path normalization for subprocess-bound media paths so ffmpeg, ffprobe, whisper, and resume flows operate on stable filesystem paths.
- Improved default run naming by switching timestamp-based output directories to include a 6-character crypto-random hex suffix before `ensureUniquePath()` fallback.
- Added transcription timeout support with `TimeoutSec`, `ErrTranscribeTimeout`, and the CLI flag `--transcribe-timeout`, allowing extraction pipelines to skip stalled transcription without losing extracted media.
- Added `framescli transcribe-run <runDir>` for resume or transcribe-only workflows and persisted `audio_path` in `run.json` to support reliable resume behavior.
- Extended the MCP server with `transcribe_run`, periodic `notifications/message` heartbeats for long-running `extract`, `extract_batch`, and `transcribe_run` calls, and improved agent-facing recipes in `docs/AGENT_RECIPES.md`.
- Added additive automation and MCP error metadata (`class`, `recovery`, `retryable`) while keeping the existing stable top-level error code contract.
- Consolidated artifact retrieval docs and selectors so `artifacts`, `open-last`, `copy-last`, and MCP recipes consistently cover transcript sidecars, manifests, CSV metadata, and frame zip outputs.
- Expanded `doctor` output with GPU detection, resolved Whisper model reporting, and an estimated transcription speed hint for CPU-only versus GPU-enabled systems.
- Added automatic FPS mode for `extract`: `--fps 0`, `--fps auto`, or positional `auto` now target roughly 60 frames based on video duration.
- Added a first-run Whisper cache warning before model download, including approximate model sizes for common models.
