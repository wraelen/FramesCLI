# FramesCLI: Next Steps for Agent Discovery

**Status:** Documentation and discovery infrastructure complete ✅
**Date:** 2026-04-13
**Goal:** Make FramesCLI the go-to tool for AI agents processing screen recordings

---

## 🚀 Immediate Actions (This Week)

### 1. Commit and Push Current Work

```bash
# Stage all new files
git add docs/AGENT_INTEGRATION.md \
        docs/MCP_REGISTRY_SUBMISSION.md \
        docs/schemas/ \
        homebrew/ \
        scripts/generate-schemas.sh \
        .gitignore \
        CLAUDE.md \
        README.md

# Commit
git commit -m "feat: add comprehensive agent integration documentation and Homebrew tap

- Add AGENT_INTEGRATION.md with framework-specific guides
- Add 'For AI Coding Assistants' section to README
- Create Homebrew tap formula for easy installation
- Prepare MCP registry submission materials
- Add JSON schemas infrastructure
- Update .gitignore to exclude test extraction outputs

Supports agent discoverability and onboarding"

# Push
git push origin main
```

### 2. Create Homebrew Tap Repository

**Time:** 15 minutes

**Steps:**

1. **Create new GitHub repo:**
   ```bash
   # Via GitHub CLI
   gh repo create wraelen/homebrew-framescli --public --description "Homebrew tap for FramesCLI"

   # Or manually at: https://github.com/new
   # Name: homebrew-framescli (MUST start with "homebrew-")
   ```

2. **Clone and setup:**
   ```bash
   git clone https://github.com/wraelen/homebrew-framescli.git
   cd homebrew-framescli
   mkdir -p Formula
   cp /path/to/frames-cli-standalone/homebrew/framescli.rb Formula/
   ```

3. **Update SHA256 checksums:**
   ```bash
   # Download checksums from your v0.1.0 release
   curl -sL https://github.com/wraelen/framescli/releases/download/v0.1.0/checksums.txt > checksums.txt

   # Extract SHA256 for each platform
   grep "darwin_amd64.tar.gz" checksums.txt  # Copy SHA256
   grep "darwin_arm64.tar.gz" checksums.txt
   grep "linux_amd64.tar.gz" checksums.txt
   grep "linux_arm64.tar.gz" checksums.txt

   # Update Formula/framescli.rb with actual SHA256 values
   # Replace all "REPLACE_WITH_ACTUAL_SHA256_*" placeholders
   ```

4. **Test locally (macOS/Linux):**
   ```bash
   brew install --build-from-source ./Formula/framescli.rb
   brew test framescli
   framescli doctor
   ```

5. **Commit and push:**
   ```bash
   git add Formula/framescli.rb
   git commit -m "Add framescli v0.1.0 formula"
   git push origin main
   ```

6. **Test tap installation:**
   ```bash
   brew uninstall framescli  # If installed from local
   brew tap wraelen/framescli
   brew install framescli
   ```

**Validation:**
- [ ] `brew tap wraelen/framescli` works
- [ ] `brew install framescli` succeeds
- [ ] `framescli doctor` runs
- [ ] Shell completions are generated

**Documentation to update after tap is live:**
- Update README.md Install section to include: `brew tap wraelen/framescli && brew install framescli`
- Tweet/announce Homebrew installation option

---

### 3. Submit to MCP Registry

**Time:** 30 minutes (if registry has clear contribution guide)

**Steps:**

1. **Prepare logo (if needed):**
   ```bash
   # Option A: Use existing icon (if it's already 512x512)
   cp brand/exports/logo-icon-color.png mcp-logo.png

   # Option B: Resize to exactly 512x512 (if required by registry)
   convert brand/exports/logo-icon-color.png \
           -resize 512x512 \
           -background none \
           -gravity center \
           -extent 512x512 \
           mcp-logo-512.png

   # Option C: Use the new SVG logo you mentioned
   # Convert SVG to 512x512 PNG:
   convert "/path/to/FramesCLI logo3.svg" \
           -resize 512x512 \
           -background none \
           -gravity center \
           -extent 512x512 \
           mcp-logo-512.png
   ```

   **Note:** For MCP registry, a **square icon (not wordmark)** is better. If your new logo is a clean icon/symbol, use that instead of the wordmark.

2. **Fork and clone MCP servers registry:**
   ```bash
   # Fork via GitHub
   gh repo fork modelcontextprotocol/servers

   # Clone your fork
   git clone https://github.com/wraelen/servers.git mcp-servers
   cd mcp-servers
   ```

3. **Check registry structure:**
   ```bash
   # Look for CONTRIBUTING.md or similar
   cat CONTRIBUTING.md

   # Check existing entries for format
   ls src/
   ```

4. **Add FramesCLI entry:**

   Follow the registry's schema. Likely need to add a JSON/YAML entry like:

   ```json
   {
     "name": "framescli",
     "description": "Turn screen recordings into timestamped frames, transcripts, and searchable artifacts for debugging and incident analysis",
     "homepage": "https://github.com/wraelen/framescli",
     "repository": "https://github.com/wraelen/framescli",
     "license": "MIT",
     "categories": ["development-tools", "media-content"],
     "logo": "https://raw.githubusercontent.com/wraelen/framescli/main/brand/exports/logo-icon-color.png",
     "installation": {
       "command": "framescli",
       "args": ["mcp"]
     },
     "documentation": "https://github.com/wraelen/framescli/blob/main/docs/AGENT_INTEGRATION.md"
   }
   ```

5. **Create PR:**
   ```bash
   git checkout -b add-framescli
   git add <modified files>
   git commit -m "Add FramesCLI - Video processing for AI agents"
   git push origin add-framescli

   # Create PR via GitHub
   gh pr create --title "Add FramesCLI - Video processing for AI agents" \
                --body "FramesCLI turns screen recordings into agent-ready artifacts (frames, transcripts, metadata).

   Use cases:
   - Debug session analysis
   - Incident review
   - Coding session documentation

   Documentation: https://github.com/wraelen/framescli/blob/main/docs/AGENT_INTEGRATION.md"
   ```

6. **Respond to review:**
   - Address any schema issues
   - Update logo if format wrong
   - Iterate on description

**Validation:**
- [ ] PR submitted to modelcontextprotocol/servers
- [ ] Logo is correct format (512x512 PNG or as required)
- [ ] Categories are appropriate
- [ ] Installation command tested

**Expected Timeline:**
- PR review: 3-7 days (depends on registry maintainers)
- Approval and merge: Variable

---

## 📊 Medium-Term Actions (Next 2-4 Weeks)

### 4. Create Demo GIF/Video

**Why:** Visual examples help humans (who then tell agents to use your tool)

**What to create:**

1. **15-second GIF showing MCP workflow:**
   - Frame 1: Configure MCP in Cursor/Cline
   - Frame 2: Ask agent "analyze this bug recording"
   - Frame 3: Agent calls framescli tools
   - Frame 4: Agent shows results with timestamps

2. **30-second YouTube demo:**
   - Show `framescli extract` in action
   - Show contact sheet generation
   - Show transcript output
   - Show agent reading artifacts

**Tools:**
- Record with OBS/QuickTime
- Edit with FFmpeg: `ffmpeg -i recording.mp4 -vf fps=10,scale=800:-1 -loop 0 demo.gif`
- Or use https://gifski.app for high-quality GIFs

**Where to use:**
- README.md (add GIF after "For AI Coding Assistants" section)
- Twitter/social when announcing
- MCP registry (if they support media)

### 5. Test with Real Agents

**Target agents to test:**
- ✅ Cursor AI (MCP support)
- ✅ Cline (VS Code extension, MCP support)
- ✅ Windsurf Cascade (MCP support)
- ✅ Claude Desktop (native MCP support)

**Testing workflow:**
1. Configure MCP in each client
2. Give prompt: "Analyze the screen recording at /path/to/video.mp4"
3. Observe which tools agents call
4. Document any pain points or confusion
5. Refine docs/prompts based on real usage

**Document findings:**
- Create `docs/AGENT_TESTING_RESULTS.md` with:
  - Which agents work well
  - Common failure modes
  - Recommended prompts for users
  - Any tool improvements needed

### 6. Gather Early Feedback

**Channels:**
- Post in MCP Discord/community (if one exists)
- Tweet with #MCP #AIAgents #DevTools
- Post in r/MachineLearning or r/LocalLLaMA if appropriate
- Share in AI agent communities (Cursor forum, Cline discussions)

**Ask for:**
- Use cases they'd want
- Pain points in current workflow
- Feature requests
- Documentation clarity

**Track in GitHub Discussions or Issues**

---

## 🔧 Code Improvements (Ongoing)

### 7. Add Missing Features (from original todo list)

**Priority order:**

1. **Fix TestApplyDropProfile** (5 min)
   - Check `internal/tui/dashboard_test.go:454`
   - Likely a profile struct comparison issue
   - Low impact but good to fix for clean tests

2. **Add `--dry-run` flag to extract** (30 min)
   - Useful for agents to validate parameters
   - Returns what *would* be extracted without running
   - Add to `extractCommand()` in `cmd/frames/main.go`

3. **Add `list_runs` MCP tool** (45 min)
   - Simpler alternative to `get_run_artifacts`
   - Returns list of run directories with basic metadata
   - Add to MCP tools in `handleMCPRequest()` around line 3309

4. **Expand MCP integration tests** (2-3 hours)
   - Add cancellation test (send cancel notification mid-extract)
   - Add timeout test (verify tool respects timeout_ms)
   - Add heartbeat verification test
   - Add to `cmd/frames/mcp_integration_test.go`

**Implementation notes in:** `docs/CODE_IMPROVEMENTS.md` (if you need detailed guidance)

---

## 📈 Growth & Discovery (Next 1-3 Months)

### 8. Content & Awareness

**Blog posts/tutorials:**
- "How AI Agents Can Analyze Screen Recordings"
- "Debugging with AI: Turn Videos into Searchable Timelines"
- "Building Agent-First Dev Tools"

**Where to publish:**
- Dev.to
- Medium
- Hacker News (Show HN: FramesCLI)
- Your personal blog

**Social:**
- Tweet thread showing agent workflow
- Demo video on YouTube
- Post in AI/DevTools communities

### 9. Integrations

**Potential integrations:**
- **Sentry/Error tracking:** Auto-analyze attached screen recordings
- **Linear/GitHub Issues:** Extract frames from repro videos
- **Slack:** Bot that processes shared recordings
- **Loom alternative:** Local-first recording + AI analysis

**Priority:** Wait for user feedback before building integrations

### 10. Package Manager Distribution

**Timeline: After 100+ GitHub stars and proven usage**

**Homebrew Core submission:**
- Requires stable releases
- Active maintenance demonstrated
- Clean audit: `brew audit --strict --online framescli`
- PR to Homebrew/homebrew-core

**Other package managers:**
- `apt` (Debian/Ubuntu): PPA or package repository
- `winget` (Windows): Submit manifest
- `nix` (NixOS): Add package

**Note:** Tap is sufficient for now. Wait for demand before expanding.

---

## ✅ Validation Checklist

Use this to track completion:

### Week 1
- [ ] Git commit and push all docs
- [ ] Create homebrew-framescli repository
- [ ] Update Homebrew formula with real SHA256s
- [ ] Test `brew install framescli` locally
- [ ] Submit MCP registry PR
- [ ] Create demo GIF (optional but recommended)

### Week 2-3
- [ ] MCP registry PR merged (waiting on maintainers)
- [ ] Test with at least 2 different AI agents
- [ ] Document agent testing results
- [ ] Fix TestApplyDropProfile
- [ ] Add --dry-run flag to extract

### Month 2
- [ ] Add list_runs MCP tool
- [ ] Expand MCP integration tests
- [ ] Write blog post or tutorial
- [ ] Share on social/communities
- [ ] Gather initial user feedback

### Month 3+
- [ ] 100+ GitHub stars
- [ ] Consider Homebrew Core submission
- [ ] Evaluate integration opportunities
- [ ] Plan v0.2.0 features based on feedback

---

## 🆘 Troubleshooting

### Homebrew tap issues

**Problem:** `brew tap` fails
- Ensure repo name starts with `homebrew-`
- Check repo is public
- Verify Formula/framescli.rb exists

**Problem:** SHA256 mismatch
- Download checksums.txt from exact release version
- Copy SHA256 values exactly (no extra spaces)
- Ensure release version in formula matches URLs

**Problem:** `brew test` fails
- Check `framescli --help` works in installed location
- Verify ffmpeg dependency is available
- Check `framescli doctor --json` succeeds

### MCP registry issues

**Problem:** Don't know where to add entry
- Check CONTRIBUTING.md in registry repo
- Look at existing entries for format
- Ask in registry issues/discussions

**Problem:** Logo wrong format
- Use 512x512 PNG with transparent background
- Ensure file size <100KB if possible
- Test logo renders well at small sizes

**Problem:** Categories unclear
- Primary: `development-tools` (debugging use case)
- Secondary: `media-content` (video processing)
- Avoid too many categories (2-3 max)

---

## 📝 Documentation to Update

After completing immediate actions:

1. **README.md:**
   - Update Install section with Homebrew tap
   - Add badge if MCP registry has one
   - Add demo GIF after "For AI Coding Assistants"

2. **CHANGELOG.md:**
   - Add entry for agent documentation release
   - Note Homebrew availability
   - List MCP registry status

3. **SKILL.md:**
   - Update with MCP registry link once live
   - Add Homebrew install command

4. **docs/AGENT_INTEGRATION.md:**
   - Add "Listed in MCP Registry" badge once merged
   - Update any outdated examples from testing

---

## 🎯 Success Metrics

Track these to measure agent adoption:

### Immediate (1 week)
- [ ] Homebrew tap has 5+ installs (via analytics if available)
- [ ] MCP registry PR has 2+ thumbs up
- [ ] At least 1 person tests and provides feedback

### Short-term (1 month)
- [ ] 50+ GitHub stars
- [ ] 10+ successful agent workflows documented
- [ ] Featured in at least 1 AI agent community post
- [ ] MCP registry entry is live

### Medium-term (3 months)
- [ ] 100+ GitHub stars
- [ ] 3+ blog posts/tutorials by community
- [ ] 20+ closed issues (indicates active usage)
- [ ] 5+ contributors

---

## 💡 Ideas for Future Consideration

**Not urgent, but keep in mind:**

1. **Agent-optimized output formats:**
   - Markdown timeline of frames + transcript
   - Single JSON with embedded frame paths + transcript
   - Mermaid diagram of video timeline

2. **Smarter frame sampling:**
   - Scene change detection (only extract when visual changes)
   - OCR-based sampling (extract when text appears on screen)
   - Motion-based sampling (more frames during activity)

3. **Multi-modal output:**
   - Combine frames into video strips
   - Annotated contact sheet with transcript overlays
   - Interactive HTML viewer

4. **Cloud integration (carefully):**
   - Optional S3/GCS upload for large artifacts
   - Shared link generation for agent collaboration
   - Keep local-first philosophy

---

## 📞 Getting Help

If stuck on any step:

1. **MCP Registry:** Check their GitHub Issues/Discussions
2. **Homebrew:** #homebrew on Libera.Chat IRC or GitHub discussions
3. **General:** Open an issue in your framescli repo and tag me (@wraelen)

---

## Final Notes

**You're in an excellent position!** The hard work of documentation and infrastructure is done. Now it's about:

1. **Distribution** (Homebrew tap, MCP registry) - Operational tasks
2. **Validation** (test with real agents) - Learn what works
3. **Iteration** (improve based on feedback) - Make it even better

**The discovery problem is solved.** Agents that search for "video processing," "screen recording analysis," or check the MCP registry will find you.

**Next big milestone:** First agent successfully using FramesCLI in the wild without your direct help. That's when you know it's working.

Good luck! 🚀
