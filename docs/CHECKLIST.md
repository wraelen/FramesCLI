# FramesCLI Ongoing Feature Checklist

Status date: 2026-03-07

This is the single ongoing build checklist for planned work.

Current note:

- The original public-beta checklist below is complete.
- Next-phase planning and parallel work prompts now live in:
  - `docs/NEXT_PHASE_ROADMAP.md`
  - `docs/AGENT_PARALLEL_WORK.md`

## High Priority

- [x] Add queue-level summary output in TUI results (success/fail counts by job).
- [x] Add per-stage elapsed/ETA display in CLI and TUI progress UI.
- [x] Add `--json` machine-readable output consistency rules across `extract`, `extract-batch`, and `preview`.
- [x] Add golden integration tests using fixed sample fixtures for extraction/transcription regressions.
- [x] Add explicit "open last run" command in CLI for fast post-processing workflows.
- [x] Add `framescli mcp` stdio server with tool wrappers for `preview`, `extract`, `extract_batch`, `doctor`, and `open_last`.
- [x] Add MCP preference tools for agent path defaults (`prefs_get`, `prefs_set`).
- [x] Add cancellation/timeout handling in MCP tool execution.

## UX Refinements

- [x] Add optional compact "vim mode" toggle and publish keymap variant in help panel.
- [x] Add richer preview sampling options (sample count + size) in TUI wizard review step.
- [x] Add "save current wizard settings as profile" in TUI and persist to `preset_profiles`.
- [x] Add queue editing actions in review step (remove/reorder pending jobs).
- [x] Add one-command clipboard export for key paths (`run dir`, `transcript`, `sheet`).

## Reliability And Ops

- [x] Add benchmark history docs/examples polish (`prune`, CSV export workflows).
- [x] Add structured diagnostics bundle for failed runs (logs + metadata + environment snapshot).
- [x] Add cross-platform tests for clipboard integration fallback behavior.
- [x] Add Linux/macOS/WSL performance baseline profile recommendations with validation docs.

## Nice To Have

- [x] Optional plugin-style post-process hooks (agent adapters, upload targets).
- [x] Optional local-only telemetry for performance regressions (opt-in).

## Public Release Readiness

### Must-Have Before Public Repo Push

- [x] `go test ./...` passes in a clean local run.
- [x] `go test -tags integration ./internal/media` passes (ffmpeg-backed fixtures).
- [x] Core CLI command failures return non-zero exit codes (including JSON mode).
- [x] README command list and examples match implemented commands/options.
- [x] README includes dependency install and quickstart verification steps.
- [x] `doctor` output provides actionable recovery guidance for missing required tools.
- [x] MCP docs include safe onboarding flow (`doctor` -> `prefs_set` -> `preview` -> `extract`).

### Nice-To-Have After Launch

- [x] Add copy-paste package-manager snippets for Whisper installation by platform.
- [x] Add CI matrix smoke tests for macOS/Linux help/doctor/preview command ergonomics.
- [x] Publish a short “agent recipes” page with end-to-end MCP workflows.
- [x] Add packaged-release verification for checksums, archive contents, installer asset resolution, and current-platform runtime smoke.
