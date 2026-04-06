# FramesCLI Brand Assets

This folder is the single source of truth for logo and icon assets.

## Current Status

- `src/logo-temp-from-ai.svg` is a temporary logo source.
- It is usable for early previews, but it is not a true vector master.
- Before final branding, replace it with a real vector SVG (paths/shapes, no embedded raster image).

## Folder Layout

- `src/` editable source assets (SVG/AI/Figma exports)
- `exports/` production-ready files used by docs/apps

## Required Export Set

- `exports/logo-horizontal.svg`
- `exports/logo-icon.svg`
- `exports/logo-mono.svg`
- `exports/logo-dark-bg.svg`
- `exports/favicon-32.png`
- `exports/favicon-16.png`

## Rules

- Prefer pure vector SVG for all logo masters.
- Keep transparent background for default logo files.
- Keep icon legible at `16px`, `24px`, and `32px`.
- Keep one monochrome variant for badges/docs terminals.
- Do not overwrite previous approved assets without noting why in PR/commit message.
