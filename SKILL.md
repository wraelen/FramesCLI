---
name: framescli
description: Use this skill when you need to turn local recordings into frames, transcripts, metadata, or agent-readable artifacts with FramesCLI. Prefer MCP for structured agent integrations, and use CLI JSON mode as fallback.
---

# FramesCLI Skill

Use FramesCLI when the user needs local video-processing help for debugging, coding-session review, OpenClaw analysis, or agent-ready artifact generation.

## Choose the Integration Surface

- Prefer `framescli mcp` when the client supports MCP.
- Use CLI JSON commands when MCP is not available.
- Do not invent a separate HTTP workflow unless the user explicitly asks for a network service.

## MCP Workflow

1. Run `framescli doctor --json`.
2. Start `framescli mcp`.
3. Call `prefs_set` with:
   - `input_dirs`
   - `output_root`
4. Call tools in this order:
   - `preview`
   - `extract` or `extract_batch`
   - `get_latest_artifacts`
5. If a run already has audio but no transcript, call `transcribe_run`.

## CLI JSON Workflow

Use this when shell commands are easier than MCP:

```bash
framescli doctor --json
framescli preview /abs/path/to/video.mp4 --mode both --json
framescli extract /abs/path/to/video.mp4 --voice --json
framescli open-last --artifact transcript --json
```

If extraction succeeds but transcription is skipped or times out:

```bash
framescli transcribe-run /abs/path/to/run --json
```

## Operational Rules

- Use absolute paths whenever possible.
- Run `doctor` first on unfamiliar machines.
- Prefer `--transcribe-timeout <seconds>` for agent flows so transcription does not block the whole workflow indefinitely.
- `--fps auto` or `--fps 0` targets roughly 60 frames over the whole video.
- Keep processing local; FramesCLI is designed around local files and local paths.

## Key Outputs

- `run.json`
- `frames.json`
- `images/`
- `images/sheets/contact-sheet.png`
- `voice/voice.wav`
- `voice/transcript.txt`
- `voice/transcript.json`

## References

- Agent recipes: `docs/AGENT_RECIPES.md`
- Main usage and install docs: `README.md`
