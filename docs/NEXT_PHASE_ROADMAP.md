# FramesCLI Next-Phase Roadmap

Status date: 2026-04-10

This document defines the next build phase after the public-beta checklist was completed.

The goal is not to add random surface area. The goal is to make FramesCLI trustworthy at scale, predictable on normal developer machines, and clearly excellent for agent-driven workflows.

## Product Direction

FramesCLI should be positioned as a local-first developer artifact pipeline for recordings:

- safe by default on ordinary laptops and desktops
- predictable for long recordings through preview, chunking, resume, and diagnostics
- strong for agent use through stable machine-readable contracts and MCP tooling
- honest about hardware limits instead of pretending every machine can process every recording at full fidelity

## Next Milestones

## `v0.1.1` Stabilization And Trust

Primary outcome: reduce failure risk and improve confidence for first-time and repeat users.

Scope:

- add MCP integration tests for handshake, cancellation, timeout, and long-running progress notifications
- expand fixture coverage for edge-case media: VFR, odd codecs, high resolution, no audio, corrupted headers, giant duration
- improve error taxonomy and recovery messaging across CLI, TUI, and MCP JSON responses
- strengthen cross-platform smoke validation for Linux, macOS, and WSL workflows
- improve diagnostics bundles so bug reports are more actionable and easier to reproduce

Exit criteria:

- agent-facing commands are covered by stable integration tests
- major failure classes produce actionable recovery guidance
- at least one long-running extraction and one resume flow are covered by automated tests

## `v0.2.0` Long-Form And Large-Input Workflows

Primary outcome: handle long recordings without surprising resource spikes or unusable runtimes.

Scope:

- add preview-time cost estimation: predicted frames, estimated disk usage, expected transcript runtime class, temp-space warnings
- add chunked transcription pipeline for long recordings with partial results and manifest tracking
- add stage-level resume semantics for extraction, contact sheet generation, audio extraction, and transcription
- add adaptive defaults for long recordings: lower-density sampling, safe image defaults, delayed expensive post-processing
- add explicit confirmation or override flags for risky workloads
- add user-facing presets for laptop-safe, balanced, and high-fidelity modes

Exit criteria:

- long recordings no longer require all-or-nothing reruns
- users can predict cost before running expensive jobs
- chunked transcript output is resumable and inspectable

## `v0.3.0` Agent Retrieval And Workflow Depth

Primary outcome: make the tool more useful after extraction, not just during extraction.

Scope:

- improve artifact indexing and retrieval ergonomics for large output roots
- add stronger “latest run” and search-oriented agent workflows
- extend MCP observability for long operations and partial outputs
- add richer machine-readable summaries for downstream agent reasoning
- publish end-to-end agent recipes for debugging, incident review, and coding-session analysis

Exit criteria:

- agents can find useful artifacts quickly without path guesswork
- extraction results expose enough structure to support automated follow-up workflows

## `v0.4.0` Distribution And Contributor Scale

Primary outcome: reduce installation friction and increase maintainability as adoption grows.

Scope:

- add package-manager distribution where practical: Homebrew first, then winget and apt-style channels if worth the maintenance cost
- improve first-run setup and doctor flows to reduce support burden
- harden release automation and smoke validation around published artifacts
- expand contributor docs, fixtures, issue templates, and troubleshooting guidance

Exit criteria:

- install paths are simpler for non-Go users
- release confidence is high enough that shipping does not require manual heroics

## `v0.5.0` VSCode Extension And Enhanced UX

Primary outcome: provide one-click installation and visual UI for VSCode/Cursor users.

Scope:

- create VSCode extension that bundles FramesCLI MCP server
- add visual UI for configuring extraction settings (fps, format, presets)
- add quick actions: "Extract frames from this video" in file explorer context menu
- add inline frame preview panel for extracted runs
- add video file detection and suggested workflows
- integrate with VSCode's output panel for progress tracking
- publish to VSCode marketplace and Cursor extension store

Exit criteria:

- VSCode/Cursor users can install FramesCLI with one click from marketplace
- Common extraction workflows are accessible via UI without CLI knowledge
- Extension handles FramesCLI binary installation/updates automatically

**Note:** This complements (not replaces) the MCP registry listing. Users can choose between:
- MCP registry + manual config (works with ALL MCP clients)
- VSCode extension (one-click install, better UX, VSCode/Cursor only)

## Cross-Cutting Workstreams

These are parallel tracks that can run alongside milestone work.

### 1. Stability

- MCP integration harness
- wider media regression corpus
- structured error codes and recovery guidance
- better interrupted-run and partial-output handling

### 2. Scale

- chunking strategy
- resume manifests
- resource estimation
- safe defaults for long inputs

### 3. Adoption

- packaging
- setup/doctor polish
- better install and troubleshooting docs
- public issue triage templates

### 4. Agent UX

- artifact retrieval ergonomics
- better summaries for machine use
- improved MCP progress, partial results, and long-job reporting
- more recipe-driven documentation

## Long-Form Video Policy

This should become an explicit product policy.

FramesCLI should not imply that every machine can comfortably process every 4K multi-hour recording at maximum fidelity. Instead:

- short and medium recordings should work well with defaults
- long recordings on CPU-only machines should default to chunked and lower-cost modes
- very long recordings should trigger preview-time warnings and safer default presets
- full-fidelity processing for extreme inputs should remain possible, but it should be an explicit choice

## Recommended Large-Input Features

### Preview And Estimation

- predicted frame count
- estimated output size by image format and sampling density
- temp-space estimate
- transcript runtime class: fast, moderate, expensive, very expensive
- CPU-only and low-disk warnings

### Chunking And Resume

- chunk manifests persisted in run metadata
- partial transcript outputs merged into final transcript artifacts
- rerun only failed or missing chunks
- resume extraction and contact-sheet generation without rebuilding finished stages

### Safer Defaults

- prefer JPEG for large runs unless PNG is explicitly requested
- lower preview/extract density on long recordings
- allow transcript-first workflows before full frame extraction
- delay contact-sheet generation until core artifacts succeed

### User Controls

- `--preset laptop-safe`
- `--preset balanced`
- `--preset high-fidelity`
- `--allow-expensive`
- `--chunk-duration`
- `--resume`

## Priority Backlog

Use this as the implementation order unless a concrete user issue reprioritizes work.

1. MCP integration test harness
2. Expanded media fixture corpus
3. Structured error/recovery contract
4. Preview cost estimation
5. Chunked transcription manifests
6. Stage-level resume behavior
7. Long-input presets and safety gates
8. Artifact indexing and retrieval improvements
9. Packaging and release automation hardening

## What To Avoid

- do not add cloud complexity unless it solves a specific user problem that local workflows cannot handle
- do not promise unlimited-scale processing on consumer hardware
- do not expand the command surface faster than tests and docs can keep up
- do not optimize only for happy-path demo videos
