# FramesCLI Agent Recipes

This page contains copy-paste flows. For setup rules, response shapes, and path
safety, see the canonical guide:
[AGENT_INTEGRATION.md](AGENT_INTEGRATION.md).

## MCP Happy Path

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

Recommended call sequence:

1. `doctor`
2. `prefs_set`
3. `preview`
4. `extract`
5. `get_run_artifacts`

Example `prefs_set`:

```json
{
  "input_dirs": ["/Users/me/Recordings", "/mnt/d/Captures"],
  "output_root": "/Users/me/frames-out"
}
```

Example `preview` using automatic fps:

```json
{
  "input": "recent",
  "fps": "auto",
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
    "target_fps": 8,
    "fps_mode": "auto",
    "format": "jpg",
    "estimate": {
      "frame_count": 96,
      "disk_summary": "~6-14 MB for extracted frames + sheet"
    }
  }
}
```

Example `extract`:

```json
{
  "input": "recent",
  "fps": 0,
  "voice": true,
  "format": "jpg",
  "preset": "balanced",
  "hwaccel": "auto"
}
```

Notes:

- MCP accepts `fps` as a positive number, `0`, or `"auto"`
- `fps: 0` and `fps: "auto"` both request automatic sampling
- `extract` now returns `fps_mode: "auto"` when auto sampling is used

## Artifact Lookup

Preferred indexed lookup:

```json
{
  "root": "/Users/me/frames-out",
  "run": "latest"
}
```

Recent runs lookup:

```json
{
  "root": "/Users/me/frames-out",
  "recent": 5
}
```

Compact latest-only lookup:

```json
{
  "root": "/Users/me/frames-out"
}
```

Use `get_run_artifacts` when the agent needs run metadata. Use
`get_latest_artifacts` when the agent only needs the compact path map under
`data.artifacts`.

## Resume Transcription

Use `transcribe_run` when a run already contains extracted audio but the
transcript is missing, incomplete, or timed out:

```json
{
  "run_dir": "/Users/me/frames-out/Monday_7-26pm-a3f9c2",
  "chunk_duration": 600
}
```

Chunked runs persist `voice/transcription-manifest.json` and resume any chunk
still marked `pending`, `in_progress`, or `failed`.

## CLI JSON Fast Path

Use this when the agent only has shell access:

```bash
framescli doctor --json
framescli preview /path/to/recording.mp4 --mode both --json
framescli extract /path/to/recording.mp4 --voice --transcribe-timeout 300 --json
framescli artifacts latest --json
```

Practical pattern:

1. `doctor --json` verifies dependencies
2. `preview --json` estimates cost and guardrails
3. `extract --json` performs the work
4. `artifacts --json` returns the indexed run view

If transcription timed out:

```bash
framescli transcribe-run /path/to/run --chunk-duration 600 --json
```

Important contract notes:

- `doctor --json` is a standalone report, not the common automation envelope
- `preview` is advisory and does not guarantee the source contains audio
- `extract --voice` and `transcribe-run` fail fast on silent inputs
- `open-last` remains useful for direct single-path retrieval such as
  `transcript-json`, `manifest`, `metadata-csv`, or `frames-zip`

## Batch Processing

For incident or archive processing:

```bash
framescli extract-batch "recordings/*.mp4" \
  --voice \
  --metadata-csv \
  --zip \
  --json
```

Optional index rebuild:

```bash
framescli index frames --out frames/index.json
```

The index at `<frames_root>/index.json` is refreshed after successful
`extract` and `transcribe-run` operations and can be rebuilt with
`framescli index` if runs were edited manually.

## Operational Notes

- Prefer `preview` before long recordings
- Prefer `--transcribe-timeout <seconds>` in unattended agent flows
- `laptop-safe` is the safest preset for very long CPU-bound runs
- MCP long-running tools emit heartbeat notifications every 10 seconds
- The MCP harness lives in `cmd/frames/mcp_integration_test.go`
