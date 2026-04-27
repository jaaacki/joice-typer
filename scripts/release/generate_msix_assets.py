#!/usr/bin/env python3
"""Generate Microsoft Store MSIX assets from the master logo PNG.

Reads ``assets/logo/joicetyper-logo-1024.png`` and writes the five
required Store PNGs into ``packaging/windows/msix/Assets/``:

- ``StoreLogo.png``         (50 x 50)
- ``Square44x44Logo.png``   (44 x 44)
- ``Square150x150Logo.png`` (150 x 150)
- ``Wide310x150Logo.png``   (310 x 150, logo centered on transparent canvas)
- ``SplashScreen.png``      (620 x 300, logo centered on dark canvas)

Square assets are produced with high-quality LANCZOS downsampling and a
small transparent margin so the glyph does not bleed to the edge.

The generated files are intended to be committed. Run this script
whenever the master logo changes.
"""

from __future__ import annotations

import sys
from pathlib import Path

from PIL import Image

REPO_ROOT = Path(__file__).resolve().parents[2]
SOURCE_LOGO = REPO_ROOT / "assets" / "logo" / "joicetyper-logo-1024.png"
ASSETS_DIR = REPO_ROOT / "packaging" / "windows" / "msix" / "Assets"

# Splash screen background color must match AppxManifest.xml.tmpl
# `<uap:SplashScreen BackgroundColor="...">`.
SPLASH_BG = (0x1F, 0x1F, 0x1F, 0xFF)

# Fraction of the canvas the centered logo occupies for non-square tiles.
# Values were tuned for a square brand mark on a wide canvas.
WIDE_LOGO_HEIGHT_FRACTION = 0.85
SPLASH_LOGO_HEIGHT_FRACTION = 0.55


def load_source() -> Image.Image:
    if not SOURCE_LOGO.exists():
        sys.exit(f"fatal: source logo missing: {SOURCE_LOGO}")
    img = Image.open(SOURCE_LOGO).convert("RGBA")
    if img.size[0] != img.size[1]:
        sys.exit(
            f"fatal: source logo must be square (got {img.size}); "
            "the centering math assumes a square master."
        )
    return img


def square(source: Image.Image, size: int, margin_fraction: float = 0.0) -> Image.Image:
    """Return ``source`` resampled to a transparent ``size`` x ``size`` canvas."""
    inner = max(1, int(round(size * (1.0 - 2.0 * margin_fraction))))
    resized = source.resize((inner, inner), Image.LANCZOS)
    canvas = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    offset = (size - inner) // 2
    canvas.paste(resized, (offset, offset), resized)
    return canvas


def wide(
    source: Image.Image,
    width: int,
    height: int,
    background: tuple[int, int, int, int],
    logo_height_fraction: float,
) -> Image.Image:
    canvas = Image.new("RGBA", (width, height), background)
    logo_size = max(1, int(round(height * logo_height_fraction)))
    resized = source.resize((logo_size, logo_size), Image.LANCZOS)
    offset = ((width - logo_size) // 2, (height - logo_size) // 2)
    canvas.paste(resized, offset, resized)
    return canvas


def write(image: Image.Image, name: str) -> Path:
    ASSETS_DIR.mkdir(parents=True, exist_ok=True)
    out = ASSETS_DIR / name
    image.save(out, format="PNG", optimize=True)
    return out


def main() -> int:
    source = load_source()
    transparent = (0, 0, 0, 0)

    targets = [
        ("StoreLogo.png", square(source, 50, margin_fraction=0.05)),
        ("Square44x44Logo.png", square(source, 44, margin_fraction=0.05)),
        ("Square150x150Logo.png", square(source, 150, margin_fraction=0.10)),
        (
            "Wide310x150Logo.png",
            wide(source, 310, 150, transparent, WIDE_LOGO_HEIGHT_FRACTION),
        ),
        (
            "SplashScreen.png",
            wide(source, 620, 300, SPLASH_BG, SPLASH_LOGO_HEIGHT_FRACTION),
        ),
    ]

    for name, image in targets:
        path = write(image, name)
        print(f"wrote {path.relative_to(REPO_ROOT)} ({image.size[0]}x{image.size[1]})")

    return 0


if __name__ == "__main__":
    sys.exit(main())
