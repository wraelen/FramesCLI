# README Media Guide

Use these filenames when adding curated product demos or screenshots:

- `docs/media/hero-cli.gif`
- `docs/media/hero-mcp.gif`
- `docs/media/cli-preview.png`
- `docs/media/mcp-session.png`

Keep media additions intentional. This directory should only carry assets that
directly support docs, release notes, or install/setup guidance.

## Capture Specs

- Aspect ratio: 16:9 preferred
- GIF width: `1200px` target (minimum `960px`)
- GIF length: `8-18s`
- Frame rate: `12-18fps`
- Keep terminal font large enough to read in GitHub preview
- Redact private data before export

## Suggested Capture Sequences

### CLI Hero (`hero-cli.gif`)

Record this flow:

1. `framescli doctor`
2. `framescli preview /path/to/video.mp4 --mode both --json`
3. `framescli extract /path/to/video.mp4 --json`
4. `framescli open-last --artifact run`

### MCP Hero (`hero-mcp.gif`)

Record this flow:

1. `framescli mcp` in one terminal pane
2. MCP client calling `preview` then `extract`
3. Show returned artifact paths and JSON result

### CLI Preview Screenshot (`cli-preview.png`)

Capture a readable terminal frame showing:

1. `framescli preview /path/to/video.mp4 --mode both --json`
2. the preview summary and JSON envelope together in one shot

## Optional Tools

- `asciinema` + `agg` (terminal-to-GIF pipeline)
- `peek` (Linux)
- `Kap` (macOS)
- `ScreenToGif` (Windows)

## Export Tips

- Crop tight around terminal/panes
- Avoid large cursor trails or rapid flicker
- Keep loops smooth (start/end state similar)
