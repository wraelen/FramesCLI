# FramesCLI

<p align="center">
  <img src="https://raw.githubusercontent.com/wraelen/framescli/main/brand/exports/logo-main.svg" alt="FramesCLI" width="600" style="margin: 20px 0;">
</p>

**FramesCLI lets AI agents "watch" videos.** Extract frames and transcripts from any video so Claude, Codex, or other AI coding assistants can analyze the visual and audio content.

> [![Go Version](https://img.shields.io/github/go-mod/go-version/wraelen/framescli?style=flat-square)](https://github.com/wraelen/framescli/blob/main/go.mod)
> [![Build](https://img.shields.io/github/actions/workflow/status/wraelen/framescli/ci.yml?branch=main&style=flat-square)](https://github.com/wraelen/framescli/actions)
> [![License](https://img.shields.io/github/license/wraelen/framescli?style=flat-square)](./LICENSE)
> [![Release](https://img.shields.io/github/v/release/wraelen/framescli?style=flat-square)](https://github.com/wraelen/framescli/releases)
> [![MCP Registry](https://img.shields.io/badge/MCP-Registry-blue?style=flat-square)](https://registry.modelcontextprotocol.io/v0.1/servers?search=io.github.wraelen/framescli)

> **Now available in the [official MCP Registry](https://registry.modelcontextprotocol.io/v0.1/servers?search=io.github.wraelen/framescli)!** Install FramesCLI in any MCP-compatible client (Claude Desktop, Cursor, Cline, Windsurf, etc.).

## What Your AI Can Do

Once FramesCLI extracts frames and transcripts, your AI agent can:

- **Analyze screen recordings**: "What error appeared at 2:30?" → Agent sees the frames and reads the transcript
- **Understand tutorials**: "Summarize the steps in this demo" → Agent follows the visual timeline
- **Review meetings**: "What decisions were made?" → Agent reads transcript + sees presentation slides in frames
- **Debug issues**: "When did the app crash?" → Agent correlates visual state with spoken context
- **Extract insights**: "What was covered in this lecture?" → Agent processes both visual and audio content

## How It Works

1. **You provide a video** (screen recording, meeting, tutorial, anything)
2. **FramesCLI extracts:**
   - Visual timeline: frames at configurable intervals (1fps, 4fps, 8fps)
   - Audio content: full transcript with word-level timestamps
   - Structured metadata: JSON with frame paths, timings, and transcript segments
3. **Your AI agent reads the artifacts** and can "watch" the video through the extracted frames + transcript
4. **Local-first**: All processing happens on your machine using ffmpeg and whisper (no cloud required)

## For AI Coding Assistants

**FramesCLI makes any video AI-readable.** If you're an AI coding assistant, use FramesCLI to turn videos into structured, searchable artifacts your users can reference and analyze:

### Quick Setup (30 seconds)

1. **Verify installation:**
   ```bash
   framescli doctor --json
   ```

2. **Configure MCP** (recommended) in your client config:
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

3. **First extraction workflow:**
   ```
   1. Call: doctor (verify tools)
   2. Call: prefs_set (configure paths)
   3. Call: preview (estimate cost)
   4. Call: extract (process video)
   5. Call: get_latest_artifacts (retrieve results)
   ```

### Why Agents Should Use This

- **Structured outputs:** All commands support `--json` with stable schema
- **MCP integration:** 10 tools for video analysis via Model Context Protocol
- **Cost estimation:** `preview` command provides frame count, disk usage, transcript time before extraction
- **Resumable workflows:** Chunked transcription with manifest-based resume
- **Path safety:** MCP enforces allowed input/output roots

### Agent-Specific Documentation

- **Complete integration guide:** [docs/AGENT_INTEGRATION.md](docs/AGENT_INTEGRATION.md)
- **Copy-paste workflows:** [docs/AGENT_RECIPES.md](docs/AGENT_RECIPES.md)
- **JSON schemas:** [docs/schemas/](docs/schemas/)
- **Agent skill file:** [SKILL.md](SKILL.md)

### Common Use Cases for Agents

| Use Case | User Request | Agent Action |
|----------|--------------|--------------|
| **Tutorial analysis** | "Summarize this coding tutorial" | Extract frames + transcript → analyze visual steps + spoken explanations |
| **Meeting review** | "What was decided in this meeting?" | Extract frames + transcript → identify key decisions from slides + discussion |
| **Debug session** | "When did the error occur?" | Extract frames + transcript → find error timestamp in visual + audio |
| **Content understanding** | "What's covered in this lecture?" | Extract frames + transcript → process visual slides + spoken content |
| **Screen recording analysis** | "Walk me through what happened" | Extract frames → provide visual timeline with transcript context |
| **Batch analysis** | "Analyze all my recordings" | Extract multiple videos → index and search across visual + audio content |

**Installation:** See [Install](#install) section below for ffmpeg/whisper dependencies.

**Need help?** Read the [agent integration guide](docs/AGENT_INTEGRATION.md) or check [issues](https://github.com/wraelen/framescli/issues).

---

## Core Capabilities

- Video input handling and validation (duration, resolution, FPS)
- Frame extraction with timestamp/frame range controls
- Audio extraction with format + trim + normalize options
- Local transcription with selectable backend (`auto|whisper|faster-whisper`) and outputs (`txt`, `json`, `srt`, `vtt`)
- Batch processing across multiple files/globs
- Machine-readable `--json` outputs for automation
- MCP server mode (`framescli mcp`) for IDE/agent integration
- Diagnostics bundles for failed runs

## Install

### Requirements

- `ffmpeg`
- `ffprobe`
- `whisper` or `faster-whisper` (only required for transcription features)

### Install FramesCLI

Recommended for most users: run the one-command bootstrap installer. It installs the latest release binary, can help install `ffmpeg`/`ffprobe`, and can launch `framescli setup` for first-run preferences.

The release installer now verifies the downloaded archive against the published `checksums.txt` before extraction.

```bash
curl -fsSL https://raw.githubusercontent.com/wraelen/framescli/main/scripts/install-release.sh | bash
```

Install a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/wraelen/framescli/main/scripts/install-release.sh | \
  bash -s -- --version v0.1.0
```

Non-interactive release install:

```bash
curl -fsSL https://raw.githubusercontent.com/wraelen/framescli/main/scripts/install-release.sh | \
  bash -s -- --yes
```

Install from source instead:

```bash
go install github.com/wraelen/framescli/cmd/frames@latest
framescli --help
```

Build locally from the checked-out repo:

```bash
go mod tidy
go build -o bin/framescli ./cmd/frames
./bin/framescli --help
```

Notes:

- The release installer places `framescli` into `~/.local/bin` by default.
- After binary install, the bootstrap flow can run `doctor` and launch `framescli setup`.
- **Try via MCP Registry**: FramesCLI is listed in the [official MCP Registry](https://registry.modelcontextprotocol.io/v0.1/servers?search=io.github.wraelen/framescli). Search for "framescli" in any MCP-compatible client (Claude Desktop, Cursor, Cline, Windsurf) for one-click installation.
- Package-manager distribution (`apt`, Homebrew, winget, etc.) is planned for future releases (see [roadmap](docs/NEXT_PHASE_ROADMAP.md)).
- The local repo build helper remains available at `./scripts/install.sh`.

### Dependency Install

Recommended (repo script):

```bash
# Install required media deps (ffmpeg/ffprobe)
./scripts/install-deps.sh --install

# Include whisper as well
./scripts/install-deps.sh --install --with-whisper
```

Make targets:

```bash
make deps
make deps-whisper
```

Manual:

```bash
# macOS (Homebrew)
brew install ffmpeg

# Ubuntu/Debian/WSL
sudo apt install ffmpeg

# Fedora
sudo dnf install ffmpeg
```

### Whisper Install (Optional, for Transcription)

```bash
# macOS / Linux / WSL (recommended via pipx)
python3 -m pip install --user pipx
python3 -m pipx ensurepath
pipx install openai-whisper

# Alternate (venv/global pip)
pip install -U openai-whisper
```

Notes:

- A transcription backend is only required for `--voice`/`transcribe` workflows.
- Backend selection supports `auto|whisper|faster-whisper` (`auto` prefers `faster-whisper` when available).
- Verify install with `<backend-binary> --help`.
- Override backend per command:
  - `--transcribe-backend auto|whisper|faster-whisper`
  - `--transcribe-bin <path-or-name>`
  - `--transcribe-language <lang>`

### Quick Verification

```bash
framescli doctor
framescli preview recent --json
```

### Public Smoke Test

Run this before opening issues or publishing a release candidate:

```bash
# Uses a generated sample video
./scripts/public-smoke.sh

# Or test against your own recording
./scripts/public-smoke.sh --video /absolute/path/to/recording.mp4
```

Outputs are written to `tmp/public-smoke/` (doctor, preview, extract, batch, open-last, MCP smoke).

### Release Verification

For maintainers, validate release artifacts separately from source-level tests:

```bash
# After goreleaser snapshot output exists in ./dist
make release-verify

# After publishing a real GitHub release
./scripts/release-verify.sh --source github --version v0.1.0
```

This verifies published checksums, expected archive contents, installer asset resolution, and a runtime smoke check for the current platform binary.

## 60-Second Quickstart: Let Your AI Watch a Video

```bash
# 1) Install and verify
framescli doctor

# 2) Extract frames + transcript from a video
framescli extract /path/to/recording.mp4 --voice --preset balanced

# 3) Now ask your AI: "Read the extracted frames and transcript, then tell me what happens in this video"
# Your AI can now see the visual timeline (frames) and hear the audio (transcript)
```

**With MCP integration:** Just tell Claude *"Use framescli to watch video.mp4 and summarize it"* — framescli handles the extraction automatically.

## URL Extraction

FramesCLI can download and extract frames/transcripts from any video URL supported by `yt-dlp` (1000+ sites including YouTube, Vimeo, Twitter, Reddit, and more).

### Requirements

Install `yt-dlp` for URL support:

```bash
# Direct binary install (recommended)
curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp -o ~/.local/bin/yt-dlp
chmod +x ~/.local/bin/yt-dlp

# Or via pip/pipx
pip install yt-dlp
# or
pipx install yt-dlp
```

Verify with `framescli doctor` — it will show `[ok] yt-dlp` if found.

### Extract from URLs

```bash
# YouTube video
framescli extract --url "https://youtube.com/watch?v=..." --preset balanced --voice

# Direct video link
framescli extract --url "https://example.com/video.mp4" --fps 4

# Any yt-dlp supported site
framescli extract --url "https://vimeo.com/..." --voice --format png
```

### How It Works

1. **URL provided via `--url` flag** (mutually exclusive with video path argument)
2. **FramesCLI downloads video** using yt-dlp to `~/.cache/framescli/videos/<hash>.mp4`
3. **Cache enabled by default** — re-running the same URL skips download
4. **Extraction proceeds normally** with the cached local file
5. **Use `--no-cache`** to force re-download

### Cache Management

Videos are cached at `~/.cache/framescli/videos/` using SHA256 hash of the URL:

```bash
# Re-download even if cached
framescli extract --url "..." --no-cache

# Manual cache cleanup (planned for v0.2.1)
# For now: rm -rf ~/.cache/framescli/videos/
```

### Error Handling

FramesCLI provides clear error messages for common failures:

- **Geo-blocked:** "Video not available in your region (use VPN or proxy)"
- **Deleted/private:** "Video unavailable (deleted or private)"
- **Auth required:** "Video requires authentication (use yt-dlp cookies)"
- **Network error:** "Download failed (check connection, retry with --no-cache)"

### Notes

- URL and path arguments are mutually exclusive
- Downloaded videos use the same extraction pipeline as local files
- Cache hits skip the download phase entirely
- Source attribution is tracked in run metadata

## Performance & Hardware Optimization

FramesCLI automatically detects available GPU hardware and enables hardware acceleration by default. **No manual configuration required** — if you have a GPU, it's already being used.

### Auto-Detection

On startup, FramesCLI detects:

- **NVIDIA GPUs** (via CUDA) — 15-30x faster frame extraction
- **AMD GPUs** (via VAAPI/ROCm) — 10-30x faster frame extraction
- **Intel QuickSync** (via QSV) — 5-15x faster frame extraction
- **Apple Silicon** (via VideoToolbox) — 10-20x faster frame extraction

GPU acceleration is enabled automatically when available. CPU-only mode is used as a fallback.

### Check Your Hardware

Run `framescli doctor` to see detected hardware and recommendations:

```bash
$ framescli doctor

Tools
[ok]   ffmpeg
[ok]   yt-dlp

Hardware
GPU:               NVIDIA GeForce RTX 3070 Ti (nvidia)
Recommended:       hwaccel=cuda

Transcription
Backend:           faster-whisper (GPU-accelerated)
Model:             base
Estimated Speed:   ~10x realtime

Recommendations
→ Your GPU is already enabled by default (hwaccel=cuda)
→ Consider upgrading to medium whisper model for better accuracy
```

### Manual Override

If you need to disable GPU acceleration or force a specific mode:

```bash
# Disable GPU (CPU-only)
framescli extract video.mp4 --hwaccel none

# Force specific GPU mode
framescli extract video.mp4 --hwaccel cuda

# Let ffmpeg auto-detect
framescli extract video.mp4 --hwaccel auto

# Set persistent preference
framescli prefs set hwaccel none
```

### Supported Hardware

| Vendor | HWAccel Mode | Typical Speedup | Notes |
|--------|--------------|-----------------|-------|
| **NVIDIA** | `cuda` | 15-30x | Recommended for RTX/GTX cards |
| **AMD** | `vaapi` | 10-30x | Linux VAAPI or ROCm |
| **Intel** | `qsv` | 5-15x | QuickSync Video on modern CPUs |
| **Apple** | `videotoolbox` | 10-20x | M1/M2/M3 chips |
| **CPU-only** | `none` | 1x baseline | Automatic fallback |

### Performance Benchmarks

Typical extraction times for a 5-minute 1080p video at 1fps:

| Hardware | Time | Speedup |
|----------|------|---------|
| **RTX 3070 Ti (CUDA)** | ~4 seconds | 25x |
| **M1 Pro (VideoToolbox)** | ~6 seconds | 15x |
| **Intel i7 QuickSync (QSV)** | ~12 seconds | 8x |
| **CPU-only (Ryzen 5600X)** | ~2 minutes | 1x |

*Actual performance varies by video codec, resolution, and system load.*

### Troubleshooting

**GPU not detected?**

```bash
# Check if NVIDIA drivers are loaded
nvidia-smi

# Check if AMD GPU is available
rocm-smi
# or for VAAPI:
vainfo

# Check Intel QuickSync
vainfo | grep -i driver
```

**GPU acceleration fails during extraction?**

FramesCLI automatically falls back to CPU-only mode and shows a warning:

```
⚠️  Note: GPU acceleration (cuda) failed, fell back to CPU
   Run 'framescli doctor' to verify GPU setup
```

If fallback happens, verify:
- GPU drivers are up to date
- ffmpeg was built with GPU support (`ffmpeg -hwaccels`)
- No other processes are saturating the GPU

### Workflow Presets

Presets apply coordinated defaults for optimal performance:

- **laptop-safe**: `1fps`, `jpg`, safe media tuning, 300s transcript chunks
- **balanced**: `4fps`, `png`, balanced tuning, 600s chunks (recommended)
- **high-fidelity**: `8fps`, `png`, fast tuning, 900s chunks (requires GPU)

```bash
# Recommended for most users
framescli extract video.mp4 --preset balanced --voice

# Maximum quality (GPU recommended)
framescli extract video.mp4 --preset high-fidelity --voice
```

Explicit flags override preset defaults. GPU acceleration is enabled automatically regardless of preset.

## Command Overview

```bash
framescli extract [<videoPath|recent>] [fps] [--url <url>] [--preset balanced] [--voice] [--format png|jpg] [--quality 1-31] [--no-cache]
framescli extract-batch <videoPathOrGlob...> [--preset balanced] [--fps auto] [--voice] [--from 00:30 --to 01:45]
framescli preview <videoPath|recent> [--preset balanced] [--fps auto --format png --mode both]
framescli artifacts [run|latest] [--recent 5] [--json]
framescli open-last [--artifact run|transcript|transcript-json|transcript-srt|transcript-vtt|sheet|log|metadata|frames|manifest|metadata-csv|frames-zip|audio]
framescli copy-last [--artifact run|transcript|transcript-json|transcript-srt|transcript-vtt|sheet|log|metadata|frames|manifest|metadata-csv|frames-zip|audio]
framescli sheet <framesDir> [--cols 6] [--out contact-sheet.png]
framescli transcribe <audioPath> [outDir] [--chunk-duration 600]
framescli transcribe-run <runDir> [--chunk-duration 600] [--timeout 300] [--json]
framescli clean [targetDir]
framescli doctor [--json] [--report] [--report-out path]
framescli index [rootDir] [--out index.json]
framescli benchmark <videoPath|recent> [--duration 20]
framescli benchmark history [--limit 20]
framescli telemetry status [--json]
framescli telemetry tail [-n 20]
framescli telemetry prune [--keep 2000]
framescli setup
framescli config
framescli mcp
framescli completion <bash|zsh|fish|powershell>
```

Primary command name is `framescli`.

## Artifact Index

FramesCLI persists a local run-artifact index at `<frames_root>/index.json`. It is refreshed automatically after successful `extract` and `transcribe-run` workflows, and you can rebuild it explicitly with:

```bash
framescli index [rootDir]
```

The index stays inspectable JSON and records only retrieval-oriented fields for each run, including:

- run directory, created/updated time, and source video path
- fps, frame format, preset, duration, and derived run status
- key artifact paths such as `run.json`, `frames.json`, transcript outputs, audio, contact sheet, manifest, log, CSV, and zip outputs when present
- chunked-transcription progress metadata and simple warning flags for partial runs

Use it from the CLI with:

```bash
framescli artifacts latest
framescli artifacts Run_20260102-150405 --json
framescli artifacts --recent 5
framescli open-last --artifact transcript-json
framescli copy-last --artifact manifest
```

Current limitation: if run directories are edited manually outside FramesCLI, the index will not update until the next successful run completion or an explicit `framescli index` rebuild.

## Common Workflows

### Preview Workload Cost

Use `preview` before expensive runs to inspect approximate frame volume, disk footprint, transcript cost, preset defaults, and risk hints:

```bash
framescli preview /path/to/video.mp4 --preset laptop-safe --mode both
```

Example human-readable output:

```text
Preview
-------
Video:       /path/to/video.mp4
Duration:    1800.00s
Resolution:  1920x1080
Source FPS:  29.97
Mode:        both
Preset:      laptop-safe (media=safe)
Target FPS:  1.00
Format:      jpg
Chunking:    300s
Frames est:  1800
Disk est:    ~103-230 MB for extracted frames + sheet
Artifacts:
- images/frame-XXXX.jpg
- images/sheets/contact-sheet.png
- run.json + frames.json
- voice/voice.wav
- voice/transcript.{txt,json,srt,vtt}
Common disk profiles:
* jpg          1.00fps ~103-230 MB selected (1800 frames)
- png          1.00fps ~206-459 MB png @ 1fps (1800 frames)
Transcript:
- backend=faster-whisper model=base hardware=gpu-capable
- class=fast
- hint=GPU-backed transcription should stay well below video runtime in common cases
- runtime=~2.1-3.8 minutes of transcript time for this clip
Guardrails:
- [warn] Recording duration exceeds 2 hours; preview estimates should be reviewed before extraction. (duration_minutes=180.0, threshold >= 120)
```

JSON output exposes the same estimates for agents and scripts:

```bash
framescli preview /path/to/video.mp4 --preset laptop-safe --mode both --json
```

Example JSON excerpt:

```json
{
  "command": "preview",
  "status": "success",
  "data": {
    "preset": "laptop-safe",
    "media_preset": "safe",
    "target_fps": 1,
    "format": "jpg",
    "chunk_duration_sec": 300,
    "estimate": {
      "frame_count": 1800,
      "estimated_mb": 166.3,
      "estimated_mb_low": 102.9,
      "estimated_mb_high": 229.6,
      "disk_summary": "~103-230 MB for extracted frames + sheet",
      "disk_profiles": [
        {
          "label": "selected",
          "format": "jpg",
          "fps": 1,
          "frame_count": 1800,
          "disk_summary": "~103-230 MB",
          "selected": true
        }
      ],
      "transcript": {
        "enabled": true,
        "backend": "faster-whisper",
        "runtime_class": "fast",
        "cost_hint": "GPU-backed transcription should stay well below video runtime in common cases",
        "chunk_duration_sec": 300
      },
      "guardrails": {
        "guardrails": [
          {
            "severity": "warn",
            "metric": "duration_minutes",
            "actual": "180.0",
            "threshold": ">= 120"
          }
        ]
      }
    }
  }
}
```

### Extract Frames at Intervals

```bash
framescli extract /path/to/video.mp4 --fps 2 --format png
framescli extract /path/to/video.mp4 --every-n 10 --name-template "frame-%05d"
```

### Workflow Presets

FramesCLI now exposes explicit workflow presets:

- `laptop-safe`: `1fps`, `jpg`, ffmpeg media preset `safe`, transcript chunking `300s`
- `balanced`: `4fps`, `png`, ffmpeg media preset `balanced`, transcript chunking `600s`
- `high-fidelity`: `8fps`, `png`, ffmpeg media preset `fast`, transcript chunking `900s`

Explicit flags still win. For example, `--preset laptop-safe --fps 3 --format png --chunk-duration 1200` keeps the preset's media-tuning choice while honoring the user-provided sampling, format, and chunk size.

Configured defaults are applied coherently:

- if `performance-mode` is set to one of the workflow presets and `default-fps` / `default-format` are still at the stock defaults, the preset sampling and format apply implicitly
- if you set custom `default-fps` or `default-format` in config, those remain the default for omitted flags and only the preset's media-tuning and transcript chunking are applied implicitly

Legacy preset names remain accepted for compatibility:

- `safe` maps to `laptop-safe`
- `fast` remains available as a legacy speed-first preset

### Extract by Time or Frame Range

```bash
framescli extract /path/to/video.mp4 --from 00:30 --to 01:45
framescli extract /path/to/video.mp4 --frame-start 150 --frame-end 200
```

### Audio + Transcript

```bash
framescli extract /path/to/video.mp4 \
  --voice \
  --audio-format mp3 \
  --audio-from 00:10 \
  --audio-to 01:20 \
  --normalize-audio
```

For long recordings, run transcription in resumable chunks:

```bash
framescli transcribe-run /path/to/run --chunk-duration 600 --json
```

This writes `voice/transcription-manifest.json` plus per-chunk outputs under `voice/chunks/`. Re-running `transcribe-run` resumes from the manifest and does not redo completed chunks.

When `extract --voice` runs through a workflow preset, FramesCLI now applies preset chunking automatically unless `--chunk-duration` is specified explicitly.

### Expensive Workload Guardrails

FramesCLI warns or blocks long-input workloads using measurable thresholds surfaced by `preview` and JSON output.

Warning thresholds:

- estimated frames `>= 20000`
- estimated extracted frame disk usage `>= 2 GB`
- duration `>= 2 hours`
- CPU-only transcript path in `slow` or `heavy` runtime classes on recordings `>= 30 minutes`

Blocking thresholds that require `--allow-expensive`:

- estimated frames `>= 40000`
- estimated extracted frame disk usage `>= 4 GB`
- duration `>= 3 hours`
- CPU-only transcript path in `slow` or `heavy` runtime classes on recordings `>= 45 minutes` without chunking

Override path:

```bash
framescli extract /path/to/video.mp4 --preset high-fidelity --voice --allow-expensive
```

`--allow-expensive` preserves expert control. It disables the blocking gate, but FramesCLI still emits the same guardrail details in preview and JSON output so the cost remains explicit.

### Batch Processing

```bash
framescli extract-batch "recordings/*.mp4" --voice --json
```

### Archive Outputs

```bash
framescli extract /path/to/video.mp4 --zip --metadata-csv
```

### Post-Process Hook (Optional)

Run a command after successful extraction to trigger adapters/uploader/indexers.

```bash
framescli extract /path/to/video.mp4 \
  --voice \
  --post-hook 'echo "new run: $FRAMESCLI_HOOK_OUT_DIR"' \
  --post-hook-timeout 45s
```

Security note: hooks execute via the system shell. Only use trusted commands and avoid untrusted interpolated input.

Available hook env vars:

- `FRAMESCLI_HOOK_EVENT` (`post_extract`)
- `FRAMESCLI_HOOK_INPUT`
- `FRAMESCLI_HOOK_VIDEO`
- `FRAMESCLI_HOOK_OUT_DIR`
- `FRAMESCLI_HOOK_ARTIFACTS_JSON`
- `FRAMESCLI_HOOK_RESULT_JSON`

## Agent and MCP Integration

FramesCLI supports automation via both CLI JSON mode and MCP.
For local AI coding agents, treat these as the API surface:

- `framescli mcp` is the preferred structured integration
- `--json` CLI commands are the fallback when MCP is unavailable
- Detailed copy-paste MCP and CLI recipes live in `docs/AGENT_RECIPES.md`

A separate HTTP API is not included right now because it adds deployment, auth, and lifecycle overhead without improving the local agent workflow this project is built for.

### Agent Quickstart (Copy/Paste)

```bash
# 1) Validate local readiness
framescli doctor --json

# 2) Start MCP server
framescli mcp
```

Then have your agent call tools in this order:

1. `prefs_set` with `input_dirs` and `output_root`
2. `preview` for the target video path
3. `extract` (or `extract_batch`) with `voice=true` when transcript is needed
4. `transcribe_run` if a previous run needs transcript recovery
5. `get_run_artifacts` for the indexed latest-run view; use `get_latest_artifacts` only when you need the compact latest-path map

### CLI JSON Contract

- `extract`, `extract-batch`, `preview`, `doctor`, `open-last`, and `transcribe-run` support `--json`
- Envelope includes: `schema_version`, `command`, `status`, timing, `data`, optional `error`
- `error.code` remains `command_failed`; newer clients can also read `error.class`, `error.recovery`, and `error.retryable`
- Schema version: `framescli.v1`
- Command failures return non-zero exit codes, including JSON-mode failures/partials
- Prefer `--transcribe-timeout <seconds>` for agent flows so transcript delays do not stall the whole run
- `preview` is a workload estimate only; if the source has no audio stream, `extract --voice` and `transcribe-run` still fail with a stable no-audio error
- `open-last` and `copy-last` accept `run`, `transcript`, `transcript-json`, `transcript-srt`, `transcript-vtt`, `sheet`, `log`, `metadata`, `frames`, `manifest`, `metadata-csv`, `frames-zip`, and `audio`

### MCP Server

Run over stdio:

```bash
framescli mcp
```

Tools:

- `preview`
- `extract`
- `extract_batch`
- `transcribe_run`
- `doctor`
- `open_last`
- `get_latest_artifacts`
- `get_run_artifacts`
- `prefs_get`
- `prefs_set`

Recommended MCP onboarding:

1. `framescli doctor --json`
2. Start `framescli mcp`
3. Call `prefs_set` to establish `input_dirs` and `output_root`
4. Run `preview` before extraction calls

Minimal MCP client config:

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

Path safety:

- MCP access is local-only
- Path arguments are restricted to configured allowed roots + current working directory
- JSON-RPC error `code` and `message` remain stable; newer clients can also read `error.data.class`, `error.data.recovery`, and `error.data.retryable`

## Output Layout

```text
frames/<RunName>/
  images/
    frame-0001.png
    sheets/contact-sheet.png
  voice/
    voice.wav
    transcription-manifest.json
    chunks/
      chunk-0000/
      chunk-0001/
    transcript.txt
    transcript.json
    transcript.srt
    transcript.vtt
index.json
```

Chunked transcription keeps final merged artifacts at the existing `voice/transcript.*` paths. Current limitation: merged `srt`/`vtt` are only written when chunk JSON includes segment timings.

Failed-run diagnostics are exported under `frames/diagnostics/diag-*.json`.

## Performance and Setup

First-time setup:

```bash
framescli setup
framescli doctor
framescli config
```

Benchmarking:

```bash
framescli benchmark recent --duration 20
framescli benchmark recent --apply
framescli benchmark history --limit 20
```

Recommended baseline starting points:

- Linux desktop/workstation: `--hwaccel auto --preset balanced`
- Linux headless/CI: `--hwaccel none --preset laptop-safe`
- macOS: `--hwaccel auto --preset balanced`
- WSL: `--hwaccel none --preset balanced`

## Configuration

Default config path:

- `~/.config/framescli/config.json`

Override:

- `FRAMES_CONFIG=/path/to/config.json`

Environment variables:

- `OBS_VIDEO_DIR`
- `WHISPER_BIN`
- `FASTER_WHISPER_BIN`
- `WHISPER_MODEL`
- `WHISPER_LANGUAGE`
- `TRANSCRIBE_BACKEND` (`auto|whisper|faster-whisper`)

Hook config keys:

- `post_extract_hook` (string command)
- `post_extract_hook_timeout_sec` (integer seconds)

Telemetry config keys (opt-in, local-only):

- `telemetry_enabled` (`true`/`false`, default `false`)
- `telemetry_path` (optional JSONL file path override)

When enabled, FramesCLI appends JSONL events to:

- default: `frames/telemetry/events.jsonl`

Telemetry commands:

```bash
framescli telemetry status
framescli telemetry tail -n 25
framescli telemetry prune --keep 2000
```

## Reliability and Testing

```bash
make preflight
go test ./...
go test ./cmd/frames
go test ./internal/media
go test -tags=integration ./internal/media
```

For MCP-only coverage, `go test ./cmd/frames -run 'TestMCPServer|TestMCPHelperProcess'` exercises the stdio harness in `cmd/frames/mcp_integration_test.go`. That harness runs `framescli mcp` with fake `ffmpeg`, `ffprobe`, and `whisper` binaries so handshake, `doctor`, `preview`, heartbeat, cancellation, timeout, and structured error metadata stay deterministic in CI.

## Documentation

- Product + usage docs: `README.md`
- Development roadmap: `docs/NEXT_PHASE_ROADMAP.md`
- Agent workflow recipes: `docs/AGENT_RECIPES.md`
- Repo-local agent skill: `SKILL.md`
- Media capture guide for future product demos: `docs/media/README_MEDIA.md`
- Brand assets and logo checklist: `brand/BRAND.md`, `brand/CHECKLIST.md`
- Contributing guide: `CONTRIBUTING.md`
- License: `LICENSE`

## Roadmap Snapshot

See `docs/NEXT_PHASE_ROADMAP.md` for the active development roadmap and upcoming milestones.
