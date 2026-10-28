#!/usr/bin/env python3
"""Build R03 review images, vector overlays, and junction details.

Requires Python 3.10+, Pillow 13.0.0, Node.js and @resvg/resvg-js 2.6.2.
Keep dependencies outside the repository. The Node module must be resolvable.
"""
from copy import deepcopy
from pathlib import Path
import json
import re
import subprocess
import xml.etree.ElementTree as ET

from PIL import Image

ROOT = Path(__file__).resolve().parent
BASELINE = ROOT.parent / 'R02-2026-10-28/a-box-cat'
VARIANTS = ('a1-head', 'a2-layer', 'a3-combined')
THEMES = {'light': ('#000000', '#F5F2EB'), 'dark': ('#FFFFFF', '#232A2C')}
NS = '{http://www.w3.org/2000/svg}'
ET.register_namespace('', NS[1:-1])
JOBS = []


def load(folder, name='source.svg'):
    return ET.fromstring((folder / name).read_text())


def add_job(file, tree, width, kind, height=None):
    JOBS.append({'file': file, 'svg': ET.tostring(tree, encoding='unicode'),
                 'width': width, 'height': height or width, 'kind': kind})


def backdrop(tree, color):
    node = ET.Element(NS + 'rect', {'width': '128', 'height': '128', 'fill': color})
    tree.insert(0, node)


def styled(folder, name, size, ink=None, background=None):
    original = load(folder, name)
    tree = original
    if size == 16 and name in ('source.svg', 'mono.svg'):
        tree = load(folder, 'menu-template.svg')
        tree.set('color', original.attrib['color'])
        if name == 'source.svg':
            tree.insert(0, deepcopy(original.find(NS + 'rect')))
    if ink:
        tree.set('color', ink)
    if background:
        backdrop(tree, background)
    return tree


def overlay(old, new, code):
    result = ET.Element(NS + 'svg', {'viewBox': '0 0 128 128', 'width': '1024', 'height': '1024'})
    backdrop(result, '#FFFFFF')
    for source, label, ink in ((old, 'old', '#087FBF'), (new, 'new', '#E45448')):
        source = deepcopy(source)
        prefix = f'{code}-{label}-'
        for node in source.iter():
            if 'id' in node.attrib:
                node.set('id', prefix + node.attrib['id'])
            for key, value in list(node.attrib.items()):
                if 'url(#' in value:
                    node.set(key, re.sub(r'url\(#([^)]+)\)', lambda m: f'url(#{prefix}{m[1]})', value))
                elif key.endswith('href') and value.startswith('#'):
                    node.set(key, '#' + prefix + value[1:])
        for defs in source.findall(NS + 'defs'):
            result.append(defs)
        group = ET.SubElement(result, NS + 'g', {'color': ink, 'opacity': '.5'})
        mark = next(x for x in source if x.tag == NS + 'g')
        group.append(mark)
    return result


def main():
    for slug in VARIANTS:
        folder = ROOT / slug
        for size in (1024, 128, 32, 16):
            add_job(f'{slug}/previews/color-{size}.png', styled(folder, 'source.svg', size), size, 'color')
        for theme, (ink, color) in THEMES.items():
            for size in (1024, 128, 32, 16):
                add_job(f'{slug}/previews/mono-{theme}-{size}.png',
                        styled(folder, 'mono.svg', size, ink, color), size, 'mono')
            for scale in (1, 2):
                for context in ('transparent', 'context'):
                    add_job(f'{slug}/previews/template-{theme}-{context}-{scale}x.png',
                            styled(folder, 'menu-template.svg', 16 * scale, ink,
                                   color if context == 'context' else None),
                            16 * scale, 'template-' + context)

    sources = [('r02', BASELINE)] + [(slug, ROOT / slug) for slug in VARIANTS]
    for slug, folder in sources:
        for side, x in (('left', 8), ('right', 72)):
            tree = load(folder)
            tree.set('viewBox', f'{x} 56 48 48')
            tree.set('width', '1024')
            tree.set('height', '1024')
            target = f'comparison/r02-{side}-junction-1024.png' if slug == 'r02' else f'{slug}/details/{side}-junction-1024.png'
            add_job(target, tree, 1024, 'junction-detail')

    comparison = ROOT / 'comparison'
    comparison.mkdir(exist_ok=True)
    board = ET.Element(NS + 'svg', {'viewBox': '0 0 384 160', 'width': '2304', 'height': '960'})
    ET.SubElement(board, NS + 'rect', {'width': '384', 'height': '160', 'fill': '#FFFFFF'})
    for index, slug in enumerate(VARIANTS):
        code = f'A{index + 1}'
        tree = overlay(load(BASELINE), load(ROOT / slug), code)
        (comparison / f'overlay-{code.lower()}.svg').write_text(ET.tostring(tree, encoding='unicode') + '\n')
        add_job(f'comparison/overlay-{code.lower()}.png', tree, 1024, 'overlay')
        label = ET.SubElement(board, NS + 'text', {'x': str(index * 128 + 8), 'y': '14', 'font-size': '6', 'fill': '#24352E', 'font-family': 'sans-serif'})
        label.text = f'R02 + {code}'
        cell = deepcopy(tree)
        cell.set('x', str(index * 128))
        cell.set('y', '20')
        cell.set('width', '128')
        cell.set('height', '128')
        board.append(cell)
    for x, ink, label in ((8, '#087FBF', 'R02 50%'), (76, '#E45448', 'R03 50%')):
        ET.SubElement(board, NS + 'rect', {'x': str(x), 'y': '151', 'width': '5', 'height': '5', 'fill': ink, 'opacity': '.5'})
        text = ET.SubElement(board, NS + 'text', {'x': str(x + 7), 'y': '155.5', 'font-size': '4', 'fill': '#24352E', 'font-family': 'sans-serif'})
        text.text = label
    (comparison / 'overlay-board.svg').write_text(ET.tostring(board, encoding='unicode') + '\n')
    add_job('comparison/overlay-board.png', board, 2304, 'overlay-board', 960)
    subprocess.run(['node', str(ROOT / 'render_svg.mjs')], input=json.dumps(JOBS), text=True, check=True)
    manifest = []
    for job in JOBS:
        with Image.open(ROOT / job['file']) as image:
            assert image.size == (job['width'], job['height']), job['file']
            if job['kind'] == 'template-transparent':
                assert image.convert('RGBA').getextrema()[3] == (0, 255), job['file']
            elif job['kind'] == 'color':
                assert image.convert('RGBA').getextrema()[3] == (255, 255), job['file']
        manifest.append({k: v for k, v in job.items() if k != 'svg'})
    (ROOT / 'preview-manifest.json').write_text(json.dumps({'files': manifest}, indent=2) + '\n')
    print(f'Rendered and checked {len(JOBS)} PNGs, including overlays and 1024px junction details.')


if __name__ == '__main__':
    main()
