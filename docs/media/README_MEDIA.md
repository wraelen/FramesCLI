# README Media Guide

Use these filenames when adding recorded product demos:

- `docs/media/hero-tui.gif`
- `docs/media/hero-mcp.gif`
- `docs/media/tui-overview.png`
- `docs/media/mcp-session.png`

Current repo still includes static SVG placeholder artwork for planning/demo layout:

- `docs/media/hero-tui.svg`
- `docs/media/hero-mcp.svg`

## Capture Specs

- Aspect ratio: 16:9 preferred
- GIF width: `1200px` target (minimum `960px`)
- GIF length: `8-18s`
- Frame rate: `12-18fps`
- Keep terminal font large enough to read in GitHub preview
- Redact private data before export

## Suggested Capture Sequences

### TUI Hero (`hero-tui.gif`)

Record this flow:

1. `framescli tui`
2. Import a video
3. Move through wizard steps
4. Show queue run progress and result actions

### MCP Hero (`hero-mcp.gif`)

Record this flow:

1. `framescli mcp` in one terminal pane
2. MCP client calling `preview` then `extract`
3. Show returned artifact paths / JSON result

## Optional Tools

- `asciinema` + `agg` (terminal-to-GIF pipeline)
- `peek` (Linux)
- `Kap` (macOS)
- `ScreenToGif` (Windows)

## Export Tips

- Crop tight around terminal/panes
- Avoid large cursor trails or rapid flicker
- Keep loops smooth (start/end state similar)
