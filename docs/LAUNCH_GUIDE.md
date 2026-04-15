# FramesCLI Launch Guide

Quick reference for launching FramesCLI on Homebrew and Hacker News.

## Pre-Launch Checklist

### 1. Create Homebrew Tap Repository

On GitHub, create a new public repository:
- Name: `homebrew-tap`
- Owner: `wraelen`
- Public visibility
- Initialize with README

**URL:** https://github.com/wraelen/homebrew-tap

### 2. Test Release Process

```bash
# Dry run to test goreleaser config
goreleaser release --snapshot --clean

# Check that it builds successfully
# Check dist/ directory for generated files
```

### 3. Create Release Tag

```bash
# Ensure all changes are committed
git add -A
git commit -m "chore: prepare for v0.2.2 release"
git push origin main

# Create and push tag
git tag -a v0.2.2 -m "v0.2.2: Add Homebrew distribution"
git push origin v0.2.2
```

This will automatically trigger GitHub Actions to:
- Run tests
- Build binaries
- Create GitHub release
- Generate and push Homebrew formula to `wraelen/homebrew-tap`

### 4. Verify Homebrew Installation

```bash
# Add the tap
brew tap wraelen/tap

# Install
brew install framescli

# Test
framescli --version
framescli doctor
```

### 5. Polish README

Before launch, ensure README has:
- [ ] Clear one-line description at the top
- [ ] Demo GIF or screenshot showing typical usage
- [ ] Quick start section (< 5 commands to get value)
- [ ] Installation instructions (Homebrew, go install, MCP)
- [ ] Real-world use cases with examples
- [ ] Link to full documentation
- [ ] Badge showing GitHub stars, build status, latest release

**Optional but recommended:**
- Record 2-minute demo video showing:
  1. Installation via Homebrew
  2. Extract frames from a video
  3. Feed frames to Claude to analyze the video
  4. Show the output artifacts

---

## Launch Day

### Step 1: Post to Show HN

**URL:** https://news.ycombinator.com/submit

**Recommended post:**
- **Title:** Show HN: Let AI agents "watch" videos by extracting frames and transcripts
- **URL:** https://github.com/wraelen/framescli
- **Body:** Use Version 3 from `SHOW_HN_DRAFT.md`

**Best time to post:** Tuesday-Thursday, 8-10am PT

### Step 2: Engage with Comments (First 2 Hours Critical)

Set aside 2 hours to:
- Answer questions quickly and thoroughly
- Thank people for feedback
- Acknowledge bugs/limitations honestly
- Add issues to GitHub for feature requests

**Common questions to prepare for:**
- "Why not just use FFmpeg?" → Explain sensible defaults, GPU auto-detection, MCP integration
- "What's MCP?" → Model Context Protocol, Anthropic's standard for AI tool integration
- "Privacy concerns?" → 100% local processing, no telemetry
- "Why Go instead of Python?" → Single binary, fast, cross-platform

### Step 3: Cross-Post to Reddit (After HN Traction)

**Wait 2-4 hours after HN post**, then share to relevant subreddits:

**r/programming** (highest priority)
- Title: "I built a CLI for extracting video frames for AI analysis"
- Link to GitHub + mention it's on HN front page (if applicable)

**r/MachineLearning**
- Title: "Tool for feeding videos to LLMs via frame extraction + transcription"
- Focus on AI/ML use cases

**r/LocalLLaMA**
- Title: "Local-first video extraction for AI analysis - no cloud required"
- Emphasize privacy and local processing

**r/opensource**
- Title: "FramesCLI - Open source CLI for video frame extraction (MIT license)"
- Post after reaching milestones (1K stars, 100 users, etc)

**Posting strategy:**
- Space posts 2-3 hours apart
- Don't spam the same content
- Engage with comments on each platform
- Different communities appreciate different angles

### Step 4: Optional - Product Hunt

**If HN goes well**, consider launching on Product Hunt:
- **Best day:** Tuesday-Thursday
- **Post time:** 12:01am PT (to be first in the day's queue)
- **Tagline:** "Let AI agents watch videos by extracting frames and transcripts"
- **First comment:** Explain the problem → solution → use cases

**Product Hunt vs HN:**
- HN: Developer audience, technical credibility
- PH: Broader audience, can drive non-developer users

---

## Post-Launch (Week 1)

### Monitor and Respond
- [ ] Check GitHub Issues daily
- [ ] Respond to comments on HN/Reddit within 24 hours
- [ ] Fix critical bugs immediately
- [ ] Add feature requests to GitHub Issues (don't promise delivery dates)

### Gather Feedback
- [ ] Note most common questions → add to FAQ in README
- [ ] Track feature requests → prioritize by demand
- [ ] Monitor installation issues → improve documentation

### Metrics to Track
- GitHub stars (goal: 100 in first week)
- Homebrew installs (check analytics if available)
- GitHub Issues (feature requests vs bugs)
- Engagement on HN/Reddit (comments, upvotes)

### Quick Wins
If you see common requests, ship fast:
- Documentation improvements (easy wins)
- Bug fixes (builds trust)
- Small feature additions that many users want

---

## Distribution Summary

### Install Methods
Users can install via:

1. **Homebrew (recommended for macOS/Linux):**
   ```bash
   brew install wraelen/tap/framescli
   ```

2. **Go install (for Go developers):**
   ```bash
   go install github.com/wraelen/framescli/cmd/frames@latest
   ```

3. **MCP Registry (for Claude Desktop users):**
   - Search "framescli" in Claude Desktop settings
   - One-click install

4. **Direct binary download:**
   - Go to GitHub Releases
   - Download for your OS/architecture
   - Extract and add to PATH

### URLs to Share
- **GitHub:** https://github.com/wraelen/framescli
- **Homebrew:** `brew install wraelen/tap/framescli`
- **Releases:** https://github.com/wraelen/framescli/releases
- **MCP Registry:** https://registry.modelcontextprotocol.io (search "framescli")

---

## Success Criteria

### Short-term (Week 1)
- [ ] 100+ GitHub stars
- [ ] 50+ Homebrew installs
- [ ] Front page of HN for at least 4 hours
- [ ] Zero critical bugs in installation process

### Medium-term (Month 1)
- [ ] 500+ GitHub stars
- [ ] 200+ Homebrew installs
- [ ] 5+ contributors (PRs, issues, discussions)
- [ ] Featured in at least one tech newsletter or blog

### Long-term (6 months)
- [ ] 2,000+ GitHub stars
- [ ] 1,000+ Homebrew installs
- [ ] 75+ stars + 30+ forks → eligible for homebrew-core submission
- [ ] Known in AI dev tools community

---

## If Things Go Wrong

### No Traction on HN
- Don't repost immediately (wait at least 1 month)
- Try different title/angle
- Focus on Reddit and Twitter instead

### Installation Issues
- Fix immediately and push patch release
- Post updates in HN thread if still active
- Update README with workarounds

### Negative Feedback
- Stay humble and professional
- Acknowledge limitations honestly
- Don't get defensive
- Thank people for feedback even if it's harsh

### Homebrew Formula Fails
- Check GitHub Actions logs for errors
- Verify `homebrew-tap` repository exists and is public
- Test locally with `brew install --debug wraelen/tap/framescli`
- Fall back to "install from source" instructions if needed

---

## Notes

- **Don't over-promise:** Better to under-promise and over-deliver
- **Ship fast:** If you find bugs, fix and release v0.2.3 immediately
- **Engage authentically:** HN values honest, technical discussion
- **Build in public:** Share learnings, metrics, decisions openly
- **Have fun:** You built something cool - enjoy the launch!

Good luck! 🚀
