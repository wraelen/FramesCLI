# Brand Checklist

Use this checklist before publishing logo assets in README, releases, and package metadata.

## Must Pass

- [ ] `exports/logo-horizontal.svg` exists and has transparent background.
- [ ] `exports/logo-icon.svg` exists and reads clearly at `32px`.
- [ ] `exports/logo-mono.svg` exists and works on light and dark backgrounds.
- [ ] `exports/favicon-16.png` and `exports/favicon-32.png` exist.
- [ ] No exported SVG uses embedded base64 raster images.
- [ ] Wordmark spelling is exactly `FramesCLI`.
- [ ] Spacing between icon and wordmark is visually balanced.

## Visual QA

- [ ] Icon is still recognizable at `16px` and `24px`.
- [ ] Terminal cue/detail does not disappear at small size.
- [ ] Contrast is acceptable in GitHub light and dark themes.
- [ ] Logo does not look blurry in README preview.

## Repo Integration

- [ ] README hero or header references finalized brand assets.
- [ ] Any old placeholder logo references are removed.
- [ ] Release notes mention branding update if changed after prior release.
