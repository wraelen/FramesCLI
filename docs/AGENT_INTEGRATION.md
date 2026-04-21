# FramesCLI Agent Integration

This is the canonical hand-written setup-and-contract document for agent
integrations. Generated contract references live under `docs/schemas/`. For
copy-paste task flows, use [AGENT_RECIPES.md](AGENT_RECIPES.md).

## Choose an Interface

FramesCLI supports two agent-facing interfaces:

- MCP over stdio: preferred when the client already supports MCP
- CLI with `--json`: fallback when the agent only has shell access

Recommended order of preference:

1. MCP
2. CLI JSON

## Quick Start

Installation check:

```bash
framescli doctor --json
```

Minimal MCP config:

```json
{
  "mcpServers": {
    "framescli": {
      "command": "framescli",
      "args": ["mcp"]
    }
  }
}
```

Recommended MCP workflow:

1. `doctor`
2. `prefs_set`
3. `preview`
4. `extract`
5. `get_run_artifacts`
6. `get_latest_artifacts` only when the compact latest-artifact map is enough

## MCP Tool Surface

The generated MCP inventory lives in [schemas/README.md](schemas/README.md).
Use that file for the current tool names, descriptions, input fields, and
envelope `command` names. The machine-readable version is
[schemas/mcp-tools.json](schemas/mcp-tools.json).

Path model:

- Allowed local roots are `agent_input_dirs`, `agent_output_root`, `frames_root`,
  and the current working directory
- Agents should call `prefs_set` before sending local paths outside the current
  workspace
- Remote inputs must use `url`, not a filesystem path

## MCP Response Shape

Successful MCP tool calls return JSON-RPC responses with structured content. The
tool payload itself is the automation envelope:

```json
{
  "schema_version": "framescli.v1",
  "command": "preview",
  "status": "success",
  "started_at": "2026-04-19T04:34:32Z",
  "ended_at": "2026-04-19T04:34:32Z",
  "duration_ms": 7,
  "data": {
    "target_fps": 8,
    "fps_mode": "auto"
  }
}
```

On failure, the envelope stays stable and `error` is populated:

```json
{
  "schema_version": "framescli.v1",
  "command": "preview",
  "status": "error",
  "started_at": "2026-04-19T04:34:32Z",
  "ended_at": "2026-04-19T04:34:32Z",
  "duration_ms": 0,
  "data": {
    "input": "/tmp/does-not-exist.mp4"
  },
  "error": {
    "code": "command_failed",
    "class": "file_not_found",
    "message": "file not found: /tmp/does-not-exist.mp4",
    "recovery": "Check the path is correct, or use 'recent' for the most recently modified video in your input dirs."
  }
}
```

Long-running MCP calls emit `notifications/message` heartbeats roughly every
10 seconds so clients can surface progress while extraction or transcription is
still running.

## CLI JSON Contract

The CLI fallback uses the same automation envelope for:

- `extract --json`
- `extract-batch --json`
- `preview --json`
- `artifacts --json`
- `open-last --json`
- `transcribe-run --json`

Example:

```bash
framescli preview video.mp4 --mode both --json
framescli extract video.mp4 --voice --preset balanced --json
framescli artifacts latest --json
```

Important exception:

- `framescli doctor --json` returns a doctor report, not the common automation
  envelope

That doctor report is still stable and agent-friendly, but it is a separate JSON
shape.

## Artifact Retrieval

Preferred retrieval order:

1. `get_run_artifacts`
2. `get_latest_artifacts`
3. `open_last`

Use `get_run_artifacts` when the agent needs indexed run metadata such as frame
counts, format, preset, transcript presence, and artifact availability.

Use `get_latest_artifacts` when the agent only needs the compact latest-artifact
map:

```json
{
  "root": "/Users/me/frames-out",
  "artifacts": {
    "run": "/Users/me/frames-out/Run_20260419-043432-a3f9c2",
    "metadata": "/Users/me/frames-out/Run_20260419-043432-a3f9c2/run.json",
    "frames": "/Users/me/frames-out/Run_20260419-043432-a3f9c2/frames.json",
    "contact_sheet": "/Users/me/frames-out/Run_20260419-043432-a3f9c2/images/sheets/contact-sheet.png"
  }
}
```

## Recommended Agent Behavior

- Call `doctor` first
- Call `preview` before expensive work
- Respect guardrails and present warnings to the user
- Prefer explicit presets over ad-hoc flag combinations for long inputs
- Use `transcribe_run` to resume interrupted transcript work instead of rerunning
  extraction
- Treat `error.class`, `error.recovery`, and `error.retryable` as additive hints,
  not as the only contract

## References

- [Agent Recipes](AGENT_RECIPES.md)
- [JSON Schemas](schemas/README.md)
- `internal/contracts/` for the source-of-truth registry that drives `tools/list`
  and generated schema docs
