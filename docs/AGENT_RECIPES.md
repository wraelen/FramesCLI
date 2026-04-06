# FramesCLI Agent Recipes

This page provides copy-paste workflows for coding agents and IDE agents.

## Recipe 1: CLI JSON Fast Path

Use this for agents that can run shell commands and parse JSON output.

```bash
framescli doctor --json
framescli preview /path/to/recording.mp4 --mode both --json
framescli extract /path/to/recording.mp4 --voice --json
framescli open-last --artifact transcript --json
```

Expected pattern:

1. `doctor` verifies local toolchain.
2. `preview` estimates work before running extraction.
3. `extract` returns structured artifact paths.
4. `open-last`/`copy-last` retrieves key outputs for downstream reasoning.

## Recipe 2: MCP First-Run Setup

Use this for IDE agents with MCP client support.

Start server:

```bash
framescli mcp
```

Call order:

1. `doctor`
2. `prefs_set`
3. `preview`
4. `extract` or `extract_batch`
5. `get_latest_artifacts`

Example `prefs_set` payload:

```json
{
  "input_dirs": ["/Users/me/Recordings", "/mnt/d/Captures"],
  "output_root": "/Users/me/frames-out"
}
```

## Recipe 3: Batch Debug Archive

For incident/debug sessions with many recordings:

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

## Recipe 4: CI/Automation Guardrails

Use command exit codes and JSON status together:

- Non-zero exit code: command failure.
- JSON `status=error` or `status=partial`: treat as failed pipeline stage.

Minimal guard command set:

```bash
framescli doctor --json
framescli preview recent --json
```

## Operational Notes

- Keep paths local; MCP access is restricted to local allowed roots.
- Use `prefs_set` once per workspace to reduce prompt/tool noise.
- Prefer `preview` before expensive extraction jobs.
- Optional automation hook: set `post_extract_hook` in config or pass `--post-hook` in CLI calls.
- Optional local telemetry: set `telemetry_enabled=true` to append JSONL events under `frames/telemetry/`.
