#!/usr/bin/env python3
"""Render only this review round; this is not an application export pipeline.

Requires Python 3.10+, CairoSVG 2.9.1 and Pillow 13.0.0, plus system Cairo.
Run from any directory: python render_previews.py
"""
from io import BytesIO
from pathlib import Path
import json
import xml.etree.ElementTree as ET

import cairosvg
from PIL import Image

ROOT = Path(__file__).resolve().parent
DIRECTIONS = ("a-nook", "b-drawer", "c-tray", "d-perch")
THEMES = {
    "light": {"ink": "#24352e", "background": "#f5f3ed", "template": "#000000"},
    "dark": {"ink": "#f5f3ed", "background": "#24312c", "template": "#ffffff"},
}
SIZES = (1024, 128, 32, 16)


def raster(source: Path, size: int, ink: str, background: str | None) -> Image.Image:
    tree = ET.fromstring(source.read_text())
    tree.set("color", ink)
    result = cairosvg.svg2png(bytestring=ET.tostring(tree), output_width=size, output_height=size)
    image = Image.open(BytesIO(result)).convert("RGBA")
    if background:
        canvas = Image.new("RGBA", image.size, background)
        canvas.alpha_composite(image)
        return canvas.convert("RGB")
    return image


def main() -> None:
    checks = []
    for direction in DIRECTIONS:
        folder = ROOT / direction
        previews = folder / "previews"
        previews.mkdir(exist_ok=True)
        for theme, colors in THEMES.items():
            for size in SIZES:
                target = previews / f"{theme}-{size}.png"
                raster(folder / "source.svg", size, colors["ink"], colors["background"]).save(target)
                checks.append({"file": str(target.relative_to(ROOT)), "size": size, "background": colors["background"]})
            for scale in (1, 2):
                size = 16 * scale
                for mode in ("transparent", "context"):
                    background = colors["background"] if mode == "context" else None
                    target = previews / f"template-{theme}-{mode}-{scale}x.png"
                    raster(folder / "menu-template.svg", size, colors["template"], background).save(target)
                    checks.append({"file": str(target.relative_to(ROOT)), "size": size, "background": background})
    for check in checks:
        with Image.open(ROOT / check["file"]) as output:
            assert output.size == (check["size"], check["size"])
            if check["background"] is None:
                assert output.mode == "RGBA"
                assert output.getextrema()[3] == (0, 255), "Template must have transparent holes and opaque ink"
    (ROOT / "preview-manifest.json").write_text(json.dumps({"files": checks}, indent=2) + "\n")
    print(f"Rendered and checked {len(checks)} PNGs across {len(DIRECTIONS)} directions.")


if __name__ == "__main__":
    main()
