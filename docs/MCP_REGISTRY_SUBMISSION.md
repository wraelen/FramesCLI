# MCP Registry Submission Materials

This document contains all materials needed to submit FramesCLI to the [MCP servers registry](https://github.com/modelcontextprotocol/servers).

## Submission Checklist

- [x] Logo/icon (512x512 PNG with transparent background)
- [x] Short description (1-2 sentences)
- [x] Categories selected
- [x] MCP server configuration example
- [x] Tool list documentation
- [x] GitHub repository is public

## Logo

**File:** `brand/exports/mcp-icon-1024.png` (1024x1024, 20KB)

This is the clapperboard logo - perfect for MCP registry as it's:
- ✅ Square format (1024x1024)
- ✅ Clean icon (no wordmark)
- ✅ Recognizable at small sizes
- ✅ Transparent background

**Source:** `brand/src/FramesCLI_logo_square.png`

**Note:** If the registry requires exactly 512x512, you can resize:
```bash
# Using Python PIL (if available)
python3 -c "from PIL import Image; img = Image.open('brand/exports/mcp-icon-1024.png'); img.resize((512, 512), Image.LANCZOS).save('brand/exports/mcp-icon-512.png')"

# Or manually in any image editor (GIMP, Preview on macOS, etc.)
```

## Short Description

**Primary (recommended - broader appeal):**
> Make videos AI-readable. Turn any video into timestamped frames and transcripts that agents can analyze, search, and reference.

**Alternative (emphasizes common use cases):**
> Turn screen recordings into timestamped frames and transcripts for debugging, documentation, tutorials, or any video content your agents need to understand.

**Alternative (technical):**
> Local-first video processing for AI agents: extract frames, generate transcripts, and produce structured JSON outputs from any video file.

## Categories

Select these categories for the MCP registry:

- ✅ **Media & Content** - Primary category (universal video/audio processing for agents)
- ✅ **Productivity** - Secondary category (agent workflow enhancement, documentation, knowledge extraction)
- ✅ **Development Tools** - Tertiary category (includes debugging, incident review use cases)

**Note:** Prioritize Media & Content as primary since FramesCLI handles ANY video content, not just development/debugging videos.

## MCP Server Configuration

### Minimal Config

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

### With Custom Paths

```json
{
  "mcpServers": {
    "framescli": {
      "command": "framescli",
      "args": ["mcp"],
      "env": {
        "FRAMES_CONFIG": "/path/to/custom/config.json"
      }
    }
  }
}
```

## Tool List

FramesCLI provides 10 MCP tools:

| Tool | Description |
|------|-------------|
| `doctor` | Check local toolchain readiness (ffmpeg, whisper) |
| `preview` | Estimate extraction cost before running (frames, disk, time) |
| `extract` | Extract frames and optional transcript from single video |
| `extract_batch` | Process multiple videos/globs |
| `transcribe_run` | Resume/add transcription to existing extraction run |
| `open_last` | Get path to specific artifact from latest run |
| `get_latest_artifacts` | Get all artifact paths from most recent run |
| `get_run_artifacts` | Query specific run or list recent N runs |
| `prefs_get` | Get agent path configuration (input dirs, output root) |
| `prefs_set` | Set agent path configuration |

## Features Highlight

- **Path safety:** MCP access restricted to configured allowed roots
- **Long-running ops:** Heartbeat notifications every 10s for extract/transcribe
- **Stable errors:** Structured error responses with recovery guidance
- **Cost estimation:** Preview command shows frame count, disk usage before extraction
- **Resumable workflows:** Chunked transcription with manifest-based resume

## Installation Requirements

### Required
- `ffmpeg` (video processing)
- `ffprobe` (video metadata)

### Optional
- `whisper` or `faster-whisper` (transcription only)

### Install Command

```bash
# One-command installer
curl -fsSL https://raw.githubusercontent.com/wraelen/framescli/main/scripts/install-release.sh | bash

# Or via Homebrew (once tap is set up)
brew tap wraelen/framescli
brew install framescli
```

## Quick Start Workflow

Standard agent workflow:

1. Call `doctor` - verify tools installed
2. Call `prefs_set` - configure input/output paths
3. Call `preview` - estimate cost for target video
4. Call `extract` - process video (with `voice: true` for transcript)
5. Call `get_latest_artifacts` - retrieve result paths

## Documentation Links

- **Full integration guide:** https://github.com/wraelen/framescli/blob/main/docs/AGENT_INTEGRATION.md
- **Agent recipes:** https://github.com/wraelen/framescli/blob/main/docs/AGENT_RECIPES.md
- **JSON schemas:** https://github.com/wraelen/framescli/tree/main/docs/schemas
- **Main README:** https://github.com/wraelen/framescli

## Registry Submission Template

Use this template when submitting to the MCP registry:

```yaml
name: framescli
description: Make videos AI-readable. Turn any video into timestamped frames and transcripts that agents can analyze, search, and reference.
homepage: https://github.com/wraelen/framescli
repository: https://github.com/wraelen/framescli
license: MIT
categories:
  - media-content
  - productivity
  - development-tools
logo: brand/exports/logo-icon-color.png
installation:
  command: framescli
  args: ["mcp"]
  requirements:
    - ffmpeg (required)
    - ffprobe (required)
    - whisper or faster-whisper (optional, for transcription)
  install_url: https://github.com/wraelen/framescli#install
documentation: https://github.com/wraelen/framescli/blob/main/docs/AGENT_INTEGRATION.md
tools:
  - doctor
  - preview
  - extract
  - extract_batch
  - transcribe_run
  - open_last
  - get_latest_artifacts
  - get_run_artifacts
  - prefs_get
  - prefs_set
```

## Submission Process

**IMPORTANT:** Submit to the **registry** repo (NOT the servers repo)

1. **Fork the MCP registry:**
   ```bash
   gh repo fork modelcontextprotocol/registry
   ```

   **Correct repo:** https://github.com/modelcontextprotocol/registry
   **NOT:** ~~https://github.com/modelcontextprotocol/servers~~ (they no longer accept server PRs)

2. **Add FramesCLI entry** to the appropriate category file

3. **Include logo** (may need to be committed to the registry repo or linked from this repo)

4. **Create PR** with:
   - Clear title: "Add FramesCLI - Make videos AI-readable"
   - Description highlighting use cases (tutorials, documentation, debugging, knowledge extraction, any video content)
   - Link to documentation

5. **Respond to review feedback** and iterate

## Maintainer Notes

- MCP registry may have specific schema requirements - check their CONTRIBUTING guide
- Logo might need to be exactly 512x512 PNG
- Description has character limits - keep it concise
- May need to provide example usage or demo GIF

## Questions for User

Before submitting, please confirm:

1. ✅ Should I use your GitHub username (`wraelen`) as the maintainer?
2. ✅ Is the logo `brand/exports/logo-icon-color.png` the correct one to use?
3. ✅ Do you want to submit immediately or after more testing?
4. ✅ Any specific categories/tags you want emphasized?
