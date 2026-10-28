#!/usr/bin/env python3
"""Render this review round, not application release assets.

Requires Python 3.10+, CairoSVG 2.9.1, Pillow 13.0.0 and system Cairo.
"""
from io import BytesIO
from pathlib import Path
import json
import xml.etree.ElementTree as ET

import cairosvg
from PIL import Image

ROOT = Path(__file__).resolve().parent
DIRECTIONS = ("a-box-cat", "b-side-eye", "c-coil-cat", "d-sitting-cat")
SIZES = (1024, 128, 32, 16)
THEMES = {
    "light": {"ink": "#000000", "background": "#F5F2EB"},
    "dark": {"ink": "#FFFFFF", "background": "#232A2C"},
}


def raster(source: Path, size: int, ink: str | None, background: str | None) -> Image.Image:
    tree = ET.fromstring(source.read_text())
    if size == 16 and source.name in ("source.svg", "mono.svg"):
        # The editable menu study is also the optical master for 16px artwork.
        original = tree
        tree = ET.fromstring((source.parent / "menu-template.svg").read_text())
        tree.set("color", original.attrib["color"])
        if source.name == "source.svg":
            backdrop = original.find("{http://www.w3.org/2000/svg}rect")
            assert backdrop is not None
            tree.insert(0, backdrop)
    if ink:
        tree.set("color", ink)
    result = cairosvg.svg2png(bytestring=ET.tostring(tree), output_width=size, output_height=size)
    output = Image.open(BytesIO(result)).convert("RGBA")
    if background:
        canvas = Image.new("RGBA", output.size, background)
        canvas.alpha_composite(output)
        return canvas.convert("RGB")
    return output


def main() -> None:
    manifest = []
    for direction in DIRECTIONS:
        folder = ROOT / direction
        output = folder / "previews"
        output.mkdir(exist_ok=True)

        def save(source: str, name: str, size: int, ink: str | None, background: str | None, kind: str) -> None:
            target = output / name
            raster(folder / source, size, ink, background).save(target)
            manifest.append({"file": str(target.relative_to(ROOT)), "size": size, "kind": kind, "geometry": "menu-template.svg" if size == 16 else source})

        for size in SIZES:
            save("source.svg", f"color-{size}.png", size, None, None, "full-bleed-color")
        for theme, colors in THEMES.items():
            for size in SIZES:
                save("mono.svg", f"mono-{theme}-{size}.png", size, colors["ink"], colors["background"], "monochrome")
            for scale in (1, 2):
                for context in ("transparent", "context"):
                    background = colors["background"] if context == "context" else None
                    save("menu-template.svg", f"template-{theme}-{context}-{scale}x.png", 16 * scale,
                         colors["ink"], background, f"menu-{context}")
    for entry in manifest:
        with Image.open(ROOT / entry["file"]) as image:
            assert image.size == (entry["size"], entry["size"])
            if entry["kind"] == "menu-transparent":
                assert image.mode == "RGBA" and image.getextrema()[3] == (0, 255)
            if entry["kind"] == "full-bleed-color":
                assert image.getextrema()[3] == (255, 255), "Color backgrounds must be full bleed"
    (ROOT / "preview-manifest.json").write_text(json.dumps({"files": manifest}, indent=2) + "\n")
    print(f"Rendered and checked {len(manifest)} PNGs: color, monochrome, and 16pt template studies.")


if __name__ == "__main__":
    main()
