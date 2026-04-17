# FramesCLI

<p align="center">
  <img src="https://raw.githubusercontent.com/wraelen/framescli/main/brand/exports/logo-main.svg" alt="FramesCLI" width="600" style="margin: 20px 0;">
</p>

**Let AI agents "watch" videos.** Extract frames and transcripts from any video so Claude, GPT, or other AI can analyze the visual and audio content.

[![Go Version](https://img.shields.io/github/go-mod/go-version/wraelen/framescli?style=flat-square)](https://github.com/wraelen/framescli/blob/main/go.mod)
[![Build](https://img.shields.io/github/actions/workflow/status/wraelen/framescli/ci.yml?branch=main&style=flat-square)](https://github.com/wraelen/framescli/actions)
[![License](https://img.shields.io/github/license/wraelen/framescli?style=flat-square)](./LICENSE)
[![Release](https://img.shields.io/github/v/release/wraelen/framescli?style=flat-square)](https://github.com/wraelen/framescli/releases)
[![MCP Registry](https://img.shields.io/badge/MCP-Registry-blue?style=flat-square)](https://registry.modelcontextprotocol.io/v0.1/servers?search=io.github.wraelen/framescli)

---

## Install → Extract → Ask Claude

**Install (30 seconds):**
```bash
brew install wraelen/tap/framescli
framescli doctor
```

**Extract a video (60 seconds):**
```bash
framescli extract video.mp4 --fps 4 --voice
```

**Ask your AI to watch it:**
> "Read the extracted frames and transcript, then summarize this video"

<p align="center">
  <img src="docs/assets/hero-demo.gif" alt="FramesCLI Demo: Install, extract, and analyze with Claude" width="800">
  <br>
  <em>From install to AI analysis in 90 seconds</em>
</p>

---

## Why This Exists

**Problem:** AI can't watch videos. You can't paste a video into Claude or GPT.

**Solution:** FramesCLI extracts the visual timeline (frames at 1fps, 4fps, or 8fps) and audio content (full transcript with timestamps), creating structured artifacts your AI can read.

**Result:** Your AI agent reads the frames + transcript and can "watch" the video.

### Real Use Cases

- **Debug screen recordings:** "What error appeared at 2:30?" → AI sees frames + transcript
- **Summarize tutorials:** "What are the key steps?" → AI follows visual timeline + narration
- **Meeting notes:** "What decisions were made?" → AI reads transcript + sees slides
- **Content analysis:** "Summarize this lecture" → AI processes slides + spoken content
- **YouTube research:** Extract from any YouTube video for AI analysis

---

## What Makes This Different

✅ **GPU auto-detection** — 15-30x faster on NVIDIA/AMD/Intel/Apple (zero config)
✅ **1000+ sites** — YouTube, Vimeo, Twitter, Reddit via yt-dlp
✅ **Local-first** — All processing on your machine, zero cloud, zero telemetry
✅ **MCP integration** — Works with Claude Desktop, Cursor, Cline, Windsurf
✅ **Single binary** — No Python venvs, no Docker, no node_modules

---

## Installation

### Homebrew (recommended)
```bash
brew install wraelen/tap/framescli
```

### Go
```bash
go install github.com/wraelen/framescli/cmd/frames@latest
```

### MCP Registry
Search for "framescli" in [Claude Desktop](https://claude.ai/download), Cursor, Cline, or Windsurf for one-click install.

### Verify
```bash
framescli doctor
```

Shows detected tools (ffmpeg, yt-dlp, whisper) and GPU hardware.

### Optional: Transcription Setup

Frame extraction works immediately. For `--voice` transcription:

```bash
brew install yt-dlp           # For URL extraction
pip install openai-whisper    # For transcription
```

**Alternative:** `pip install faster-whisper` for better GPU support.

FramesCLI auto-detects the best available backend.

---

## Quick Start

### Extract from local video
```bash
framescli extract video.mp4 --fps 4 --preset balanced
```

### Extract from YouTube
```bash
framescli extract --url "https://youtube.com/watch?v=..." --fps 4 --voice
```

### Preview before extracting
```bash
framescli preview video.mp4 --preset balanced
# Shows: frame count, disk usage, transcript time
```

### Use with Claude Desktop (MCP)

Add to `~/Library/Application Support/Claude/claude_desktop_config.json`:

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

Then ask Claude:
> "Use framescli to watch video.mp4 and summarize it"

Claude automatically extracts frames, transcribes audio, and analyzes the content.

---

## Output Structure

```
frames/Run_20260415-083045/
  images/
    frame-0001.png
    frame-0002.png
    ...
  voice/
    transcript.txt        # Plain text
    transcript.json       # Timestamped segments
    transcript.srt        # SubRip format
    voice.wav             # Extracted audio
  run.json                # Metadata: fps, duration, preset
  frames.json             # Per-frame timing and paths
```

All artifacts are JSON-parseable for automation. See [schemas](docs/schemas/) for details.

---

## Workflow Presets

| Preset | FPS | Format | Use Case |
|--------|-----|--------|----------|
| **laptop-safe** | 1 | jpg | Low resource usage, fast |
| **balanced** | 4 | png | Recommended default |
| **high-fidelity** | 8 | png | Maximum detail (GPU recommended) |

```bash
framescli extract video.mp4 --preset balanced --voice
```

Override any preset:
```bash
framescli extract video.mp4 --preset balanced --fps 8
```

---

## GPU Acceleration

FramesCLI auto-detects and uses available GPUs:

| GPU Type | Speedup | Support |
|----------|---------|---------|
| **NVIDIA (CUDA)** | 15-30x | ✅ Auto-detected |
| **Apple Silicon** | 10-20x | ✅ Auto-detected |
| **AMD (VAAPI)** | 10-30x | ✅ Auto-detected |
| **Intel QuickSync** | 5-15x | ✅ Auto-detected |

Check status:
```bash
framescli doctor
```

Example output:
```
Hardware
GPU:               NVIDIA GeForce RTX 3070 Ti (nvidia)
Recommended:       hwaccel=cuda

Transcription
Backend:           faster-whisper (GPU-accelerated)
Estimated Speed:   ~10x realtime
```

CPU fallback is automatic if GPU fails.

---

## Common Commands

```bash
# Core extraction
framescli extract <video|recent> [--fps 4] [--voice] [--preset balanced]
framescli extract --url <url> [--fps 4] [--voice]
framescli extract-batch "recordings/*.mp4" [--fps 1] [--voice]

# Utilities
framescli preview <video> [--preset balanced]
framescli doctor [--json]
framescli artifacts [latest|run-name]
framescli open-last [--artifact transcript]

# MCP server
framescli mcp
```

Full reference: [Command Reference](docs/COMMAND_REFERENCE.md)

---

## Documentation

**Getting Started:**
- [Installation Guide](docs/INSTALL.md) - Platform-specific instructions
- [Quickstart](docs/QUICKSTART.md) - 5-minute walkthrough
- [Performance Guide](docs/PERFORMANCE.md) - GPU setup and optimization

**Integration:**
- [Agent Integration](docs/AGENT_INTEGRATION.md) - MCP tools, JSON schemas
- [Agent Recipes](docs/AGENT_RECIPES.md) - Copy-paste workflows

**Development:**
- [Contributing](CONTRIBUTING.md) - How to contribute
- [Architecture](CLAUDE.md) - Build, test, internals
- [Roadmap](docs/NEXT_PHASE_ROADMAP.md) - Planned features

---

## FAQ

**Q: Do I need a GPU?**
No. GPU is optional but provides 15-30x speedup.

**Q: What video formats are supported?**
Any format ffmpeg can read: MP4, MOV, AVI, MKV, WebM, etc.

**Q: Does this send data to the cloud?**
No. 100% local processing. Zero telemetry.

**Q: Can I extract from YouTube?**
Yes. Install `yt-dlp` and use `--url`. Supports 1000+ sites.

**Q: How much disk space do I need?**
Use `framescli preview` for estimates. Typical: 5-min 1080p @ 4fps = ~200-500MB.

---

## Troubleshooting

**"ffmpeg not found"**
`brew install ffmpeg` (macOS) or `sudo apt install ffmpeg` (Ubuntu/WSL)

**"yt-dlp not found"**
`brew install yt-dlp` or `pip install yt-dlp`

**"whisper not found"**
`pip install openai-whisper` or `pip install faster-whisper`

**GPU not detected**
Run `framescli doctor` for diagnostics. Check drivers: `nvidia-smi` or `vainfo`.

**Extraction is slow**
- Check GPU: `framescli doctor`
- Use `--preset laptop-safe` or `--fps 1`

**Full diagnostics:**
```bash
framescli doctor --report
```

---

## Performance Benchmarks

5-minute 1080p video @ 1fps:

| Hardware | Time | Speedup |
|----------|------|---------|
| **RTX 3070 Ti (CUDA)** | ~4 sec | 25x |
| **M1 Pro (VideoToolbox)** | ~6 sec | 15x |
| **Intel i7 QuickSync** | ~12 sec | 8x |
| **CPU-only (Ryzen 5600X)** | ~2 min | 1x |

Run your own:
```bash
framescli benchmark video.mp4 --duration 20
```

---

## Contributing

Contributions welcome! See [CONTRIBUTING.md](CONTRIBUTING.md).

```bash
git clone https://github.com/wraelen/framescli
cd framescli
make build && make test
```

---

## License

MIT License - see [LICENSE](LICENSE)

---

## Built With

- [FFmpeg](https://ffmpeg.org/) - Video processing
- [yt-dlp](https://github.com/yt-dlp/yt-dlp) - Video downloading
- [OpenAI Whisper](https://github.com/openai/whisper) - Transcription
- [Cobra](https://github.com/spf13/cobra) - CLI framework

---

**Ready to let your AI watch videos?**

```bash
brew install wraelen/tap/framescli
framescli extract video.mp4 --fps 4 --voice
```

⭐ Star this repo if you find it useful · [Report issues](https://github.com/wraelen/framescli/issues)
