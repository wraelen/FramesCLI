# Homebrew Distribution Setup

This document explains how FramesCLI's Homebrew distribution works and how to publish releases.

## Overview

FramesCLI uses **goreleaser** to automatically generate and publish Homebrew formulas. When you create a new GitHub release, goreleaser will:

1. Build binaries for macOS (Intel + Apple Silicon), Linux, and Windows
2. Create release archives with checksums
3. Generate a Homebrew formula
4. Push the formula to `wraelen/homebrew-tap`

## One-Time Setup

### 1. Create Homebrew Tap Repository

Create a new GitHub repository called `homebrew-tap`:

```bash
# On GitHub, create: wraelen/homebrew-tap
# Keep it public
# Initialize with a README
```

### 2. Set GitHub Token

Goreleaser needs a GitHub token to push the formula. Add this to your environment or GitHub Actions:

```bash
export GITHUB_TOKEN=ghp_your_token_here
```

**For GitHub Actions**, add the token as a secret:
- Go to Settings → Secrets and variables → Actions
- Add `GITHUB_TOKEN` with a Personal Access Token that has `repo` and `workflow` permissions

## Publishing a Release

### Method 1: GitHub Actions (Recommended)

We'll set up a GitHub Actions workflow that automatically runs goreleaser on new tags.

**Coming soon:** See `.github/workflows/release.yml` (not yet created)

### Method 2: Manual Release

```bash
# 1. Create and push a tag
git tag -a v0.2.2 -m "Release v0.2.2"
git push origin v0.2.2

# 2. Run goreleaser (requires GITHUB_TOKEN env var)
export GITHUB_TOKEN=ghp_your_token_here
goreleaser release --clean

# This will:
# - Build binaries
# - Create GitHub release
# - Push Homebrew formula to wraelen/homebrew-tap
```

### Method 3: Test Locally (Dry Run)

```bash
# Test without publishing
goreleaser release --snapshot --clean
```

## User Installation

Once published, users can install via:

```bash
# Add the tap
brew tap wraelen/tap

# Install framescli
brew install framescli

# Or in one command
brew install wraelen/tap/framescli
```

## Homebrew Formula Location

The formula will be published to:
- Repository: `https://github.com/wraelen/homebrew-tap`
- File: `Formula/framescli.rb`

Users can also install from the generated formula directly:
```bash
brew install https://raw.githubusercontent.com/wraelen/homebrew-tap/main/Formula/framescli.rb
```

## Updating the Formula

Goreleaser automatically updates the formula on each release. You don't need to manually edit the formula file.

To update:
1. Create a new git tag (e.g., `v0.2.3`)
2. Run `goreleaser release`
3. Formula is automatically updated with new version and checksums

## Dependencies

The formula declares:
- **Required**: `ffmpeg` (for frame extraction)
- **Optional**: `yt-dlp` (for URL downloads)

Users will see a caveat message after installation guiding them to install optional dependencies.

## Submitting to homebrew-core (Future)

Once FramesCLI has:
- 75+ GitHub stars
- 30+ forks
- Stable release history
- Active maintenance

We can submit to `homebrew-core` for inclusion in the main Homebrew repository:

```bash
# Users would then install with just:
brew install framescli  # No tap needed
```

Submission process: https://docs.brew.sh/Adding-Software-to-Homebrew

## Troubleshooting

**Formula not found:**
- Ensure `homebrew-tap` repository exists and is public
- Check that goreleaser successfully pushed the formula (check GitHub Actions logs)
- Run `brew update` to refresh Homebrew's repository cache

**Installation fails:**
- Check that the release archives exist on GitHub releases page
- Verify checksums match in the formula and release assets
- Test with `brew install --debug wraelen/tap/framescli`

## References

- [Goreleaser Homebrew Documentation](https://goreleaser.com/customization/homebrew/)
- [Homebrew Tap Documentation](https://docs.brew.sh/Taps)
- [Homebrew Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)
