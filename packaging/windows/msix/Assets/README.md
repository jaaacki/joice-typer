# MSIX Store Assets

Production PNGs derived from `assets/logo/joicetyper-logo-1024.png`
live in this directory and are committed. Regenerate them whenever the
master logo changes:

```bash
python3 scripts/release/generate_msix_assets.py
```

If a required PNG is somehow missing at packaging time,
`scripts/release/windows_package_msix.ps1` falls back to a placeholder
generated from `assets/windows/joicetyper.ico`. The fallback is for
emergencies only — Store certification expects the proper artwork.

## Required files (referenced by `AppxManifest.xml.tmpl`)

| File                     | Pixel size | Purpose                                  |
|--------------------------|------------|------------------------------------------|
| `Square44x44Logo.png`    | 44 × 44    | Taskbar icon, list views                 |
| `Square150x150Logo.png`  | 150 × 150  | Medium tile (Start menu default)         |
| `Wide310x150Logo.png`    | 310 × 150  | Wide tile                                |
| `StoreLogo.png`          | 50 × 50    | Store listing thumbnail, package logo    |
| `SplashScreen.png`       | 620 × 300  | First-launch splash                      |

## Recommended scale variants

For a polished Start menu / taskbar appearance, also provide scaled
variants (Microsoft's `MakeAppx` honours these via filename suffixes).
The minimum recommended set:

| Base file               | Required scales (filename suffix)                            |
|-------------------------|---------------------------------------------------------------|
| `Square44x44Logo.png`   | `.scale-100.png`, `.scale-150.png`, `.scale-200.png`, `.scale-400.png` |
| `Square150x150Logo.png` | `.scale-100.png`, `.scale-150.png`, `.scale-200.png`, `.scale-400.png` |
| `Wide310x150Logo.png`   | `.scale-100.png`, `.scale-150.png`, `.scale-200.png`, `.scale-400.png` |
| `StoreLogo.png`         | `.scale-100.png`, `.scale-150.png`, `.scale-200.png`, `.scale-400.png` |
| `SplashScreen.png`      | `.scale-100.png`, `.scale-150.png`, `.scale-200.png`, `.scale-400.png` |

You can also supply target-size variants for the start menu
(`Square44x44Logo.targetsize-16.png`, `-24`, `-32`, `-48`, `-256`,
`-256_altform-unplated.png` for a transparent variant). These are
optional but improve crispness at small sizes.

## Design guidance

- Use the same orange-on-dark palette as `assets/logo/` for brand
  consistency.
- The 44×44 icon must remain legible — avoid fine detail.
- Keep `BackgroundColor` in the manifest in sync with whatever colour
  surrounds the tile artwork (currently `transparent` for tiles and
  `#1F1F1F` for the splash screen).
