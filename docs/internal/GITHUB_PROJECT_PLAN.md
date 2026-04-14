# GitHub Labels, Milestones, and Triage Plan

Use this as the default project-management setup for FramesCLI.

## Labels

### Type

- `type:bug` - Incorrect behavior or regression
- `type:feature` - New user-facing capability
- `type:enhancement` - Improvement to existing behavior
- `type:docs` - Documentation-only changes
- `type:test` - Test coverage and quality improvements
- `type:refactor` - Internal code changes without user-facing behavior change
- `type:chore` - Maintenance, tooling, or repo housekeeping

### Area

- `area:cli` - CLI commands, flags, output contracts
- `area:tui` - Terminal UI and interaction flows
- `area:mcp` - MCP server and tool wrappers
- `area:media` - ffmpeg/ffprobe/whisper pipelines
- `area:config` - config loading, persistence, env overrides
- `area:docs` - README/checklist/contributing content
- `area:ci` - CI, preflight, release validation

### Priority

- `priority:P0` - Critical broken workflow or blocker
- `priority:P1` - High-impact issue for broad users
- `priority:P2` - Medium impact or narrower scope
- `priority:P3` - Nice-to-have

### Status

- `status:needs-triage` - New issue not yet classified
- `status:accepted` - Confirmed and planned
- `status:in-progress` - Actively being worked
- `status:blocked` - Waiting on dependency/decision
- `status:needs-repro` - Cannot proceed without repro details

### Special

- `good first issue` - Suitable for new contributors
- `help wanted` - Open for community contribution
- `breaking-change` - Requires migration note or versioning care

## Milestones

### `v0.1.1` (Stabilization)

Goal: Hardening after first public beta feedback.

Suggested scope:

- cross-platform smoke CI (Linux/macOS/WSL docs or matrix approximation)
- top extraction bug fixes and edge-case codec handling
- improved error and recovery messages in CLI/TUI
- MCP integration test harness for basic handshake/tool calls

### `v0.2.0` (Agent Workflows)

Goal: Make agent/IDE usage first-class.

Suggested scope:

- agent recipe docs with end-to-end examples
- optional hooks and post-processing adapters
- stronger artifact indexing and retrieval ergonomics
- improved MCP observability/timeouts and cancellation UX

### `v1.0.0` (General Availability)

Goal: Stable and well-documented core feature set.

Suggested scope:

- contract/versioning guarantees and migration notes
- automated release and smoke validation pipeline
- broader fixture corpus and regression gating
- finalized docs IA and polished onboarding

## Triage Rules

1. New issues default to `status:needs-triage` + one `type:*` label.
2. Within triage, always add one `area:*` and one `priority:*` label.
3. P0/P1 issues should be attached to the nearest active milestone.
4. Add `needs-repro` within 24 hours if no minimal repro is available.
5. Close with a linked PR and include test evidence whenever possible.

## Release Cadence Recommendation

- Patch (`v0.1.x`): weekly or biweekly as bugfixes accumulate
- Minor (`v0.x.0`): once major UX/feature slices are stable
- Keep release notes short: highlights, fixes, known limitations, upgrade notes
