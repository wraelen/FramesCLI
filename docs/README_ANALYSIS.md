# README Analysis & Documentation Strategy

**Purpose:** Critical analysis of current README and strategic plan for pre-launch polish

**Created:** 2026-04-15
**For:** HN launch tomorrow (targeting 8-10am PT Tuesday-Thursday)

---

## Current README: What's Wrong

### 1. **Buried Lede - Installation is Hidden**

**Problem:** Installation section starts at line 112, after:
- Logo
- Badges
- "What Your AI Can Do" (aspirational marketing)
- "How It Works" (technical explanation)
- "For AI Coding Assistants" (45 lines of agent-focused content)
- "Core Capabilities" (feature list)

**Reality:** Developers on HN want to try it **immediately**. If they can't install in 10 seconds, they bounce.

**Fix:** Installation needs to be in the top 20 lines, right after the value prop.

### 2. **AI Slop Detected - "For AI Coding Assistants" Section**

**Problem:** Lines 37-98 are written FOR agents, not FOR humans. This reads like:
- Enterprise marketing copy
- Documentation written by committee
- AI-generated feature list

**Reality:** HN readers are developers. They want:
- "Does this solve my problem?"
- "Can I install it in 10 seconds?"
- "Does it actually work?"

**Fix:** Either cut this entirely or move it to a separate `docs/AGENT_INTEGRATION.md` (which already exists).

### 3. **Installation Section is Outdated**

**Current priority order:**
1. Curl script (64 lines of explanation)
2. `go install` (1 line)
3. Build from source (multiple options)
4. Dependency install (40 lines)
5. Whisper install (20 lines)
6. Verification (5 lines)

**What's missing:** Homebrew is buried in a note on line 163: "Package-manager distribution (apt, Homebrew, winget, etc.) is planned for future releases"

**Reality:** We shipped Homebrew yesterday. It's LIVE. It should be the first option.

**Fix:**
```markdown
### Install

**Homebrew (recommended):**
```bash
brew install wraelen/tap/framescli
```

**Go:**
```bash
go install github.com/wraelen/framescli/cmd/frames@latest
```

Done. Scroll down for transcription setup if you need it.
```

### 4. **Quickstart is Too Late**

**Current:** Quickstart appears at line 256, after installation, dependencies, verification, smoke tests, and release verification.

**Reality:** People want to see if it works in 60 seconds, not read 250 lines first.

**Fix:** Quickstart should be right after installation. "Here's how to use it in 30 seconds."

### 5. **README is 944 Lines - Too Long**

**Problem:** Current README tries to be:
- Marketing page
- Installation guide
- Agent integration docs
- MCP reference
- Configuration reference
- Testing guide
- Architecture overview

**Reality:** HN readers will skim the first 100 lines, run one command, and decide if they're interested.

**Fix:** README should be:
- **What it does** (10 lines)
- **Install** (5 lines)
- **Try it** (10 lines)
- **Links to detailed docs** (everything else)

### 6. **URL Extraction Section is Buried**

**Problem:** URL extraction with yt-dlp is a KILLER feature (1000+ sites supported), but it's hidden at line 270.

**Reality:** "Extract frames from any YouTube video for AI analysis" is way cooler than "Extract frames from video.mp4"

**Fix:** Highlight URL extraction in the hero section and quickstart.

### 7. **GPU Auto-Detection is Buried**

**Problem:** GPU auto-detection (15-30x speedup) is mentioned at line 340.

**Reality:** This is a differentiator. Most tools require manual GPU setup.

**Fix:** Call this out early: "Automatically detects and uses your GPU (NVIDIA/AMD/Intel/Apple). No configuration needed."

---

## What's Actually Good

### ✅ Logo and Branding
Clean, professional SVG logo at the top. Keep this.

### ✅ Badges
Build status, license, release version, MCP registry link. Keep these.

### ✅ One-Liner Description
"FramesCLI lets AI agents 'watch' videos" - clear, specific. Keep this.

### ✅ Technical Accuracy
All the commands work. The documentation is correct. Just too much of it.

### ✅ Real-World Use Cases
Lines 21-25 ("Analyze screen recordings", "Understand tutorials") are concrete. Keep this, but condense.

### ✅ MCP Registry Callout
"Now available in the official MCP Registry" - good social proof. Keep this.

### ✅ Output Layout Section
Shows actual file structure. Developers like this. Keep it.

### ✅ Command Overview
Comprehensive list of all commands. Good reference. Keep it lower in the README.

---

## Proposed New README Structure

### Section 1: Hero (Lines 1-30)
```markdown
# FramesCLI

[Logo]

**Let AI agents "watch" videos.** Extract frames + transcripts so Claude, GPT, or any AI can analyze visual and audio content.

[Badges]

### Install (30 seconds)

**Homebrew:**
```bash
brew install wraelen/tap/framescli
framescli doctor
```

**Go:**
```bash
go install github.com/wraelen/framescli/cmd/frames@latest
```

**MCP Registry:** Search "framescli" in Claude Desktop, Cursor, Cline, or Windsurf.

### Try It (60 seconds)

```bash
# Extract from local video
framescli extract video.mp4 --fps 4 --preset balanced

# Or from YouTube
framescli extract --url "https://youtube.com/watch?v=..." --fps 4 --voice

# Now ask Claude: "Read the extracted frames and summarize this video"
```

[Screenshots showing before/after]
```

**Why this works:**
- Developers can copy/paste and try it in 60 seconds
- No reading walls of text
- Homebrew-first (easiest install)
- Concrete examples (not abstract features)

### Section 2: Why This Exists (Lines 31-60)
```markdown
### Why?

**Problem:** AI can't watch videos. You can't paste a video into Claude.

**Solution:** FramesCLI extracts the visual timeline (frames at 1fps, 4fps, 8fps) and the audio (full transcript with timestamps).

**Result:** Your AI agent can "watch" videos by reading the structured artifacts.

### Real-World Use Cases

- **Debug screen recordings:** "What error appeared at 2:30?" → Agent sees frames + transcript
- **Summarize tutorials:** "What are the key steps?" → Agent follows visual timeline
- **Meeting notes:** "What decisions were made?" → Agent reads transcript + sees slides
- **YouTube analysis:** "Summarize this lecture" → Agent processes frames + spoken content

### What Makes This Different

✅ **GPU auto-detection:** 15-30x faster on NVIDIA/AMD/Intel/Apple (no setup)
✅ **1000+ sites:** Download from YouTube, Vimeo, Twitter via yt-dlp
✅ **Local-first:** Zero cloud dependencies, zero telemetry
✅ **MCP integration:** Works with Claude Desktop, Cursor, Cline, Windsurf
✅ **Single binary:** No Python venvs, no Docker, no Node modules
```

**Why this works:**
- Concrete problem/solution framing
- Real use cases (not marketing speak)
- Differentiators without hype

### Section 3: Transcription Setup (Lines 61-90)
```markdown
### Optional: Transcription Setup

Frame extraction works out of the box. For `--voice` transcription, install whisper:

```bash
brew install yt-dlp  # Optional: for URL extraction
pip install openai-whisper  # Optional: for transcription
```

Verify:
```bash
framescli doctor
```

Shows detected GPU, transcription backend, and recommendations.
```

**Why this works:**
- Clear that transcription is optional
- One-line install commands
- Links to `doctor` for verification

### Section 4: Common Workflows (Lines 91-150)
```markdown
### Common Workflows

**Extract frames from local video:**
```bash
framescli extract video.mp4 --fps 4 --preset balanced
```

**Extract from YouTube with transcript:**
```bash
framescli extract --url "https://youtube.com/watch?v=..." --fps 4 --voice
```

**Preview cost before extraction:**
```bash
framescli preview video.mp4 --preset balanced --mode both
# Shows: frame count, disk usage, transcript time, guardrails
```

**Batch process multiple videos:**
```bash
framescli extract-batch "recordings/*.mp4" --fps 1 --voice
```

**Use with Claude Desktop (MCP):**
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

Then: "Claude, use framescli to watch video.mp4 and summarize it"
```

**Why this works:**
- Copy/paste ready
- Covers 80% of use cases
- Concrete commands, not abstract features

### Section 5: Documentation Links (Lines 151-180)
```markdown
### Documentation

**For Developers:**
- [Agent Integration Guide](docs/AGENT_INTEGRATION.md) - JSON schemas, MCP tools, automation
- [Command Reference](docs/COMMAND_REFERENCE.md) - All CLI commands with examples
- [Performance Guide](docs/PERFORMANCE.md) - GPU setup, benchmarking, optimization

**For Contributors:**
- [Contributing Guide](CONTRIBUTING.md)
- [Development Setup](docs/DEVELOPMENT.md)
- [Roadmap](docs/NEXT_PHASE_ROADMAP.md)

**Project:**
- [License](LICENSE) - MIT
- [Issues](https://github.com/wraelen/framescli/issues)
- [Releases](https://github.com/wraelen/framescli/releases)
```

**Why this works:**
- Links, not walls of text
- Clear categories (users vs contributors)
- Everything else is in dedicated docs

### Section 6: Output Structure (Lines 181-210)
```markdown
### Output Structure

```text
frames/Run_20260415-083045/
  images/
    frame-0001.png
    frame-0002.png
    ...
  voice/
    transcript.txt
    transcript.json
    transcript.srt
    voice.wav
  run.json          # Metadata: fps, duration, preset, paths
  frames.json       # Per-frame timing and paths
```

All artifacts are JSON-parseable for automation. See [schemas/](docs/schemas/) for details.
```

**Why this works:**
- Shows actual file structure
- Developers understand immediately
- Links to schemas for automation

---

## Proposed PDF Guides (Visual Documentation)

### PDF 1: "Install FramesCLI in 30 Seconds"
**Format:** Screenshot walkthrough
**Content:**
1. Open terminal
2. Run `brew install wraelen/tap/framescli`
3. Run `framescli doctor`
4. Success - green checkmarks for ffmpeg, GPU detected

**Why:** Lowers barrier to entry. Visual proof it's easy.

### PDF 2: "Ask Claude to Watch a Video"
**Format:** Step-by-step with screenshots
**Content:**
1. Install framescli via Homebrew
2. Add MCP config to Claude Desktop
3. Upload a video
4. Chat: "Use framescli to watch video.mp4 and summarize it"
5. Show Claude's response with frame references

**Why:** This is the killer use case. Show it working end-to-end.

### PDF 3: "Extract Frames from YouTube"
**Format:** Terminal recording → GIF/screenshots
**Content:**
1. `framescli extract --url "https://youtube.com/watch?v=..." --fps 4 --voice`
2. Show progress output
3. Show extracted frames directory
4. Show transcript.txt content
5. Total time: 30 seconds

**Why:** URL extraction is a differentiator. Prove it works.

### PDF 4: "GPU Auto-Detection"
**Format:** Side-by-side comparison
**Content:**
1. `framescli doctor` output showing GPU detected
2. Benchmark comparison: CPU vs GPU (1x vs 25x)
3. No configuration needed - it just works

**Why:** GPU acceleration is a killer feature. Show the speedup.

### PDF 5: "Batch Process 100 Videos"
**Format:** Terminal recording
**Content:**
1. Directory with 100 videos
2. `framescli extract-batch "videos/*.mp4" --fps 1 --preset laptop-safe`
3. Progress bar processing all videos
4. Show final artifact index

**Why:** Demonstrates scale and automation.

---

## Documentation Hierarchy

**README.md** (200 lines max)
- Hero: What it does, install, try it
- Why: Problem/solution, use cases, differentiators
- Common workflows
- Links to detailed docs

**docs/INSTALL.md** (Move current installation content here)
- Homebrew (primary)
- Go install
- Build from source
- Docker (future)
- Dependency installation (ffmpeg, yt-dlp, whisper)
- Platform-specific notes (WSL, macOS, Linux)

**docs/QUICKSTART.md** (Expand current quickstart)
- 5-minute walkthrough
- Extract from local video
- Extract from URL
- Transcription
- MCP integration
- Batch processing

**docs/AGENT_INTEGRATION.md** (Already exists)
- Keep all agent-specific content here
- MCP tools reference
- JSON schemas
- Automation recipes
- Error handling

**docs/COMMAND_REFERENCE.md** (New - extract from README)
- All CLI commands
- All flags
- Examples for each
- Exit codes
- JSON output schemas

**docs/PERFORMANCE.md** (New - extract from README)
- GPU auto-detection
- Hardware acceleration modes
- Benchmarking
- Workflow presets
- Optimization tips

**docs/GUIDES/** (New directory for PDF guides)
- `01-install-homebrew.pdf`
- `02-claude-watch-video.pdf`
- `03-youtube-extraction.pdf`
- `04-gpu-acceleration.pdf`
- `05-batch-processing.pdf`

---

## Action Plan (Pre-Launch)

### Phase 1: README Rewrite (30 min)
1. ✅ Analyze current README (this document)
2. ⏳ Write new README (200 lines max)
3. ⏳ Move detailed content to dedicated docs
4. ⏳ Verify all links work
5. ⏳ Test all commands in README

### Phase 2: Visual Documentation (60 min)
1. ⏳ Record terminal session: Homebrew install + doctor
2. ⏳ Record terminal session: YouTube extraction
3. ⏳ Screenshot: Claude Desktop MCP integration
4. ⏳ Screenshot: GPU auto-detection output
5. ⏳ Convert recordings to GIFs or MP4s
6. ⏳ Create PDFs from screenshots

### Phase 3: Documentation Split (30 min)
1. ⏳ Create `docs/INSTALL.md` (move from README)
2. ⏳ Create `docs/QUICKSTART.md` (expand)
3. ⏳ Create `docs/COMMAND_REFERENCE.md` (extract)
4. ⏳ Create `docs/PERFORMANCE.md` (extract)
5. ⏳ Update README links to point to new docs

### Phase 4: Final Polish (15 min)
1. ⏳ Spell check
2. ⏳ Link check
3. ⏳ Version consistency (v0.2.2 everywhere)
4. ⏳ Badge accuracy
5. ⏳ One final `make preflight`

**Total estimated time:** 2h 15m
**Can be done tonight:** Yes

---

## README Principles (Anti-AI-Slop)

### ✅ DO
- Use concrete examples ("Extract from YouTube")
- Show actual commands that work
- Use screenshots/GIFs sparingly but effectively
- Write for developers who skim
- Front-load value (install in top 20 lines)
- Link to detailed docs (don't inline everything)

### ❌ DON'T
- Use marketing language ("revolutionary", "cutting-edge")
- Write for AI agents in a human-facing README
- Bury installation behind explanations
- Explain every single flag inline
- Use abstract feature lists
- Write walls of text

### Examples of Good vs Bad

**Bad (AI slop):**
> "FramesCLI is a revolutionary, cutting-edge solution for next-generation AI-powered video analysis workflows, enabling seamless integration with modern LLM architectures through our innovative frame extraction pipeline."

**Good:**
> "Extract video frames + transcripts so AI can watch videos."

**Bad (over-explanation):**
> "To install FramesCLI, you have multiple options depending on your environment and preferences. The recommended approach for most users is to utilize our bootstrap installer script, which automatically handles binary installation, dependency verification, and initial configuration setup."

**Good:**
> "Install: `brew install wraelen/tap/framescli`"

**Bad (abstract features):**
> "FramesCLI provides comprehensive video ingestion capabilities with support for multiple transport protocols and configurable sampling strategies."

**Good:**
> "Extract frames from YouTube, local files, or any URL yt-dlp supports."

---

## Next Steps

Ready to execute. Waiting for your go-ahead on:

1. **README rewrite** - Start with the 200-line structure above?
2. **Visual docs** - Which PDFs are highest priority?
3. **Documentation split** - Move detailed content to dedicated docs?

All of this can be done tonight before the HN launch tomorrow.
