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
5. `get_run_artifacts`
6. `get_latest_artifacts` only when the agent needs the compact latest-path map instead of full indexed run metadata

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

Example `preview` response excerpt:

```json
{
  "command": "preview",
  "status": "success",
  "data": {
    "target_fps": 2,
    "format": "jpg",
    "estimate": {
      "frame_count": 2400,
      "disk_summary": "~137-305 MB for extracted frames + sheet",
      "disk_profiles": [
        {
          "label": "selected",
          "format": "jpg",
          "fps": 2,
          "frame_count": 2400,
          "disk_summary": "~137-305 MB",
          "selected": true
        }
      ],
      "transcript": {
        "enabled": true,
        "backend": "faster-whisper",
        "runtime_class": "moderate",
        "cost_hint": "transcript runtime depends on backend, model, and local hardware"
      },
      "warnings": []
    }
  }
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

Example `get_run_artifacts` payloads:

```json
{
  "root": "/Users/me/frames-out",
  "run": "latest"
}
```

```json
{
  "root": "/Users/me/frames-out",
  "recent": 5
}
```

Resume transcription for a completed run:

```json
{
  "run_dir": "/Users/me/frames-out/Monday_7-26pm-a3f9c2",
  "chunk_duration": 600
}
```

Call this with the `transcribe_run` MCP tool when a run already has extracted audio under `voice/` but the transcript is missing or incomplete. Chunked runs persist `voice/transcription-manifest.json` and resume from any chunk still marked `pending`, `in_progress`, or `failed`.

## CLI JSON Fast Path

Use this for agents that can run shell commands and parse JSON output.

```bash
framescli doctor --json
framescli preview /path/to/recording.mp4 --mode both --json
framescli extract /path/to/recording.mp4 --voice --transcribe-timeout 300 --json
framescli artifacts latest --json
```

Expected pattern:

1. `doctor` verifies local toolchain.
2. `preview` estimates frame count, disk ranges, transcript cost hints, resolved preset defaults, and guardrails before running extraction.
3. `extract` returns structured artifact paths.
4. `artifacts` returns the stable indexed view for the latest or a specific run; `open-last` and `copy-last` remain useful for direct single-path retrieval such as `transcript-json`, `transcript-srt`, `manifest`, `metadata-csv`, or `frames-zip`.

Contract notes:

- `preview` is advisory. It reports transcript workload estimates, but it does not guarantee the source actually contains an audio stream.
- Prefer explicit workflow presets for long inputs:
  - `laptop-safe` => `1fps`, `jpg`, media preset `safe`, transcript chunking `300s`
  - `balanced` => `4fps`, `png`, media preset `balanced`, transcript chunking `600s`
  - `high-fidelity` => `8fps`, `png`, media preset `fast`, transcript chunking `900s`
- Expensive workload blocks are measurable and explicit. `extract` requires `--allow-expensive` when any blocking threshold is crossed:
  - estimated frames `>= 40000`
  - estimated extracted frame disk usage `>= 4 GB`
  - duration `>= 3 hours`
  - CPU-only `slow`/`heavy` transcript path on recordings `>= 45 minutes` without chunking
- Warning thresholds start at `20000` estimated frames, `2 GB` estimated disk, `2 hours` duration, and CPU-only `slow`/`heavy` transcript paths at `30 minutes`.
- Explicit flags still override preset defaults. Use `--fps`, `--format`, or `--chunk-duration` when an agent intentionally needs a different tradeoff.
- Configured `performance-mode` applies preset sampling/format implicitly when `default-fps` and `default-format` are still at the stock defaults; custom config defaults continue to win otherwise.
- `extract --voice` and `transcribe-run` fail fast on silent inputs with `requested audio output, but video has no audio stream: <path>`.
- `open-last` and `copy-last` accept `run`, `transcript`, `transcript-json`, `transcript-srt`, `transcript-vtt`, `sheet`, `log`, `metadata`, `frames`, `manifest`, `metadata-csv`, `frames-zip`, and `audio`.
- JSON and MCP failures keep stable top-level codes while also exposing additive recovery metadata via `error.class`, `error.recovery`, `error.retryable`, or `error.data.*` on MCP responses.

If extraction succeeded but transcription timed out, resume later with:

```bash
framescli transcribe-run /path/to/run --chunk-duration 600 --json
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

The artifact index lives at `<frames_root>/index.json`, is refreshed after successful `extract` and `transcribe-run` operations, and can be rebuilt explicitly with `framescli index` if runs were edited manually on disk.

## Pipeline Notes

- For agent pipelines, prefer `--transcribe-timeout <seconds>` on `framescli extract --voice` so a slow transcription does not block the entire run forever.
- On CPU-only machines, Whisper `base` and larger models can be very slow. FramesCLI warns on CPU use and auto-selects `tiny` when the model would otherwise fall back to the default `base`.
- On GPU-equipped machines, larger models are much more practical, especially for longer recordings.
- MCP long-running tools emit `notifications/message` heartbeats every 10 seconds so OpenClaw and other clients can surface progress while extraction or resume transcription is still running.
- Keep paths local. MCP path access is restricted to allowed roots from config and the current workspace.
- The MCP stdio harness lives in `cmd/frames/mcp_integration_test.go`; use `go test ./cmd/frames` for the full package or `go test ./cmd/frames -run 'TestMCPServer|TestMCPHelperProcess'` when focusing on MCP behavior only.
