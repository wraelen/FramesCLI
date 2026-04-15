# Show HN: Draft Post

## Title Options

**Option 1 (Direct):**
```
Show HN: FramesCLI – Extract video frames and transcripts for AI analysis
```

**Option 2 (Benefit-focused):**
```
Show HN: Let AI agents "watch" videos by extracting frames and transcripts
```

**Option 3 (Technical):**
```
Show HN: CLI tool for feeding videos to LLMs via frame extraction + transcription
```

**Recommended:** Option 2 (most engaging, explains the "why")

---

## Post Body

### Version 1: Technical & Concise

```markdown
I built FramesCLI to solve a problem I kept running into: how do you let AI analyze screen recordings, tutorials, or meeting videos when they can only see text and images?

FramesCLI extracts:
- Video frames at configurable intervals (1fps, 4fps, 8fps)
- Full transcripts with word-level timestamps
- Structured JSON metadata for easy parsing

Then you can feed the frames + transcript to Claude, GPT-4V, or any multimodal LLM.

**Tech:**
- Written in Go, works on macOS/Linux/Windows
- Uses ffmpeg for frame extraction (with GPU acceleration)
- Supports URL ingestion via yt-dlp (YouTube, Vimeo, 1000+ sites)
- MCP server for direct integration with Claude Desktop, Cursor, etc.

**Install:**
```bash
# Homebrew (macOS/Linux)
brew install wraelen/tap/framescli

# Or from source
go install github.com/wraelen/framescli/cmd/frames@latest
```

**Use cases I've tested:**
- Debugging: "What error appeared at 2:30 in this screen recording?"
- Tutorials: "Summarize the steps in this demo video"
- Meetings: "What decisions were made?" (frames show slides, transcript shows discussion)

It's 100% local-first (no cloud required), MIT licensed, and designed for agent workflows.

GitHub: https://github.com/wraelen/framescli

Would love feedback on the MCP integration - it's my first time building a Model Context Protocol server and I'm curious if other developers would find this useful.
```

---

### Version 2: Story-Driven

```markdown
I was trying to get Claude to help me debug a production issue from a screen recording. Problem: Claude can't watch videos.

So I built a CLI tool that turns videos into something AI can actually consume: extracted frames + full transcripts.

**What it does:**
- Extracts video frames as images (configurable FPS)
- Transcribes audio with word-level timestamps
- Outputs structured JSON for easy parsing
- Works with URLs (YouTube, Vimeo, etc via yt-dlp)

**Then you can ask Claude:**
"Look at these frames from my screen recording and tell me when the error appeared"

**Example workflow:**
```bash
framescli extract meeting.mp4 --voice --preset balanced
# → Outputs: frames/meeting-2024-04-15/
#   ├── images/frame-0001.png ... frame-0477.png
#   ├── voice/transcript.json
#   └── run.json
```

Now Claude can "see" the meeting slides (frames) and "hear" the discussion (transcript).

**Tech details:**
- Written in Go, cross-platform (macOS/Linux/Windows)
- GPU-accelerated extraction (CUDA, VAAPI, VideoToolbox)
- MCP server for direct integration with Claude Desktop/Cursor
- 100% local-first, no cloud required

**Install:**
```bash
brew install wraelen/tap/framescli
```

GitHub: https://github.com/wraelen/framescli

I built this primarily for my own workflows (analyzing customer support calls, debugging sessions, meeting notes), but figured others might find it useful. Open to feedback!
```

---

### Version 3: Problem → Solution (Recommended)

```markdown
**Problem:** AI can't watch videos. You can describe what's in a screen recording, but you can't just hand Claude a 30-minute tutorial and say "explain this."

**Solution:** Extract the video into frames (what the AI can see) + transcript (what the AI can hear), then feed both to any multimodal LLM.

That's what FramesCLI does. It's a CLI tool (and MCP server) that turns videos into AI-readable artifacts.

**How it works:**
```bash
framescli extract tutorial.mp4 --voice --preset balanced
```

This outputs:
- 400+ frames as PNG/JPG images (sampled at your chosen FPS)
- Full transcript with word-level timestamps (JSON, SRT, TXT, VTT)
- Structured metadata (duration, resolution, frame timings)

Then you can ask Claude/GPT-4V:
- "What error appeared at 2:30 in this recording?"
- "Summarize the steps in this tutorial"
- "What decisions were made in this meeting?"

**Real-world use cases:**
- **Debugging:** Analyze screen recordings to find when/where errors occur
- **Tutorials:** Let AI summarize coding walkthroughs or conference talks
- **Meetings:** Extract key decisions from recorded calls
- **Support:** Analyze customer screen recordings to diagnose issues

**Features:**
- 🚀 GPU acceleration (CUDA, VAAPI, VideoToolbox) - 15-30x faster extraction
- 🌐 URL ingestion via yt-dlp (YouTube, Vimeo, 1000+ sites)
- 🤖 MCP server for direct Claude Desktop/Cursor integration
- 📦 100% local-first (no cloud, no tracking)
- 🎛️ Configurable presets: laptop-safe, balanced, high-fidelity

**Install:**
```bash
# Homebrew (macOS/Linux)
brew install wraelen/tap/framescli

# Or from source
go install github.com/wraelen/framescli/cmd/frames@latest
```

**Technical:**
- Written in Go, cross-platform
- Uses ffmpeg for extraction, whisper for transcription
- MIT licensed, actively maintained
- MCP server implements Model Context Protocol for AI agent integration

GitHub: https://github.com/wraelen/framescli

I built this primarily for my own workflows (analyzing sales calls, debugging sessions), but realized it solves a broader problem. Would love feedback, especially on the MCP integration!
```

---

## Posting Strategy

### When to Post
- **Best days:** Tuesday, Wednesday, Thursday
- **Best time:** 8-10am PT (when HN traffic peaks)
- **Avoid:** Friday afternoon, weekends, holidays

### Engagement Tips
1. **Respond quickly** - First 2 hours are critical for ranking
2. **Be humble** - HN appreciates "would love feedback" over "revolutionary tool"
3. **Share technical details** - If someone asks "how did you implement X", give a detailed answer
4. **Acknowledge limitations** - "Currently doesn't support X, but planning to add it"
5. **Thank people** - Even for critical feedback

### Common Questions to Prepare For
- **Q: Why not just use FFmpeg directly?**
  - A: You can! FramesCLI is a wrapper with sensible defaults, GPU auto-detection, and MCP integration. It's for people who want `framescli extract video.mp4` instead of memorizing ffmpeg flags.

- **Q: How is this different from existing tools?**
  - A: Most video analysis tools are cloud-based SaaS products. FramesCLI is local-first, designed for agent workflows, and integrates directly with Claude Desktop via MCP.

- **Q: What's MCP?**
  - A: Model Context Protocol - Anthropic's standard for connecting AI assistants to external tools. It lets Claude directly call FramesCLI commands without copy-pasting.

- **Q: Why Go instead of Python?**
  - A: Single-binary distribution, fast startup, easy cross-compilation. Users get `brew install framescli` instead of managing Python dependencies.

- **Q: Privacy concerns?**
  - A: 100% local processing. No telemetry, no cloud uploads, no tracking. All video processing happens on your machine.

---

## Launch Checklist

Before posting to Show HN:

- [ ] Create `wraelen/homebrew-tap` repository on GitHub
- [ ] Push v0.2.2 tag to trigger Homebrew formula generation
- [ ] Test Homebrew installation: `brew install wraelen/tap/framescli`
- [ ] Record 2-minute demo video (optional but highly recommended)
- [ ] Add demo GIF to README (for quick visual understanding)
- [ ] Ensure GitHub Issues are enabled
- [ ] Add CONTRIBUTING.md if you want contributors
- [ ] Polish README with clear examples

---

## Recommended: Version 3

**Why Version 3 is best:**
- Starts with the problem (AI can't watch videos) → immediately relatable
- Shows the solution clearly (extract frames + transcript)
- Includes real-world use cases (debugging, tutorials, meetings)
- Technical details for developers who want to dig deeper
- Humble tone ("built for my own workflows, but figured others might find it useful")
- Invites feedback on MCP integration (shows you're open to learning)

**Title to use:**
```
Show HN: Let AI agents "watch" videos by extracting frames and transcripts
```

**URL to submit:**
```
https://github.com/wraelen/framescli
```

Good luck with the launch! 🚀
