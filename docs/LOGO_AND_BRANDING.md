# Logo and Branding Updates

## Changes Made (2026-04-13)

### 1. MCP Registry Logo
**Selected:** Clapperboard icon (`brand/src/FramesCLI_logo_square.png`)
- **Location:** `brand/exports/mcp-icon-1024.png` (1024x1024)
- **Why:** Clean icon design, no wordmark, works great at small sizes
- **Use for:** MCP registry submission

### 2. README Dark Mode Fix
**Problem:** Logo-readme.svg was all black, invisible on GitHub dark mode
**Solution:** Added theme-aware `<picture>` element

**Before:**
```html
<img src="brand/exports/logo-readme.svg" alt="FramesCLI" width="320">
```

**After:**
```html
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="brand/exports/logo-dark-bg.svg">
  <source media="(prefers-color-scheme: light)" srcset="brand/exports/logo-readme.svg">
  <img src="brand/exports/logo-readme.svg" alt="FramesCLI" width="320">
</picture>
```

**Result:** Logo now automatically switches based on GitHub theme

## Logo Inventory

| File | Size | Use Case |
|------|------|----------|
| `mcp-icon-1024.png` | 1024x1024 | ✅ **MCP registry** |
| `logo-readme.svg` | Vector | README light mode |
| `logo-dark-bg.svg` | Vector | README dark mode |
| `logo-icon-color.png` | 250KB | General icon use |
| `FramesCLI_logo_square.png` | 1024x1024 | Master clapperboard icon |

## Recommendations

1. **For MCP Registry:** Use `mcp-icon-1024.png` (or resize to 512x512 if required)
2. **For Social Media:** Use clapperboard icon for profile pictures
3. **For Documentation:** Use wordmark logos (`logo-readme.svg`, `logo-horizontal.svg`)
4. **For Favicons:** Use existing `favicon-16.png` and `favicon-32.png`

## If You Need to Resize

**512x512 version (if MCP registry requires it):**
```bash
# Option 1: Python PIL
python3 -c "from PIL import Image; img = Image.open('brand/exports/mcp-icon-1024.png'); img.resize((512, 512), Image.LANCZOS).save('brand/exports/mcp-icon-512.png')"

# Option 2: Use any image editor
# - Open brand/exports/mcp-icon-1024.png
# - Resize to 512x512 (with LANCZOS/high-quality interpolation)
# - Save as brand/exports/mcp-icon-512.png
```

## GitHub Theme-Aware Images

GitHub supports automatic theme switching using the `<picture>` element:

```html
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="path/to/dark-version.svg">
  <source media="(prefers-color-scheme: light)" srcset="path/to/light-version.svg">
  <img src="path/to/fallback.svg" alt="Description">
</picture>
```

This works in:
- ✅ README.md
- ✅ GitHub Issues
- ✅ GitHub Discussions
- ✅ Wiki pages

## Next Steps

1. ✅ Clapperboard logo ready for MCP submission
2. ✅ README now works in dark mode
3. ⏳ Submit to MCP registry with `mcp-icon-1024.png`
4. ⏳ Test README appearance on GitHub (both themes)
