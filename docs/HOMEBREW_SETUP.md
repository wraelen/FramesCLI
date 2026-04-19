# FramesCLI Homebrew Release Guide

This is the canonical Homebrew document for FramesCLI. The short note in
`homebrew/README.md` exists only to point here.

## Current Model

FramesCLI does not keep a hand-maintained formula in this repo. The release flow
is:

1. Tag a new release in `wraelen/FramesCLI`
2. GitHub Actions runs `.github/workflows/release.yml`
3. GoReleaser builds archives, publishes the GitHub release, and generates the
   Homebrew formula from `.goreleaser.yml`
4. The formula is pushed to `wraelen/homebrew-tap` at `Formula/framescli.rb`
5. The workflow verifies that the tap formula version matches the tag before
   reporting success

User install command:

```bash
brew install wraelen/tap/framescli
```

Tap repo:

- Source: `https://github.com/wraelen/homebrew-tap`
- Formula path: `Formula/framescli.rb`
- Tap alias: `wraelen/tap`

## Release Prerequisites

Before cutting a release, make sure all of these are true:

- `go test ./...` is green
- `CHANGELOG.md` has an entry for the new version
- `.github/workflows/release.yml` still matches the intended release behavior
- `.goreleaser.yml` still points at `wraelen/homebrew-tap` and writes the
  formula under `Formula/`
- GitHub Actions has `HOMEBREW_TAP_TOKEN` configured with `contents:write` on
  `wraelen/homebrew-tap`

Without `HOMEBREW_TAP_TOKEN`, the GitHub release can succeed while the Homebrew
formula push fails. The workflow now explicitly checks the tap version to catch
that failure mode.

## Cutting a Release

Recommended sequence for a patch release:

```bash
go test ./...
git push origin main
git tag -a v0.2.6 -m "FramesCLI v0.2.6"
git push origin v0.2.6
```

What happens after the tag push:

- GitHub Actions runs the `Release` workflow
- The workflow runs the full Go test suite
- GoReleaser publishes release archives plus `checksums.txt`
- GoReleaser updates `wraelen/homebrew-tap/Formula/framescli.rb`
- The workflow fetches the published formula from the tap and fails if its
  `version` does not match the tag

## Local Dry Run

Use a snapshot build before tagging if you want to validate the release layout
locally:

```bash
export HOMEBREW_TAP_TOKEN=dummy
goreleaser release --snapshot --clean
./scripts/release-verify.sh --source dist --dist-dir ./dist
```

Notes:

- `--snapshot` does not publish a GitHub release or update the tap
- The placeholder token is only there because the GoReleaser brew stanza still
  expects the environment variable to exist
- `scripts/release-verify.sh` validates archive names, checksums, README/LICENSE
  inclusion, and installer URL resolution

## Post-Release Verification

After the GitHub Actions release finishes:

1. Check the GitHub release page for the new tag
2. Run:

```bash
./scripts/release-verify.sh --source github --version v0.2.6
```

3. Confirm the tap formula reports the same version:

```bash
curl -fsSL https://raw.githubusercontent.com/wraelen/homebrew-tap/main/Formula/framescli.rb | grep -E '^\s*version\s+"'
```

4. Sanity-check Homebrew install on a clean machine or VM:

```bash
brew install wraelen/tap/framescli
framescli --version
framescli doctor
```

## Dependencies and Caveats

The generated formula declares:

- Required: `ffmpeg`
- Optional: `yt-dlp`

Transcription backends remain separate Python installs:

```bash
pip install openai-whisper
# or
pip install faster-whisper
```

## Troubleshooting

### Tap formula did not update

Check:

- `HOMEBREW_TAP_TOKEN` is still present in GitHub Actions secrets
- `.goreleaser.yml` still points at `wraelen/homebrew-tap`
- The workflow log for the `Run GoReleaser` step
- The `Verify Homebrew tap was bumped` step in `.github/workflows/release.yml`

If the release assets were published but the tap did not move, fix the root
cause and cut a new patch release. Do not hand-edit a formula in this repo to
paper over a broken release.

### `brew install wraelen/tap/framescli` still sees an older version

Run:

```bash
brew update
brew untap wraelen/tap && brew tap wraelen/tap
```

Then re-check the formula version from the raw tap URL.

## References

- `.goreleaser.yml`
- `.github/workflows/release.yml`
- `scripts/release-verify.sh`
- [GoReleaser Homebrew docs](https://goreleaser.com/customization/homebrew/)
