# Homebrew Notes

Canonical Homebrew documentation lives in [docs/HOMEBREW_SETUP.md](../docs/HOMEBREW_SETUP.md).

This directory is intentionally minimal. FramesCLI does not maintain a hand-edited
formula snapshot in the source repo anymore. Releases are published from
`.goreleaser.yml`, and the canonical formula is generated into:

- `wraelen/homebrew-tap`
- `Formula/framescli.rb`

For users, the install command remains:

```bash
brew install wraelen/tap/framescli
```

For maintainers, use the release flow in [docs/HOMEBREW_SETUP.md](../docs/HOMEBREW_SETUP.md).
