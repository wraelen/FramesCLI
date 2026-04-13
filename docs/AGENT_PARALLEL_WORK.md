# FramesCLI Parallel Work Prompts

Use these prompts to split work across multiple Codex agents without stepping on each other.

Each prompt is intentionally scoped to one workstream. Assign one agent per prompt. If two prompts need to touch the same files heavily, sequence them instead of running them at the same time.

## Prompt 1: MCP Integration Harness

You are working in the FramesCLI repo. Build an MCP integration test harness for `framescli mcp` with a focus on stability and agent trust.

Goals:

- cover server startup and basic handshake behavior
- cover at least `doctor`, `preview`, and one long-running tool path
- cover cancellation and timeout behavior where feasible
- verify progress or heartbeat messages for long-running operations
- keep tests deterministic and suitable for CI where possible

Constraints:

- prefer small, focused changes
- add or update docs if the test workflow needs explanation
- do not redesign unrelated product behavior

Deliverables:

- automated tests
- any small supporting test helpers or fixtures
- concise docs or comments explaining the harness

Before changing code, inspect existing MCP implementation and test structure. After changes, run the relevant tests and summarize any remaining gaps.

## Prompt 2: Media Fixture Expansion

You are working in the FramesCLI repo. Expand the automated media regression corpus so the project covers more real-world failure modes.

Goals:

- identify current fixture coverage and gaps
- add fixtures or fixture-generation helpers for edge cases like VFR, no-audio, odd codecs, high resolution, long duration, and corrupted inputs
- add integration tests that assert stable behavior and useful failure messages
- keep fixture sizes reasonable for repo health

Constraints:

- prefer generated fixtures where possible
- avoid introducing huge binary assets unless clearly justified
- keep test assertions high signal and not brittle

Deliverables:

- new or improved fixtures
- tests covering added scenarios
- doc updates if fixture generation or execution rules change

Before editing, map the current fixture/test layout. After changes, run the relevant test commands and report what is still not covered.

## Prompt 3: Error Contract And Recovery Messaging

You are working in the FramesCLI repo. Improve the reliability and clarity of error handling across CLI, TUI, and JSON/MCP-facing outputs.

Goals:

- identify inconsistent or vague failure messages
- define a cleaner error taxonomy for common failure classes
- improve recovery guidance for missing dependencies, invalid inputs, unsupported media, disk-space problems, and interrupted runs
- preserve machine-readable behavior for automation users

Constraints:

- keep compatibility risks explicit
- do not make broad refactors without proving the value
- add tests for any behavior that becomes contract-like

Deliverables:

- code changes
- tests
- concise docs on any user-visible error contract changes

Inspect existing error paths first. Then make the smallest defensible set of changes that improve trust and debuggability.

## Prompt 4: Preview Cost Estimation

You are working in the FramesCLI repo. Add preview-time cost estimation so users can understand workload size before running expensive jobs.

Goals:

- estimate frame count for the planned extraction
- estimate likely disk usage for common formats and sampling densities
- surface transcript runtime class or cost hints
- warn on risky long-form or CPU-only scenarios
- expose results in both human-readable and JSON output where appropriate

Constraints:

- keep estimates transparent and approximate, not falsely precise
- avoid blocking normal workflows with overly aggressive warnings
- preserve existing preview behavior unless there is a strong reason to change it

Deliverables:

- implementation
- tests
- doc updates with examples of the new preview output

Inspect `preview`, `doctor`, and benchmark-related code before editing so the estimation model fits the existing architecture.

## Prompt 5: Chunked Transcription And Resume Manifests

You are working in the FramesCLI repo. Design and implement a first pass of chunked transcription for long recordings with resumable manifests.

Goals:

- add a manifest model that tracks chunk boundaries and per-chunk state
- support resuming incomplete chunked transcription runs
- merge partial outputs into final transcript artifacts
- keep the design compatible with future chunked extraction or other resumable stages

Constraints:

- do not overbuild a generic job engine if a narrower design is enough for this phase
- make on-disk state easy to inspect and debug
- document tradeoffs and known limitations

Deliverables:

- implementation
- tests
- doc updates for long-form workflows and resume behavior

Read the current transcription and `transcribe-run` flow first. Prefer a clean manifest format over clever abstractions.

## Prompt 6: Long-Input Presets And Safety Gates

You are working in the FramesCLI repo. Add user-facing presets and guardrails for long or resource-heavy recordings.

Goals:

- define safe presets such as `laptop-safe`, `balanced`, and `high-fidelity`
- add warnings or explicit override requirements for clearly expensive jobs
- make defaults adapt better to long recordings without removing expert control
- keep behavior understandable from docs and help text

Constraints:

- avoid surprise behavior changes without surfacing them clearly
- preserve backward compatibility where possible
- tie warnings to measurable conditions, not vague heuristics

Deliverables:

- implementation
- tests
- help text and docs updates

Inspect current preset, sampling, and transcription-default behavior before editing. Explain any compatibility tradeoffs in your summary.

## Prompt 7: Artifact Indexing And Retrieval

You are working in the FramesCLI repo. Improve post-extraction artifact discovery so agents and humans can find useful outputs faster.

Goals:

- review current `index`, `open-last`, `copy-last`, and MCP artifact flows
- improve retrieval ergonomics for large output roots
- add better machine-readable summaries for latest or matching runs
- keep agent workflows simple and deterministic

Constraints:

- prefer additive improvements over breaking command changes
- keep output contracts clear
- update recipes if the recommended workflow changes

Deliverables:

- implementation
- tests
- docs updates for agent and CLI retrieval flows

Inspect existing run metadata and indexing logic first. Optimize for the common question: “what is the most useful artifact from the latest relevant run?”

## Prompt 8: Packaging And Release Hardening

You are working in the FramesCLI repo. Improve installation and release reliability for broader adoption.

Goals:

- evaluate the current installer and release flow
- implement the highest-value next packaging step, likely Homebrew first
- improve release smoke validation for published artifacts
- reduce first-run friction and support burden

Constraints:

- prefer maintainable distribution work over flashy breadth
- avoid introducing channels that will be hard to keep current
- document the operational workflow clearly

Deliverables:

- packaging or release-automation changes
- validation steps
- docs updates for installation and release maintenance

Inspect existing scripts, release docs, and GitHub workflow assumptions before editing. Make sure the chosen packaging path is worth the maintenance cost.

## Coordination Notes

- Safe to run in parallel:
  - Prompt 1 with Prompt 4
  - Prompt 2 with Prompt 8
  - Prompt 3 with Prompt 7
- Needs coordination:
  - Prompt 5 with Prompt 6
  - Prompt 4 with Prompt 6
  - Prompt 5 with Prompt 7
- Best first wave:
  1. Prompt 1
  2. Prompt 2
  3. Prompt 3
  4. Prompt 4

These four create the foundation for the long-form and agent-UX work that follows.
