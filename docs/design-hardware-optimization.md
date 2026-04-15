# Design: Hardware-Aware Defaults & Smart Artifact Organization

**Status:** DRAFT
**Created:** 2026-04-15
**For:** v0.2.1 polish before movie benchmark

## Problem Statement

FramesCLI v0.2.0 has URL ingestion working, but two critical UX issues prevent it from being "genuinely amazing":

1. **Hardware capabilities are detected but not used** - Users with GPUs get CPU-only extraction because hwaccel defaults to "none"
2. **Artifact folders are cryptic** - "Monday_3-22pm" doesn't indicate what video was extracted or where it came from

Before running the movie benchmark, we need FramesCLI to:
- Automatically suggest optimal settings based on detected hardware
- Gracefully guide users toward faster configurations
- Create artifact folders that are human-readable and AI-agent-friendly

## Design Principles

1. **Graceful degradation** - Suggest best settings, but never break if hardware unavailable
2. **Explain, don't assume** - Tell users WHY a setting was chosen
3. **Make URLs first-class** - Artifact folders should show video title, not timestamp
4. **Optimize for browsing** - Humans and AI agents should find videos easily

---

## Part 1: Hardware-Aware Recommendations

### 1.1 Enhanced GPU Detection

**Expand detection beyond NVIDIA:**

```go
type GPUInfo struct {
    Available bool
    Vendor    string // "nvidia", "amd", "intel", "apple", "none"
    Model     string // "RTX 4090", "Radeon RX 7900", etc
    HWAccel   string // Recommended hwaccel mode: "cuda", "vaapi", "qsv", "videotoolbox"
}

func detectGPU() GPUInfo {
    // Check NVIDIA
    if hasNVIDIA() {
        return GPUInfo{
            Available: true,
            Vendor:    "nvidia",
            Model:     getNVIDIAModel(),
            HWAccel:   "cuda",
        }
    }

    // Check AMD via ROCm or VAAPI
    if hasAMD() {
        return GPUInfo{
            Available: true,
            Vendor:    "amd",
            Model:     getAMDModel(),
            HWAccel:   "vaapi",
        }
    }

    // Check Intel QuickSync
    if hasIntelQSV() {
        return GPUInfo{
            Available: true,
            Vendor:    "intel",
            Model:     "QuickSync",
            HWAccel:   "qsv",
        }
    }

    // Check Apple Silicon
    if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
        return GPUInfo{
            Available: true,
            Vendor:    "apple",
            Model:     "M-series",
            HWAccel:   "videotoolbox",
        }
    }

    return GPUInfo{Available: false, Vendor: "none", HWAccel: "none"}
}
```

### 1.2 Smart Config Defaults

**On first run or `framescli init`, detect hardware and suggest config:**

```bash
$ framescli init
Detecting hardware capabilities...

✓ GPU detected: NVIDIA GeForce RTX 4090
✓ FFmpeg hwaccel support: cuda, vaapi, qsv
✓ Transcription: faster-whisper available (GPU-accelerated)

Recommended configuration:
  hwaccel: cuda               (15-30x faster frame extraction)
  preset: high-fidelity       (your hardware can handle it)
  transcribe_backend: faster-whisper
  whisper_model: medium       (good accuracy, fast on GPU)

Save this config to ~/.config/framescli/config.json? [Y/n]
```

**If user has no GPU:**

```bash
$ framescli init
Detecting hardware capabilities...

⚠ No GPU detected
✓ FFmpeg available (CPU-only)
⚠ Whisper: CPU-only (transcription will be slow)

Recommended configuration:
  hwaccel: none
  preset: laptop-safe         (optimized for CPU-only)
  transcribe_backend: auto
  whisper_model: base         (faster on CPU)

Save this config to ~/.config/framescli/config.json? [Y/n]
```

### 1.3 Runtime Preset Suggestions

**When user runs extraction without specifying preset:**

```bash
$ framescli extract video.mp4

Hardware check: NVIDIA RTX 4090 detected
Recommended: Add --preset high-fidelity --hwaccel cuda for 20x faster extraction
(or run 'framescli init' to set defaults)

[1/4] Resolving video input
...
```

**If user explicitly uses laptop-safe on a powerful machine:**

```bash
$ framescli extract video.mp4 --preset laptop-safe

Note: You have GPU available (NVIDIA RTX 4090)
Consider --preset balanced or --preset high-fidelity with --hwaccel cuda
(suppress with --quiet)

[1/4] Resolving video input
...
```

### 1.4 Enhanced Doctor Command

**Add recommendations section:**

```bash
$ framescli doctor

Tools
[ok]   ffmpeg
[ok]   yt-dlp

Hardware
GPU:               NVIDIA GeForce RTX 4090
CUDA Available:    yes
Recommended:       hwaccel: cuda, preset: high-fidelity

Transcription
Backend:           faster-whisper (GPU-accelerated)
Model:             base
Estimated Speed:   ~10x realtime (GPU)

Recommendations
→ Enable GPU acceleration: framescli prefs set hwaccel cuda
→ Upgrade to medium model for better accuracy: WHISPER_MODEL=medium
→ Use high-fidelity preset for best quality on your hardware
```

---

## Part 2: Smart Artifact Organization

### 2.1 Descriptive Folder Names

**Current:** `frames/Monday_3-22pm/`
**Proposed:** `frames/youtube-LPZh9BOjkQs-TheVideoTitle/`

**Folder Naming Rules:**

1. **For URL extractions:**
   - Format: `{source}-{identifier}-{sanitized-title}/`
   - Example: `youtube-dQw4w9WgXcQ-RickAstleyNeverGonnaGiveYouUp/`
   - Example: `vimeo-123456789-ConferenceTalkTitle/`
   - Example: `archive-NOTLD1968-NightOfTheLivingDead/`

2. **For local files:**
   - Format: `{filename-without-ext}-{timestamp}/`
   - Example: `meeting-recording-20260415-1422/`
   - Example: `screen-capture-Monday_3-22pm/` (current behavior as fallback)

3. **Collision handling:**
   - Append `-run2`, `-run3` instead of `-2`, `-3`
   - Makes it clear these are multiple extractions of the same video

**Title Sanitization:**
```go
func sanitizeTitle(title string) string {
    // Remove special chars, limit length, preserve readability
    // "My Video: The Best Tutorial (2024)" → "MyVideoTheBestTutorial2024"
    // Limit to 50 chars to avoid filesystem issues
}
```

### 2.2 Enhanced run.json Metadata

**Add source context:**

```json
{
  "video_path": "/home/user/.cache/framescli/videos/0c7b3df...mp4",
  "source_type": "url",
  "source_url": "https://www.youtube.com/watch?v=LPZh9BOjkQs",
  "source_title": "The Original Video Title from yt-dlp",
  "source_domain": "youtube.com",
  "cached": true,
  "cache_hit": true,
  "run_name": "youtube-LPZh9BOjkQs-TheOriginalVideoTitle",
  "fps": 1,
  "frame_format": "jpg",
  "duration_sec": 477.518367,
  "width": 3840,
  "height": 2160,
  "source_fps": 30,
  "preset": "laptop-safe",
  "hwaccel": "none"
}
```

**For local files:**

```json
{
  "video_path": "/mnt/c/Users/wraelen/Videos/meeting.mp4",
  "source_type": "file",
  "source_filename": "meeting.mp4",
  "run_name": "meeting-20260415-1422",
  ...
}
```

### 2.3 Artifact Index with Rich Metadata

**Update `frames/index.json` to include source info:**

```json
{
  "runs": [
    {
      "dir": "youtube-LPZh9BOjkQs-TheOriginalVideoTitle",
      "created_at": "2026-04-15T04:26:46Z",
      "source_type": "url",
      "source_url": "https://www.youtube.com/watch?v=LPZh9BOjkQs",
      "source_title": "The Original Video Title",
      "duration_sec": 477.5,
      "fps": 1,
      "frames": 477,
      "format": "jpg",
      "transcribed": false,
      "artifacts": {
        "run_json": "youtube-LPZh9BOjkQs-TheOriginalVideoTitle/run.json",
        "frames_json": "youtube-LPZh9BOjkQs-TheOriginalVideoTitle/frames.json",
        "images_dir": "youtube-LPZh9BOjkQs-TheOriginalVideoTitle/images/"
      }
    }
  ]
}
```

**Benefits:**
- AI agents can browse index and find videos by title/URL
- Humans can `ls frames/` and immediately understand contents
- Search becomes trivial: `grep -r "youtube" frames/index.json`

---

## Part 3: Implementation Plan

### Phase 1: Enhanced Hardware Detection (2 hours)
- [ ] Expand `detectGPU()` to support AMD, Intel, Apple
- [ ] Add `GPUInfo` struct with vendor/model/hwaccel recommendation
- [ ] Update `doctor` command to show GPU details
- [ ] Test on NVIDIA, AMD, Apple Silicon, CPU-only systems

### Phase 2: Smart Defaults & Recommendations (3 hours)
- [ ] Create `framescli init` command
- [ ] Add hardware-based config suggestions
- [ ] Show runtime hints when suboptimal preset used
- [ ] Add recommendations section to `doctor` output
- [ ] Make recommendations suppressible with `--quiet` flag

### Phase 3: Descriptive Artifact Folders (3 hours)
- [ ] Update `generateRunName()` to use URL metadata
- [ ] Extract video title from yt-dlp `VideoMetadata`
- [ ] Implement title sanitization (50 char limit, alphanumeric + dash)
- [ ] Update collision handling to use `-run2` suffix
- [ ] Add `source_type`, `source_url`, `source_title` to run.json
- [ ] Update artifact index to include source metadata

### Phase 4: Documentation (1 hour)
- [ ] Add "Hardware Optimization Guide" to README
- [ ] Document `framescli init` command
- [ ] Update MCP docs with new run.json schema
- [ ] Add troubleshooting for GPU detection issues

---

## Success Criteria

**Functional:**
- ✅ `framescli init` detects GPU and suggests optimal config
- ✅ `framescli doctor` shows clear hardware recommendations
- ✅ URL extractions create folders with video titles
- ✅ Artifact index includes source metadata for easy browsing

**User Experience:**
- ✅ User with RTX 4090 sees "Add --hwaccel cuda for 20x speed" hint
- ✅ User with no GPU sees "CPU-only, use laptop-safe preset" hint
- ✅ User extracts YouTube video → folder named `youtube-{id}-{title}/`
- ✅ User runs `ls frames/` and immediately understands what's there

**Quality:**
- ✅ Works gracefully when GPU unavailable (no errors, just hints)
- ✅ Handles missing yt-dlp metadata (fallback to hash-based names)
- ✅ Filesystem-safe folder names (no special chars, length limits)
- ✅ Backward compatible with existing artifact structure

---

## Open Questions

1. **Should `framescli init` be mandatory on first run?**
   → Recommend: No, but show a one-time hint on first `extract` command

2. **Should we auto-migrate existing `Monday_3-22pm` folders?**
   → Recommend: No, too risky. New extractions use new names, old runs stay as-is

3. **How to handle very long video titles (>100 chars)?**
   → Recommend: Truncate to 50 chars, append ellipsis if truncated

4. **Should folder names include resolution/fps?**
   → Recommend: No, too verbose. Metadata is in run.json

---

## Example: Before vs After

### Before (v0.2.0)

```bash
$ framescli extract --url "https://youtube.com/watch?v=dQw4w9WgXcQ"

[1/5] Downloading video from URL
[2/5] Resolving video input
[3/5] Probing video metadata
[4/5] Extracting frames (this may take a while)
[5/5] Finalizing artifacts

Output: frames/Monday_3-22pm/
```

**Issues:**
- No hardware hints
- Folder name is meaningless
- User has no idea if GPU could be used

### After (v0.2.1)

```bash
$ framescli extract --url "https://youtube.com/watch?v=dQw4w9WgXcQ"

Hardware: NVIDIA RTX 4090 detected
Tip: Add --hwaccel cuda --preset high-fidelity for 20x faster extraction
(or run 'framescli init' to set defaults)

[1/5] Downloading video from URL
Fetching metadata from youtube.com...
Title: "Rick Astley - Never Gonna Give You Up (Official Video)"

[2/5] Resolving video input (cached)
[3/5] Probing video metadata (4K, 3m32s, 30fps)
[4/5] Extracting frames at 1fps (CPU-only)
[5/5] Finalizing artifacts

Output: frames/youtube-dQw4w9WgXcQ-RickAstleyNeverGonnaGiveYouUp/

Performance note: Extraction took 2m14s. With --hwaccel cuda, estimated <10s.
Run 'framescli doctor' to see hardware recommendations.
```

**Improvements:**
- ✅ User knows GPU is available
- ✅ Folder name shows video title
- ✅ Performance comparison shown
- ✅ Clear path to optimization

---

## Dependencies

**New:**
- Title sanitization logic (regex, length limits)
- GPU vendor detection (AMD, Intel, Apple)

**Modified:**
- `generateRunName()` - use URL metadata
- `doctor` command - add recommendations section
- `extract` command - show hardware hints
- `run.json` schema - add source metadata

**Risks:**
- GPU detection may fail on edge cases (WSL, Docker)
  → Mitigation: Graceful fallback to "none" with explanation
- Very long video titles may hit filesystem limits
  → Mitigation: 50-char truncation with ellipsis
- Existing scripts may parse folder names
  → Mitigation: Only affect NEW extractions, old folders unchanged

---

## Ship Criteria

Before movie benchmark:
1. ✅ GPU detection works on test systems (NVIDIA, AMD, CPU-only)
2. ✅ `framescli init` creates optimized config
3. ✅ `framescli doctor` shows actionable recommendations
4. ✅ URL extractions create descriptive folder names
5. ✅ Documentation updated with hardware optimization guide

This ensures the movie benchmark showcases FramesCLI at its best performance,
and the resulting artifacts are easily discoverable and shareable.
