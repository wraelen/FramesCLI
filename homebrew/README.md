# Homebrew Tap for FramesCLI

This directory contains the Homebrew formula for FramesCLI.

## For Users

### Install via tap:

```bash
brew tap wraelen/framescli
brew install framescli
```

### Or install directly:

```bash
brew install wraelen/framescli/framescli
```

## For Maintainers

### Setting up the tap repository

1. **Create a new GitHub repo named `homebrew-framescli`** (must start with `homebrew-`)

2. **Copy the formula:**
   ```bash
   cp homebrew/framescli.rb <path-to-homebrew-framescli-repo>/Formula/framescli.rb
   ```

3. **Update SHA256 checksums after each release:**

   After publishing a GitHub release, download the checksums.txt and update the formula:

   ```bash
   # Download checksums from release
   curl -sL https://github.com/wraelen/framescli/releases/download/v0.1.0/checksums.txt

   # Update formula with actual SHA256 values for each platform
   ```

4. **Push to tap repo:**
   ```bash
   cd <path-to-homebrew-framescli-repo>
   git add Formula/framescli.rb
   git commit -m "Add framescli v0.1.0"
   git push
   ```

### Updating the formula for new releases

```bash
# 1. Update version in framescli.rb
# 2. Update URLs to point to new release tag
# 3. Download new release artifacts and update SHA256 checksums
# 4. Test locally:
brew uninstall framescli
brew install --build-from-source wraelen/framescli/framescli

# 5. If tests pass, commit and push
git commit -am "Update framescli to vX.Y.Z"
git push
```

### Updating SHA256 checksums

```bash
# Generate SHA256 for each platform from checksums.txt:
grep "darwin_amd64.tar.gz" checksums.txt
grep "darwin_arm64.tar.gz" checksums.txt
grep "linux_amd64.tar.gz" checksums.txt
grep "linux_arm64.tar.gz" checksums.txt

# Replace REPLACE_WITH_ACTUAL_SHA256_* placeholders in formula
```

## Formula Structure

The formula handles:
- ✅ Multi-platform support (macOS Intel, macOS Apple Silicon, Linux AMD64, Linux ARM64)
- ✅ ffmpeg dependency declaration
- ✅ Shell completion generation
- ✅ Post-install caveats for whisper setup
- ✅ Basic smoke tests

## Testing

```bash
# Test formula locally
brew install --build-from-source ./homebrew/framescli.rb

# Run tests
brew test framescli

# Audit formula
brew audit --strict framescli
```

## Future: Homebrew Core Submission

Once the tap is stable and the project has demonstrated usage (100+ stars, active maintenance), consider submitting to Homebrew core:

1. Fork `Homebrew/homebrew-core`
2. Add formula to `Formula/f/framescli.rb`
3. Submit PR with formula

Benefits of core inclusion:
- No `tap` command needed
- Shows up in `brew search`
- Higher visibility

Requirements for core:
- Stable project with regular releases
- Active maintenance
- No known security issues
- Passes `brew audit --strict --online`
