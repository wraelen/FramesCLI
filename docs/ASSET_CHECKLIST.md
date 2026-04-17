# Visual Assets Checklist for HN Launch

**Purpose:** Clear task list for creating visual documentation assets
**Owner:** Wraelen
**Deadline:** Before HN post tomorrow morning

---

## Priority 1: Hero Demo (Required for README)

### [ ] Task 1: Homebrew Installation GIF
**What to record:**
```bash
# Start from clean terminal
brew install wraelen/tap/framescli
framescli --version
framescli doctor
```

**Expected output:**
- Homebrew install progress
- Version: framescli v0.2.2
- Doctor output showing:
  - [ok] ffmpeg
  - [ok] ffprobe
  - [ok] yt-dlp
  - GPU: NVIDIA GeForce RTX 3070 Ti (nvidia)
  - Recommended: hwaccel=cuda

**Format:** Terminal recording → GIF (asciinema or vhs)
**Filename:** `docs/assets/install-homebrew.gif`
**Duration:** ~30 seconds
**Size:** Keep under 5MB for GitHub

**Tools:**
- asciinema: `brew install asciinema` → `asciinema rec install.cast`
- OR vhs: `brew install vhs` → create vhs script
- Convert to GIF: `agg install.cast install.gif --speed 1.5`

---

### [ ] Task 2: Quick Extract Demo GIF
**What to record:**
```bash
# Use a short local video (5-10 seconds)
framescli extract test-video.mp4 --fps 4 --preset balanced

# Show output
ls frames/Run_*/
ls frames/Run_*/images/ | head -10
```

**Expected output:**
- Progress bar: "Extracting frames... 100%"
- Success message with artifact paths
- Directory listing showing extracted frames

**Format:** Terminal recording → GIF
**Filename:** `docs/assets/extract-demo.gif`
**Duration:** ~20 seconds
**Size:** Keep under 5MB

---

### [ ] Task 3: YouTube Extraction Demo GIF
**What to record:**
```bash
# Use a short YouTube video (1-2 min)
framescli extract --url "https://www.youtube.com/watch?v=dQw4w9WgXcQ" --fps 1 --no-sheet --out /tmp/yt-test

# Show output
ls /tmp/yt-test/images/ | wc -l
cat /tmp/yt-test/run.json | jq '.source_url'
```

**Expected output:**
- yt-dlp download progress
- Frame extraction progress
- Success with frame count

**Format:** Terminal recording → GIF
**Filename:** `docs/assets/youtube-extract.gif`
**Duration:** ~40 seconds (can speed up download phase)
**Size:** Keep under 5MB

---

## Priority 2: Screenshots (Quick to Create)

### [ ] Task 4: Doctor Output Screenshot
**What to capture:**
```bash
framescli doctor
```

**Expected output:**
Full doctor output showing all green checkmarks + GPU detection

**Format:** Screenshot → PNG
**Filename:** `docs/assets/doctor-output.png`
**Size:** Crop to terminal window only

---

### [ ] Task 5: GPU Detection Screenshot
**What to capture:**
```bash
framescli doctor
```

**Expected output:**
Focus on the "Hardware" section showing GPU detection

**Format:** Screenshot → PNG (crop to just hardware section)
**Filename:** `docs/assets/gpu-detection.png`

---

### [ ] Task 6: Extracted Frames Directory Screenshot
**What to capture:**
```bash
# After running an extraction
tree frames/Run_20260415-083045/ -L 2
# OR
ls -lah frames/Run_*/images/ | head -20
```

**Expected output:**
Directory structure showing:
- images/frame-0001.png
- images/frame-0002.png
- run.json
- frames.json

**Format:** Screenshot → PNG
**Filename:** `docs/assets/output-structure.png`

---

## Priority 3: PDF Guides (Optional if Time Permits)

### [ ] Task 7: "Install FramesCLI in 30 Seconds" PDF
**Content:**
1. Title slide: "Install FramesCLI in 30 Seconds"
2. Screenshot: Terminal with `brew install wraelen/tap/framescli`
3. Screenshot: `framescli doctor` output (all green)
4. Final slide: "Done. Now extract frames."

**Format:** PDF (4 slides)
**Filename:** `docs/guides/01-install-homebrew.pdf`
**Tool:** Google Slides or Keynote → Export PDF

---

### [ ] Task 8: "Ask Claude to Watch a Video" PDF
**Content:**
1. Title: "Ask Claude to Watch a Video"
2. Screenshot: Claude Desktop MCP settings with framescli config
3. Screenshot: Chat window - "Use framescli to watch video.mp4"
4. Screenshot: Claude's response referencing frames
5. Final: "Claude can now watch videos"

**Format:** PDF (5 slides)
**Filename:** `docs/guides/02-claude-watch-video.pdf`

---

### [ ] Task 9: "Extract from YouTube" PDF
**Content:**
1. Title: "Extract Frames from YouTube"
2. Screenshot: Terminal with URL extract command
3. Screenshot: Progress output
4. Screenshot: Extracted frames directory
5. Final: "Works with 1000+ sites via yt-dlp"

**Format:** PDF (5 slides)
**Filename:** `docs/guides/03-youtube-extraction.pdf`

---

## Asset Directory Structure

Create this structure:
```
docs/
  assets/           # GIFs and screenshots for README
    install-homebrew.gif
    extract-demo.gif
    youtube-extract.gif
    doctor-output.png
    gpu-detection.png
    output-structure.png
  guides/           # PDF guides
    01-install-homebrew.pdf
    02-claude-watch-video.pdf
    03-youtube-extraction.pdf
```

---

## Tools Needed

### Terminal Recording
- **asciinema** (recommended): `brew install asciinema`
- **agg** (for GIF conversion): `brew install agg`
- OR **vhs**: `brew install vhs`

### Screenshots
- macOS: Cmd+Shift+4 (select region)
- Linux: `gnome-screenshot` or `flameshot`

### PDF Creation
- Google Slides (web)
- Keynote (macOS)
- LibreOffice Impress (free)

### Image Optimization
- **ImageOptim** (macOS): Drag/drop to compress
- **pngquant**: `brew install pngquant`

---

## Recording Tips

### For Terminal GIFs:
1. **Clear terminal history first**: `clear`
2. **Use a clean prompt**: `export PS1="$ "`
3. **Type slowly** or use vhs script for consistent timing
4. **Keep window size consistent**: 80x24 or 120x30
5. **Speed up boring parts**: 1.5x or 2x in post

### For Screenshots:
1. **Crop to content**: Remove desktop clutter
2. **Use high contrast**: Light terminal theme or dark
3. **Readable font size**: 14pt minimum
4. **Compress before commit**: Keep under 500KB each

---

## Checklist Summary

**Must-Have (for README):**
- [ ] install-homebrew.gif
- [ ] extract-demo.gif
- [ ] youtube-extract.gif
- [ ] doctor-output.png

**Nice-to-Have:**
- [ ] gpu-detection.png
- [ ] output-structure.png

**Optional (if time permits):**
- [ ] PDF guides (all 3)

---

## How to Use This Checklist

1. Install tools first:
   ```bash
   brew install asciinema agg
   ```

2. Create asset directories:
   ```bash
   mkdir -p docs/assets docs/guides
   ```

3. Work through Priority 1 tasks (required for README)

4. Work through Priority 2 if time permits

5. Skip Priority 3 if running out of time (can add post-launch)

6. Commit assets:
   ```bash
   git add docs/assets/ docs/guides/
   git commit -m "docs: add visual assets for HN launch"
   ```

---

## Estimated Time

- **Priority 1 (GIFs):** 30-45 minutes
- **Priority 2 (Screenshots):** 10-15 minutes
- **Priority 3 (PDFs):** 30-45 minutes

**Total:** 1h 10m - 1h 45m

**Recommendation:** Focus on Priority 1 + 2, skip PDFs for now. Can add PDFs post-launch based on feedback.

---

Ready to start! Check off each task as you complete it.
