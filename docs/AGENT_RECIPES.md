# FramesCLI Agent Recipes

This page provides copy-paste workflows for coding agents, MCP clients, and OpenClaw.

## OpenClaw MCP Recipe

Use this when the agent can connect to a stdio MCP server and should stay on structured tool calls instead of shelling out directly.

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

Recommended tool sequence:

1. `doctor`
2. `prefs_set`
3. `preview`
4. `extract`
5. `get_latest_artifacts`

Example `prefs_set` payload:

```json
{
  "input_dirs": ["/Users/me/Recordings", "/mnt/d/Captures"],
  "output_root": "/Users/me/frames-out"
}
```

Example `preview` payload:

```json
{
  "input": "recent",
  "fps": 2,
  "format": "jpg",
  "mode": "both"
}
```

Example `extract` payload:

```json
{
  "input": "recent",
  "fps": 2,
  "voice": true,
  "format": "jpg",
  "preset": "balanced",
  "hwaccel": "auto"
}
```

Example `get_latest_artifacts` payload:

```json
{
  "root": "/Users/me/frames-out"
}
```

Resume transcription for a completed run:

```json
{
  "run_dir": "/Users/me/frames-out/Monday_7-26pm-a3f9c2"
}
```

Call this with the `transcribe_run` MCP tool when a run already has extracted audio under `voice/` but the transcript is missing or incomplete.

## CLI JSON Fast Path

Use this for agents that can run shell commands and parse JSON output.

```bash
framescli doctor --json
framescli preview /path/to/recording.mp4 --mode both --json
framescli extract /path/to/recording.mp4 --voice --transcribe-timeout 300 --json
framescli open-last --artifact transcript --json
```

Expected pattern:

1. `doctor` verifies local toolchain.
2. `preview` estimates work before running extraction.
3. `extract` returns structured artifact paths.
4. `open-last` or `copy-last` retrieves key outputs for downstream reasoning.

If extraction succeeded but transcription timed out, resume later with:

```bash
framescli transcribe-run /path/to/run --json
```

## Batch Debug Archive

For incident or debug sessions with many recordings:

```bash
framescli extract-batch "recordings/*.mp4" \
  --voice \
  --metadata-csv \
  --zip \
  --json
```

Recommended follow-up:

```bash
framescli index frames --out frames/index.json
framescli tui --root frames
```

## Pipeline Notes

- For agent pipelines, prefer `--transcribe-timeout <seconds>` on `framescli extract --voice` so a slow transcription does not block the entire run forever.
- On CPU-only machines, Whisper `base` and larger models can be very slow. FramesCLI warns on CPU use and auto-selects `tiny` when the model would otherwise fall back to the default `base`.
- On GPU-equipped machines, larger models are much more practical, especially for longer recordings.
- MCP long-running tools emit `notifications/message` heartbeats every 10 seconds so OpenClaw and other clients can surface progress while extraction or resume transcription is still running.
- Keep paths local. MCP path access is restricted to allowed roots from config and the current workspace.
