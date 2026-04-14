# FramesCLI Agent Integration Guide

**For AI Coding Assistants**: This guide helps AI agents integrate with FramesCLI to make any video AI-readable through structured, searchable artifacts.

## What FramesCLI Does for Agents

FramesCLI extracts timestamped frames and transcripts from any video, enabling agents to:

- Turn tutorials and demos into step-by-step documentation
- Extract knowledge from meeting recordings and presentations
- Analyze screen recordings for debugging and troubleshooting
- Process educational content into searchable, referenceable formats
- Generate summaries and timelines from any video content

**Primary Interface:** MCP (Model Context Protocol) stdio server
**Fallback Interface:** CLI with `--json` output

---

## 30-Second Agent Cheat Sheet

**Installation check:** `framescli doctor --json`
**MCP workflow:** `doctor` → `prefs_set` → `preview` → `extract` → `get_latest_artifacts`
**CLI workflow:** `framescli extract video.mp4 --voice --json`
**Preset guide:** `laptop-safe` (<30min) | `balanced` (default) | `high-fidelity` (short+GPU)
**Cost estimation:** Always call `preview` first for large videos
**Path safety:** Must call `prefs_set` before MCP `extract`

---

## Quick Start for Agents

### 1. Verify Installation

```bash
framescli doctor --json
```

**Expected response:**
```json
{
  "tools": [
    {"name": "ffmpeg", "required": true, "found": true},
    {"name": "ffprobe", "required": true, "found": true},
    {"name": "whisper", "required": false, "found": true}
  ],
  "required_failed": false
}
```

If `required_failed: true`, user needs to install dependencies:
```bash
# Install script can help
curl -fsSL https://raw.githubusercontent.com/wraelen/framescli/main/scripts/install-release.sh | bash
```

### 2. Choose Integration Mode

| Mode | When to Use | Setup Complexity |
|------|-------------|------------------|
| **MCP Server** | Agent supports MCP (Cursor, Cline, Claude Desktop) | Low |
| **CLI JSON** | Shell access available | Minimal |

---

## MCP Integration (Recommended)

### Cursor / Claude Desktop / Cline Setup

**Add to MCP client config:**

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

**Config file locations:**
- **Cursor:** `~/Library/Application Support/Cursor/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json`
- **Claude Desktop:** `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS) or `%APPDATA%/Claude/claude_desktop_config.json` (Windows)
- **Cline:** Check Cline settings panel for MCP servers

### Available MCP Tools

| Tool | Purpose | Required First? |
|------|---------|-----------------|
| `doctor` | Verify ffmpeg/whisper installation | ✅ Yes (first call) |
| `prefs_set` | Configure allowed input/output paths | ✅ Yes (before extract) |
| `preview` | Estimate extraction cost (frames, disk, time) | 🟡 Recommended |
| `extract` | Extract frames + optional transcript from video | Required for processing |
| `extract_batch` | Process multiple videos | Optional |
| `transcribe_run` | Add/resume transcription for existing run | Optional |
| `get_latest_artifacts` | Get paths to most recent run outputs | Common |
| `get_run_artifacts` | Query specific run or recent N runs | Common |
| `open_last` | Get path to specific artifact type | Utility |

### Standard MCP Workflow

```
┌─────────────────────────────────────────────────┐
│  Agent receives: "Analyze recording.mp4"        │
└─────────────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────────────┐
│  1. Call: doctor                                │
│     → Verify tools available                    │
└─────────────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────────────┐
│  2. Call: prefs_set                             │
│     → Set input_dirs: ["/Users/me/Videos"]      │
│     → Set output_root: "/Users/me/frames"       │
└─────────────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────────────┐
│  3. Call: preview                               │
│     → Input: "recording.mp4"                    │
│     → Get: frame_count, disk estimate, warnings │
└─────────────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────────────┐
│  4. Call: extract                               │
│     → Input: "recording.mp4"                    │
│     → Voice: true (for transcript)              │
│     → Preset: "balanced"                        │
└─────────────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────────────┐
│  5. Call: get_latest_artifacts                  │
│     → Retrieve: transcript.json, frames.json    │
└─────────────────────────────────────────────────┘
                    ↓
┌─────────────────────────────────────────────────┐
│  6. Read artifacts and analyze                  │
│     → Parse transcript for spoken content       │
│     → Parse frames.json for visual timeline     │
│     → Generate summary for user                 │
└─────────────────────────────────────────────────┘
```

### Example MCP Tool Calls

**1. Configure paths (first time only):**
```json
{
  "tool": "prefs_set",
  "arguments": {
    "input_dirs": ["/Users/me/Recordings", "/Users/me/Videos"],
    "output_root": "/Users/me/frames-output"
  }
}
```

**2. Preview before extraction:**
```json
{
  "tool": "preview",
  "arguments": {
    "input": "recent",
    "fps": 4,
    "format": "jpg",
    "mode": "both"
  }
}
```

**Response:**
```json
{
  "command": "preview",
  "status": "success",
  "data": {
    "estimate": {
      "frame_count": 1200,
      "disk_summary": "~68-152 MB for extracted frames + sheet",
      "transcript": {
        "runtime_class": "fast",
        "cost_hint": "GPU transcription should complete quickly"
      },
      "guardrails": []
    }
  }
}
```

**3. Extract with transcript:**
```json
{
  "tool": "extract",
  "arguments": {
    "input": "recent",
    "voice": true,
    "preset": "balanced",
    "fps": 4,
    "format": "png"
  }
}
```

**4. Get results:**
```json
{
  "tool": "get_latest_artifacts",
  "arguments": {
    "root": "/Users/me/frames-output"
  }
}
```

**Response:**
```json
{
  "run_dir": "/Users/me/frames-output/Run_20260413-143022-a3f9c2",
  "run_json": "/Users/me/frames-output/Run_20260413-143022-a3f9c2/run.json",
  "frames_json": "/Users/me/frames-output/Run_20260413-143022-a3f9c2/frames.json",
  "transcript_json": "/Users/me/frames-output/Run_20260413-143022-a3f9c2/voice/transcript.json",
  "transcript_txt": "/Users/me/frames-output/Run_20260413-143022-a3f9c2/voice/transcript.txt",
  "images_dir": "/Users/me/frames-output/Run_20260413-143022-a3f9c2/images"
}
```

### MCP Error Handling

All MCP errors include:
- `code`: Stable error identifier (e.g., `"command_failed"`)
- `message`: Human-readable description
- `data.class`: Error category (`validation`, `execution`, `not_found`)
- `data.recovery`: Suggested fix for agent/user
- `data.retryable`: Boolean indicating if retry might succeed

**Example error response:**
```json
{
  "error": {
    "code": "command_failed",
    "message": "requested audio output, but video has no audio stream",
    "data": {
      "class": "validation",
      "recovery": "Remove --voice flag or use a video with an audio track",
      "retryable": false
    }
  }
}
```

**Agent should:**
1. Check `data.retryable` before retrying
2. Present `data.recovery` to user if action needed
3. Log `data.class` for debugging

### MCP Path Safety

For security, MCP tools only access:
1. Configured `input_dirs` from `prefs_set`
2. Configured `output_root` from `prefs_set`
3. Current working directory

**Agent must call `prefs_set` before `extract`** or path validation will fail.

### MCP Long-Running Operations

For operations >10 seconds, FramesCLI sends heartbeat notifications:

```json
{
  "method": "notifications/message",
  "params": {
    "level": "info",
    "message": "Extraction in progress: 45% complete"
  }
}
```

**Agents should:**
- Display progress to user
- Not timeout during heartbeats
- Allow user cancellation (send SIGINT to framescli process)

---

## CLI JSON Integration (Fallback)

When MCP is unavailable, use CLI commands with `--json` flag.

### Standard CLI Workflow

```bash
# 1. Verify installation
framescli doctor --json

# 2. Preview workload
framescli preview /path/to/video.mp4 --mode both --json

# 3. Extract frames + transcript
framescli extract /path/to/video.mp4 --voice --preset balanced --json

# 4. Get artifact paths
framescli artifacts latest --json
```

### CLI JSON Response Envelope

All commands return:
```json
{
  "schema_version": "framescli.v1",
  "command": "extract",
  "status": "success|partial|failed",
  "started_at": "2026-04-13T14:30:00Z",
  "completed_at": "2026-04-13T14:32:15Z",
  "data": { /* command-specific payload */ },
  "error": { /* present if status != success */ }
}
```

**Exit codes:**
- `0` = success
- `1` = failed
- Check `status` field in JSON for partial completions

### CLI Error Handling

```json
{
  "status": "failed",
  "error": {
    "code": "command_failed",
    "message": "ffmpeg not found in PATH",
    "class": "validation",
    "recovery": "Install ffmpeg: brew install ffmpeg (macOS) or sudo apt install ffmpeg (Linux)",
    "retryable": false
  }
}
```

---

## Common Agent Workflows

### Workflow 1: Debug Session Analysis

**User request:** "Analyze this bug reproduction video"

**Agent steps:**
1. Call `doctor` to verify tools
2. Call `preview` to estimate cost (warn if >5GB)
3. Call `extract` with `voice: true, preset: "balanced"`
4. Read `transcript.json` for spoken errors/context
5. Read `frames.json` for visual timeline
6. Identify error timestamps and corresponding frames
7. Summarize findings with timestamps

**Key artifacts to read:**
- `voice/transcript.json` - Full transcript with word-level timestamps
- `frames.json` - Frame metadata with exact timestamps
- `images/frame-XXXX.png` - Visual frames at error moments

### Workflow 2: Incident Review

**User request:** "Summarize what happened in this incident recording"

**Agent steps:**
1. Extract with `--voice --preset laptop-safe` (faster, lower disk)
2. Parse `transcript.txt` for timeline of events
3. Generate incident summary with:
   - Timeline of actions (from transcript)
   - Key decision points (from transcript + frame timestamps)
   - Error states (from frames.json metadata)

**Optimization:**
- Use `--fps 1` for hour+ recordings
- Use `--transcribe-timeout 300` to avoid blocking on slow transcription
- If timeout, call `transcribe_run` later to resume

### Workflow 3: Coding Session Documentation

**User request:** "Document this pair programming session"

**Agent steps:**
1. Extract with `--voice --preset balanced`
2. Parse transcript for:
   - Code discussion topics
   - Decision rationale
   - Action items
3. Generate markdown doc with:
   - Session summary
   - Key decisions with timestamps
   - Follow-up tasks mentioned

### Workflow 4: Batch Processing

**User request:** "Process all recordings in my Videos folder"

**Agent steps:**
1. Call `extract_batch` with glob pattern: `"/Users/me/Videos/*.mp4"`
2. Poll `get_run_artifacts` with `recent: 10` to get all runs
3. For each run, read artifacts and generate summary
4. Compile consolidated report

---

## Framework-Specific Integration Examples

### Cursor AI

**1. Configure MCP server:**
Open Cursor settings → MCP Servers → Add:
```json
{
  "framescli": {
    "command": "framescli",
    "args": ["mcp"]
  }
}
```

**2. Example prompt:**
```
@framescli analyze the most recent screen recording and tell me what errors occurred
```

Cursor will:
1. Call `doctor` to verify installation
2. Call `prefs_set` if not configured (prompt user for paths)
3. Call `preview` on recent video
4. Call `extract` with appropriate settings
5. Read transcript and frames
6. Provide analysis

### Cline

**Setup via Cline MCP panel:**
1. Open Cline settings
2. Add MCP server: `framescli mcp`
3. Cline auto-discovers tools

**Example task:**
```
Use framescli to extract frames from bug-repro.mp4 and help me find where the crash occurred
```

### Windsurf

**Configure Cascade with MCP:**
```json
{
  "mcp": {
    "servers": {
      "framescli": {
        "command": "framescli",
        "args": ["mcp"]
      }
    }
  }
}
```

### DIY Agent (Using CLI)

**Python example:**
```python
import subprocess
import json

# Check if framescli is available
result = subprocess.run(
    ["framescli", "doctor", "--json"],
    capture_output=True,
    text=True
)
doctor = json.loads(result.stdout)

if doctor["required_failed"]:
    print("Missing dependencies:", doctor["tools"])
    exit(1)

# Extract frames + transcript
result = subprocess.run([
    "framescli", "extract", "/path/to/video.mp4",
    "--voice",
    "--preset", "balanced",
    "--json"
], capture_output=True, text=True)

extraction = json.loads(result.stdout)

if extraction["status"] == "success":
    # Get artifacts
    result = subprocess.run(
        ["framescli", "artifacts", "latest", "--json"],
        capture_output=True,
        text=True
    )
    artifacts = json.loads(result.stdout)

    # Read transcript
    with open(artifacts["data"]["transcript_json"]) as f:
        transcript = json.load(f)

    # Analyze...
```

---

## Preset Selection Guide for Agents

| Preset | When to Use | FPS | Format | Chunk Duration |
|--------|-------------|-----|--------|----------------|
| `laptop-safe` | Long recordings (>30min), CPU-only, low disk | 1 | jpg | 300s |
| `balanced` | Default choice, good quality/speed tradeoff | 4 | png | 600s |
| `high-fidelity` | Short critical recordings, GPU available | 8 | png | 900s |

**Decision tree:**
```
Is video >30 minutes?
  YES → Use laptop-safe
  NO  → Is GPU available (check doctor.gpu_available)?
          YES → Use balanced or high-fidelity
          NO  → Use laptop-safe
```

---

## Guardrails and Cost Estimation

Before extraction, agents should check `preview` guardrails:

**Warning thresholds** (inform user):
- Estimated frames ≥ 20,000
- Estimated disk ≥ 2 GB
- Duration ≥ 2 hours
- CPU-only transcript on 30+ min video

**Blocking thresholds** (require `--allow-expensive`):
- Estimated frames ≥ 40,000
- Estimated disk ≥ 4 GB
- Duration ≥ 3 hours
- CPU-only transcript on 45+ min video without chunking

**Agent should:**
1. Call `preview` first
2. Check `estimate.guardrails` array
3. If blocking guardrails present:
   - Inform user of cost
   - Suggest lower preset or trimmed duration
   - Only proceed if user confirms with `--allow-expensive`

---

## Artifact Structure for Agents

After successful extraction, read these files:

### `run.json` - Run metadata
```json
{
  "run_id": "Run_20260413-143022-a3f9c2",
  "created_at": "2026-04-13T14:30:22Z",
  "video_path": "/Users/me/Videos/recording.mp4",
  "duration": 125.5,
  "fps": 4,
  "format": "png",
  "preset": "balanced",
  "frame_count": 502
}
```

### `frames.json` - Frame timeline
```json
{
  "frames": [
    {
      "index": 1,
      "timestamp": 0.0,
      "filename": "frame-0001.png",
      "path": "/path/to/run/images/frame-0001.png"
    },
    {
      "index": 2,
      "timestamp": 0.25,
      "filename": "frame-0002.png",
      "path": "/path/to/run/images/frame-0002.png"
    }
  ]
}
```

### `voice/transcript.json` - Full transcript with timing
```json
{
  "text": "Okay so I'm going to reproduce this bug...",
  "segments": [
    {
      "id": 0,
      "text": "Okay so I'm going to reproduce this bug",
      "start": 0.0,
      "end": 3.2,
      "words": [
        {"word": "Okay", "start": 0.0, "end": 0.5},
        {"word": "so", "start": 0.5, "end": 0.7}
      ]
    }
  ]
}
```

**Agent analysis pattern:**
1. Parse transcript segments
2. Map segment timestamps to frame indices
3. Cross-reference spoken content with visual timeline
4. Identify key moments (errors, decisions, actions)

---

## Troubleshooting for Agents

### Issue: MCP tool calls fail with "path not allowed"

**Cause:** Paths not configured via `prefs_set`

**Fix:**
```json
{
  "tool": "prefs_set",
  "arguments": {
    "input_dirs": ["/Users/me/Videos"],
    "output_root": "/Users/me/frames"
  }
}
```

### Issue: Transcription always times out

**Cause:** Whisper on CPU is slow for long videos

**Fix:**
1. Check `doctor` output for `gpu_available: false`
2. Use `--transcribe-timeout 600` (10 min limit)
3. If timeout, call `transcribe_run` later with chunking:
   ```json
   {
     "tool": "transcribe_run",
     "arguments": {
       "run_dir": "/path/to/run",
       "chunk_duration": 300
     }
   }
   ```

### Issue: Extraction estimates show warnings

**Cause:** Video is large/long

**Fix:**
1. Inform user of cost from `preview.estimate`
2. Suggest `--preset laptop-safe` for lower resource use
3. Suggest `--from` and `--to` flags to extract subset
4. Only proceed with user confirmation

### Issue: "ffmpeg not found" error

**Cause:** User hasn't installed dependencies

**Recovery:**
```
User needs to install ffmpeg:
  macOS: brew install ffmpeg
  Linux: sudo apt install ffmpeg
  Or run: curl -fsSL https://raw.githubusercontent.com/wraelen/framescli/main/scripts/install-release.sh | bash
```

---

## Performance Expectations

| Video Length | Preset | Extraction Time | Transcript Time (GPU) | Transcript Time (CPU) |
|--------------|--------|-----------------|----------------------|----------------------|
| 5 min | balanced | ~30s | ~30s | ~5 min |
| 30 min | balanced | ~3 min | ~3 min | ~30 min |
| 2 hour | laptop-safe | ~10 min | ~15 min | ~2+ hours |

**Agent should:**
- Inform user of expected wait time based on `preview` estimates
- Use `--transcribe-timeout` for agent workflows
- Consider suggesting `--no-voice` for analysis that doesn't need audio

---

## Advanced: Resumable Workflows

For interrupted extractions or timeout transcriptions:

**1. Resume transcription:**
```bash
framescli transcribe-run /path/to/run --chunk-duration 600 --json
```

**2. Check manifest:**
```bash
cat /path/to/run/voice/transcription-manifest.json
```

**Manifest structure:**
```json
{
  "chunks": [
    {"index": 0, "start": 0, "end": 600, "status": "completed"},
    {"index": 1, "start": 600, "end": 1200, "status": "in_progress"}
  ]
}
```

**Agent can:**
- Resume failed chunks only
- Report progress to user
- Handle partial results gracefully

---

## JSON Schema Reference

See `docs/schemas/` for complete JSON schemas of:
- MCP tool inputs/outputs
- CLI JSON response envelopes
- Artifact file formats (run.json, frames.json, transcript.json)

---

## Getting Help

- **Documentation:** https://github.com/wraelen/framescli
- **Issues:** https://github.com/wraelen/framescli/issues
- **MCP Troubleshooting:** Check MCP client logs for stdio communication
- **CLI Troubleshooting:** Run `framescli doctor --json` and share output

---

## Summary: What Agents Should Do

### ✅ Best Practices
1. **Always call `doctor` first** to verify installation
2. **Call `prefs_set` before extraction** to configure paths
3. **Use `preview` before expensive operations** to check cost
4. **Respect guardrails** and inform user of warnings/blocks
5. **Use appropriate presets** based on video length and hardware
6. **Handle timeouts gracefully** with `--transcribe-timeout` and `transcribe_run`
7. **Read artifact index** via `get_latest_artifacts` for paths
8. **Parse JSON artifacts** (transcript.json, frames.json) for analysis

### ❌ Common Mistakes
1. Calling `extract` without `prefs_set` (path validation fails)
2. Ignoring `preview` guardrails (wastes user resources)
3. Using `high-fidelity` preset on hour-long CPU-only videos (very slow)
4. Not checking `doctor.required_failed` before extraction
5. Assuming all videos have audio (check `preview`, handle errors)

### 🎯 Typical Agent Workflow
```
doctor → prefs_set → preview → extract → get_latest_artifacts → analyze
```

**Time to first result:** ~2-5 minutes for typical 10-minute recording on modern hardware.
