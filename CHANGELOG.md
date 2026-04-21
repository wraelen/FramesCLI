# Changelog

## [0.3.0] - 2026-04-20

Pre-1.0 cleanup pass preparing for the first public announcement. Focus: smaller surface, fewer personal-workflow artifacts, better agent-first UX.

### Removed
- **Interactive setup wizard.** Deleted the 1,054-line Bubbletea wizard and its dependencies (`charmbracelet/bubbletea`, `bubbles`, `lipgloss`). `framescli setup` with no flags now prints a cheat-sheet of current config plus one-liner examples for changing values. Use `--non-interactive --flag=value` to persist changes, or `--yes` to write current defaults without changing anything.
- **OBS-specific config.** Removed the `obs_video_dir` config key, the `OBS_VIDEO_DIR` environment variable, the `DefaultOBSVideoDir()` helper, and the hardcoded `C:\Users\wraelen\Videos\OBS` fallback path. `extract recent` now requires `video_input_dirs` to be set and errors with a clear hint if it isn't — no more silent fallback to a path that only ever worked on one machine.
- **Legacy TUI config fields.** `tui_theme_preset`, `tui_simple_mode`, `tui_welcome_seen`, `tui_vim_mode` are gone from the `Config` struct. Old configs with these keys still load cleanly (Go silently ignores unknown fields on unmarshal); on next save the stale keys are dropped.
- **Dead `drop_modal.go`.** 211 lines of unreferenced modal code from a removed dashboard.

### Changed
- **Renamed `recent_video_dirs` → `video_input_dirs`** and **`recent_extensions` → `video_input_extensions`**. The `--recent-video-dirs` / `--recent-extensions` CLI flags are now `--video-input-dirs` / `--video-input-extensions`. The old names were misleading ("recent" implied a list of recently used videos — the field is actually "where to look for videos"). Old config keys are silently dropped on load; set the new keys via `framescli setup --non-interactive --video-input-dirs <path>`.
- **`framescli doctor` prints a first-run hint** when no config file exists, pointing at `framescli setup --non-interactive --video-input-dirs <path>` for users who want `extract recent` to work. Defaults still require zero config for everything else.
- **MCP `prefs_get` / `prefs_set` are now concurrency-safe.** Added a mutex around `appCfg` access so concurrent tool calls don't return stale snapshots.

### Added
- **`internal/contracts/` source-of-truth registry.** MCP tool definitions and input schemas are now derived from Go types via reflection. `cmd/contractgen` regenerates `docs/schemas/{cli-response-envelope.json,mcp-tools.json,README.md}` from the registry. `scripts/generate-schemas.sh` is the entrypoint; `TestGeneratedAgentContractArtifactsAreCurrent` fails CI if generated docs drift.
- **`scripts/qa-pass.sh` canonical QA entrypoint.** Runs preflight + MCP contract tests + public smoke. CI (`ci.yml`), integration (`integration.yml`), and release (`release.yml`) workflows now call this instead of ad-hoc step lists.
- **README "Configuration" section** documenting the three flags most users care about, with `prefs_set` called out explicitly as the agent-facing surface.

### Fixed
- **Correct `--fps auto` help text.** Both `extract --fps` and `preview --fps` flag help now describe the real target-frame formula (~480 frames, clamped 1–8 fps) instead of the obsolete "~60-frame mode" claim.
- **`framescli config` output alignment.** Labels in the config printout now have a space between the colon and the value (previously rendered as `Whisper Language:-`).
- **MCP stdio smoke test uses the real handshake.** `scripts/mcp-smoke.sh` now sends `initialize` + `notifications/initialized` before each tool call, matching the MCP spec.

## [0.2.6] - 2026-04-19

Patch release for the shipped v0.2.5 regressions plus documentation cleanup.

### Fixed
- **MCP auto-fps now behaves like the CLI path.** MCP `preview`, `extract`, and `extract_batch` now accept `fps` as a number, `0`, or `"auto"`. Explicit `0` is treated as auto mode instead of silently falling back to config default fps, and MCP no longer rejects `"auto"` with a Go JSON unmarshal error.
- **Auto-selected fps is visible in MCP extraction results.** `extract` results now include `fps_mode: "auto"` when auto sampling was requested, matching the preview path and the run metadata.
- **`framescli sheet <run-dir>` now does the obvious thing.** The command auto-detects a run directory's `images/` subdirectory and writes the contact sheet to `images/sheets/contact-sheet.png` by default.
- **`scripts/mcp-smoke.sh` no longer requires ripgrep.** The smoke check now uses `grep -Eq`, so it runs on systems that do not have `rg`.

### Documentation
- **Canonicalized Homebrew docs.** `docs/HOMEBREW_SETUP.md` now reflects the real GoReleaser-plus-tap workflow, and `homebrew/README.md` is reduced to a pointer so there is one source of truth.
- **Collapsed agent doc duplication.** `docs/AGENT_INTEGRATION.md` is now the setup/contract document, while `docs/AGENT_RECIPES.md` is kept as the copy-paste cookbook.
- **Corrected schema drift.** The documented automation envelope now matches the real fields and values (`ended_at`, `duration_ms`, `status: error`), and the schema docs now state that MCP tool schemas are sourced from `tools/list` / `cmd/frames/main.go` rather than checked-in per-tool files.

## [0.2.4] - 2026-04-18

Follow-up polish driven by an external second-pass debug pack against v0.2.3.

### Fixed
- **`--transcribe-timeout` now reports the correct value when it fires.** The chunked path wasn't propagating `TimeoutSec` to the inner whisper call, so stderr printed `transcription timed out after 0s` even when the user passed `--transcribe-timeout 5`. The timeout itself fired correctly — only the message was wrong.
- **`--fps auto` is now visible in output and metadata.** Previously the auto-resolved fps silently appeared as a concrete number with no indication that auto-mode was chosen. Now run.json records `fps_mode: "auto"`, preview JSON includes `fps_mode`, and the extract header prints `fps: 1.00 (auto, computed from duration 1068.5s)`.
- **Short clips skip the chunked transcription pipeline.** Clips that fit in one chunk (with 10% overhang tolerance) now go through a single-shot path — no manifest, no ffmpeg chunk-split, no merge step. A 60-second clip at the default 600s chunk duration saves several seconds of overhead and produces identical output. Existing manifests still always resume via the chunked path, so interrupted long runs are unaffected. The transcribe header now shows `single-shot (clip 60s fits in one 600s chunk)` instead of a misleading `chunks=1`.

### Added
- **Hidden `import` deprecation command.** `framescli import <url>` used to return "unknown command" after the URL flow moved to `extract --url`. Now it prints a clear migration message pointing at the new command form. Kept hidden from `--help` so it doesn't re-advertise the removed surface.
- **Transcription heartbeat makes progress visible during long chunks.** A per-chunk heartbeat goroutine emits stage + elapsed-time updates every second, with interpolated pct capped at 90% of the chunk's span so the bar never claims completion before whisper actually returns. Output now looks like `chunk 1/2 · 00:47 elapsed` instead of a frozen `chunk 1/2 (0%)`. Single-shot runs get the same treatment. Previous behavior emitted pct only at chunk boundaries, which made minutes of whisper work appear stuck.
- **Renderer switches to newline cadence when stdout isn't a TTY.** When stdout is a pipe, log file, or CI buffer, the renderer emits one progress line every 5s instead of carriage-return animation at 200ms. Fixes the external tester report of "hundreds of repeated `chunk 1/2 (0%)` lines" in non-terminal environments.

### Changed
- **Default output root moved to `~/framescli/runs/`.** Previously ran in `./frames/` relative to the current working directory, which dumped potentially-GB-sized output wherever the user happened to invoke the command. Existing user configs with a custom `frames_root` are untouched; only first-run defaults change. `framescli doctor` now surfaces `Output root:` and total usage across runs.
- **`framescli clean` gained selective pruning flags.** New `--older-than <duration>` (supports `30d`, `12h`, `7d12h`), `--keep-last <N>`, and `--dry-run` — prune without nuking the entire root. The no-flag form preserves the historical "wipe everything" behavior for backward compat. Runs are identified as direct subdirs containing `run.json`, so stray non-run content (diagnostics, index.json) is left alone. Extract's end-of-run summary prints a soft storage hint when the root exceeds 5 GB.
- **`--fps auto` produces usable density for every duration, not just <60s clips.** The old formula `round(60/duration)` targeted ~60 total frames, which clamped to 1 fps for anything longer than 45 seconds — effectively making `auto` a constant for every realistic video. The new formula targets ~480 frames with clamps at `[1, 8]` fps: a 10s clip gets 8 fps (80 frames), a 2-min clip gets 4 fps (480 frames), a 5-min video gets 1.5 fps (450 frames), a 30-min video keeps 1 fps (1800 frames). Short/medium clips catch per-second action that used to slip between samples; long recordings keep the sensible 1 fps baseline.
- **GPU acceleration is now reported per-subsystem.** Previously a single `doctorHasGPU()` bool drove both extraction *and* transcription speed claims — so a machine with an NVIDIA GPU + only `openai-whisper` installed got `"~10x realtime"` headlines while whisper actually ran on CPU (emitting `FP16 is not supported on CPU; using FP32 instead`). Now:
  - Doctor shows `Accel: GPU | CPU` with an explicit reason line under Transcription.
  - `openai-whisper` always reports CPU estimates (its typical pip install is CPU-only even on GPU systems); `faster-whisper` reports GPU when hardware GPU is present. The Hardware section continues to surface the detected GPU for extraction.
  - The extract transcribe header now prints `accel: <reason>` so the expectation is honest at run time.
  - Preview JSON, doctor JSON, and extract messaging all consume the same `transcribeAccel` result. New fields on doctor JSON: `transcribe_uses_gpu`, `transcribe_accel_reason`.
  - Rule-based for now; a Python-level CUDA probe (`ctranslate2.get_cuda_device_count`, `torch.cuda.is_available`) can replace the rule in a follow-up if the heuristic misclassifies real setups.

### Credits
Thanks to the same external live-test agent whose second-pass debug pack (2026-04-17) surfaced these follow-ups.

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
- **`--version` flag** with goreleaser-populated `main.version`/`main.commit`/`main.date` via ldflags. The Homebrew formula's `system "framescli", "--version"` test now actually works.
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
