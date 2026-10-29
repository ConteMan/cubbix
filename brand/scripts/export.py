#!/usr/bin/env python3
"""Export approved Cubbix artwork through the pen.dev headless CLI.

The .pen file is never parsed directly or written by this script. resvg
renders derived SVG in memory; Python dependencies belong outside the repo.
"""
from __future__ import annotations

import argparse
import hashlib
from importlib.metadata import version
from io import BytesIO
import json
import math
from pathlib import Path
import re
import selectors
import shutil
import subprocess
import tempfile
import time
import xml.etree.ElementTree as ET

from PIL import Image
from svgpathtools import parse_path

BRAND = Path(__file__).resolve().parents[1]
SOURCE = BRAND / 'cubbix-logo.pen'
DIST = BRAND / 'dist'
MASTER_ID = 'n0a0W'
STATE_NAME = '菜单栏状态 · 常亮点'
INK = '#202522'
ORANGE = '#FF9A3D'
SIZES = (16, 32, 64, 128, 256, 512, 1024)
NS = '{http://www.w3.org/2000/svg}'
ET.register_namespace('', NS[1:-1])
ANSI = re.compile(r'\x1b\[[0-9;?]*[ -/]*[@-~]')
BEGIN = 'CUBBIX_MASTER_BEGIN'
END = 'CUBBIX_MASTER_END'


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def canonical(value) -> bytes:
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(',', ':')).encode()


def read_source(work_dir: Path | None) -> tuple[dict, dict, str]:
    """Call only Get/Print in an isolated headless document, then close it."""
    before = sha256(SOURCE.read_bytes())
    if work_dir:
        work_dir.mkdir(parents=True, exist_ok=True)
        if work_dir.resolve().is_relative_to(BRAND.parent.resolve()):
            raise ValueError('The work directory must be outside the repository')
    with tempfile.TemporaryDirectory(prefix='cubbix-logo-', dir=work_dir) as temporary:
        temporary = Path(temporary)
        command = ['pen', 'interactive', '--in', str(SOURCE), '--out', str(temporary / 'read-only-copy.pen'),
                   '--preview-output', str(temporary / 'unused-preview.png')]
        process = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                   stderr=subprocess.STDOUT, bufsize=0)
        selector = selectors.DefaultSelector()
        selector.register(process.stdout, selectors.EVENT_READ)
        transcript = ''
        sent = False
        deadline = time.monotonic() + 90
        data = None
        try:
            while time.monotonic() < deadline:
                for key, _ in selector.select(timeout=.25):
                    chunk = key.fileobj.read(65536)
                    if not chunk:
                        selector.unregister(key.fileobj)
                        break
                    transcript += chunk.decode('utf-8', errors='replace')
                text = ANSI.sub('', transcript).replace('\r', '')
                if not sent and '[INFO] Ready.' in text:
                    program = (f'Print("{BEGIN}");Print(JSON.stringify({{roots:Get((n,c)=>{{c.skipChildren();'
                               'return {id:n.id,type:n.type,name:n.name}}),'
                               f'master:Get("{MASTER_ID}",{{depth:12,includePathGeometry:true,resolveVariables:true}}),'
                               'states:Get((n,c)=>{c.skipChildren();'
                               f'if(n.name==={json.dumps(STATE_NAME, ensure_ascii=False)})'
                               'return Get(n.id,{depth:2,resolveVariables:true})})}));'
                               f'Print("{END}")')
                    request = 'execute(' + json.dumps({'input': program}, ensure_ascii=False) + ')\n'
                    process.stdin.write(request.encode())
                    process.stdin.flush()
                    sent = True
                match = re.search(rf'(?m)^{BEGIN}\s*\n(.*?)\n{END}\s*$', text, flags=re.DOTALL)
                if match:
                    data = json.loads(match.group(1).strip())
                    break
                if process.poll() is not None:
                    break
            if data is None:
                raise RuntimeError('pen CLI did not return the master data; check the CLI and saved source')
            process.stdin.write(b'exit()\n')
            process.stdin.flush()
            process.wait(timeout=15)
        finally:
            selector.close()
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
            for stream in (process.stdin, process.stdout):
                if stream:
                    stream.close()
    after = sha256(SOURCE.read_bytes())
    if before != after:
        raise RuntimeError('The source changed during export; save it and rerun')
    validate_roots(data['roots'])
    master = data['master']
    if (master['width'], master['height']) != (1024, 1024):
        raise ValueError('The master canvas must remain 1024 by 1024')
    if len(data['states']) != 1:
        raise ValueError('Expected exactly one active-state artboard')
    return master, data['states'][0], before


def validate_roots(roots: list[dict]) -> None:
    masters = [n for n in roots if n['id'] == MASTER_ID and n['type'] == 'frame']
    states = [n for n in roots if n.get('name') == STATE_NAME and n['type'] == 'frame']
    if len(roots) != 2 or len(masters) != 1 or len(states) != 1 or masters[0] == states[0]:
        raise ValueError('Only the approved master and named active-state artboards are allowed')


def active_nodes(state: dict, master: dict, bounds) -> tuple[dict, dict]:
    """Validate editable circles in the master's local coordinate system."""
    for frame in (master, state):
        for key in ('x', 'y', 'width', 'height'):
            if not finite_number(frame.get(key)):
                raise ValueError('Artboard coordinates must be finite numbers')
    if (state['width'], state['height']) != (1024, 1024):
        raise ValueError('The active-state canvas must remain 1024 by 1024')
    if state['x'] < master['x'] + master['width'] + 80 or state['y'] != master['y']:
        raise ValueError('Place the active-state artboard to the right with at least 80 units of space')
    if state.get('layout', 'none') != 'none' or unsupported_style(state):
        raise ValueError('Keep the active-state artboard untransformed and absolutely positioned')
    layers = state.get('children', [])
    if len(layers) != 2 or {n.get('name') for n in layers} != {'常亮点', '常亮点留白'}:
        raise ValueError('Keep exactly the dot and clearance layers in the active-state artboard')
    for layer in layers:
        if layer['type'] != 'ellipse' or unsupported_style(layer):
            raise ValueError('Active-state layers must be plain, enabled, solid circles')
        if any(layer.get(k, 0) for k in ('innerRadius', 'startAngle')) or layer.get('sweepAngle', 360) != 360:
            raise ValueError('Active-state circles must not be rings or arcs')
        if not all(finite_number(layer.get(k)) for k in ('x', 'y', 'width', 'height')):
            raise ValueError('Active-state geometry must be finite')
        if layer['width'] <= 0 or layer['width'] != layer['height']:
            raise ValueError('Active-state layers must be circles with positive diameters')
        if not (0 <= layer['x'] <= 1024 - layer['width'] and 0 <= layer['y'] <= 1024 - layer['height']):
            raise ValueError('Active-state circles must stay inside the 1024 canvas')
    dot = next(n for n in layers if n['name'] == '常亮点')
    clearance = next(n for n in layers if n['name'] == '常亮点留白')
    if str(dot.get('fill', '')).upper() != INK or str(clearance.get('fill', '')).upper() != ORANGE:
        raise ValueError('Use ink for the dot and the master background for the clearance guide')
    if any(not math.isclose(dot[k] + dot['width'] / 2, clearance[k] + clearance['width'] / 2,
                            rel_tol=0, abs_tol=1e-7) for k in ('x', 'y')):
        raise ValueError('The dot and clearance must be concentric')
    unit = (bounds[3] - bounds[1]) / 16
    if dot['width'] < 3 * unit or (clearance['width'] - dot['width']) / 2 < unit:
        raise ValueError('At 16pt mark height the dot must be at least 3pt and clearance at least 1pt')
    box = path_box(dot)
    if any(box[i] < bounds[i] for i in (0, 1)) or any(box[i] > bounds[i] for i in (2, 3)):
        raise ValueError('The dot must not expand the original mark bounds')
    return dot, clearance


def finite_number(value) -> bool:
    return isinstance(value, (int, float)) and not isinstance(value, bool) and math.isfinite(value)


def unsupported_style(node: dict) -> bool:
    return (node.get('enabled', True) is not True or node.get('rotation', 0)
            or node.get('opacity', 1) != 1 or node.get('flipX') or node.get('flipY')
            or any(node.get(k) for k in ('stroke', 'effect', 'effects')))


def source_nodes(master: dict) -> list[dict]:
    nodes = master['children']
    if not isinstance(nodes, list) or not nodes:
        raise ValueError('The master contains no vector layers')
    for node in nodes:
        if node['type'] not in ('path', 'ellipse'):
            raise ValueError('Keep direct path and ellipse layers in the master')
        if node.get('enabled', True) is not True:
            raise ValueError('Remove disabled draft layers from the master')
        if node.get('rotation', 0) or node.get('opacity', 1) != 1 or node.get('stroke') or node.get('effects'):
            raise ValueError('Unsupported layer styling; export stopped without changing the source')
        if str(node.get('fill', '')).upper() != INK:
            raise ValueError('All approved foreground layers must use the ink color')
        for key in ('x', 'y', 'width', 'height'):
            if not isinstance(node.get(key), (int, float)) or not math.isfinite(node[key]):
                raise ValueError(f'Invalid layer {key}')
        if node['width'] <= 0 or node['height'] <= 0:
            raise ValueError('Layer dimensions must be positive')
        if node['type'] == 'path':
            vx, vy, vw, vh = node['viewBox']
            if vw <= 0 or vh <= 0:
                raise ValueError('A path has an empty viewBox')
            parse_path(node['geometry'])
    if str(master.get('fill', '')).upper() != ORANGE:
        raise ValueError('The master frame must retain the approved orange')
    return nodes


def path_box(node: dict, path=None) -> tuple[float, float, float, float]:
    if node['type'] == 'ellipse':
        return node['x'], node['y'], node['x'] + node['width'], node['y'] + node['height']
    path = parse_path(node['geometry']) if path is None else path
    low_x, high_x, low_y, high_y = path.bbox()
    vx, vy, vw, vh = node['viewBox']
    sx, sy = node['width'] / vw, node['height'] / vh
    return (node['x'] + (low_x - vx) * sx, node['y'] + (low_y - vy) * sy,
            node['x'] + (high_x - vx) * sx, node['y'] + (high_y - vy) * sy)


def artwork_box(nodes: list[dict]) -> tuple[float, float, float, float]:
    bounds = [path_box(node) for node in nodes]
    return min(b[0] for b in bounds), min(b[1] for b in bounds), max(b[2] for b in bounds), max(b[3] for b in bounds)


def pupil_ids(nodes: list[dict]) -> list[str]:
    """Identify the two small ellipses inside the head's actual eye apertures.

    Layer names are deliberately not used: the approved paw circles also have
    legacy pupil names. Any ambiguous match stops the export.
    """
    heads = [node for node in nodes if node['type'] == 'path' and '头' in node.get('name', '')]
    if len(heads) != 1:
        raise ValueError('Expected one named head path')
    head = heads[0]
    holes = parse_path(head['geometry']).continuous_subpaths()[1:]
    if len(holes) != 2:
        raise ValueError('Expected two eye apertures in the approved head')
    result = []
    for hole in holes:
        x0, y0, x1, y1 = path_box(head, hole)
        matches = []
        for node in nodes:
            if node['type'] != 'ellipse':
                continue
            cx, cy = node['x'] + node['width'] / 2, node['y'] + node['height'] / 2
            if x0 < cx < x1 and y0 < cy < y1 and node['width'] < x1 - x0 and node['height'] < y1 - y0:
                matches.append(node['id'])
        if len(matches) != 1:
            raise ValueError('Each eye aperture must contain exactly one pupil ellipse')
        result.append(matches[0])
    if len(set(result)) != 2:
        raise ValueError('The two pupil layers must be distinct')
    return result


def number(value: float) -> str:
    return format(value, '.15g')


def svg_root(width, height, box) -> ET.Element:
    return ET.Element(NS + 'svg', {'width': number(width), 'height': number(height),
                                  'viewBox': ' '.join(number(v) for v in box)})


def append_nodes(parent: ET.Element, nodes: list[dict], color: str, omitted=()) -> None:
    for node in nodes:
        if node['id'] in omitted:
            continue
        common = {'id': 'source-' + node['id'], 'fill': color}
        if node['type'] == 'path':
            vx, vy, vw, vh = node['viewBox']
            sx, sy = node['width'] / vw, node['height'] / vh
            matrix = (sx, 0, 0, sy, node['x'] - sx * vx, node['y'] - sy * vy)
            ET.SubElement(parent, NS + 'path', {**common, 'd': node['geometry'],
                          'fill-rule': node.get('fillRule', 'nonzero'),
                          'transform': 'matrix(' + ' '.join(number(v) for v in matrix) + ')'})
        else:
            ET.SubElement(parent, NS + 'ellipse', {**common,
                          'cx': number(node['x'] + node['width'] / 2),
                          'cy': number(node['y'] + node['height'] / 2),
                          'rx': number(node['width'] / 2), 'ry': number(node['height'] / 2)})


def placed(nodes, bounds, color, cx, cy, height, omitted=()) -> ET.Element:
    x0, y0, x1, y1 = bounds
    scale = height / (y1 - y0)
    group = ET.Element(NS + 'g', {'transform':
                       f'translate({number(cx)} {number(cy)}) scale({number(scale)}) '
                       f'translate({number(-(x0 + x1) / 2)} {number(-(y0 + y1) / 2)})'})
    append_nodes(group, nodes, color, omitted)
    return group


def squircle() -> str:
    points = []
    for step in range(4096):
        angle = 2 * math.pi * step / 4096
        values = (math.cos(angle), math.sin(angle))
        coords = [512 + 412 * (0 if abs(v) < 1e-12 else math.copysign(abs(v) ** .4, v)) for v in values]
        points.append(' '.join(number(v) for v in coords))
    return 'M ' + ' L '.join(points) + ' Z'


def app_svg(nodes, bounds) -> ET.Element:
    tree = svg_root(1024, 1024, (0, 0, 1024, 1024))
    ET.SubElement(tree, NS + 'path', {'id': 'generated-superellipse', 'fill': ORANGE, 'd': squircle()})
    tree.append(placed(nodes, bounds, INK, 512, 500, 560))
    return tree


def template_svg(nodes, bounds, pupils, scale, active=None) -> ET.Element:
    tree = svg_root(18 * scale, 18 * scale, (0, 0, 18, 18))
    group = placed(nodes, bounds, '#000000', 9, 9, 16, pupils)
    if active:
        dot, clearance = active
        x0, y0, x1, y1 = bounds
        # Keep mask edges far from the silhouette to avoid multiplying its antialiasing.
        padding = max(x1 - x0, y1 - y0)
        region = {'x': number(x0 - padding), 'y': number(y0 - padding),
                  'width': number(x1 - x0 + 2 * padding), 'height': number(y1 - y0 + 2 * padding)}
        defs = ET.SubElement(tree, NS + 'defs')
        mask = ET.SubElement(defs, NS + 'mask', {'id': 'active-clearance', 'maskUnits': 'userSpaceOnUse',
                                                **region})
        ET.SubElement(mask, NS + 'rect', {**region, 'fill': '#FFFFFF'})
        append_nodes(mask, [clearance], '#000000')
        cutout = ET.Element(NS + 'g', {'mask': 'url(#active-clearance)'})
        for child in list(group):
            group.remove(child)
            cutout.append(child)
        group.append(cutout)
        append_nodes(group, [dot], '#000000')
    tree.append(group)
    return tree


def mark_svg(nodes, bounds, color) -> ET.Element:
    x0, y0, x1, y1 = bounds
    tree = svg_root(x1 - x0, y1 - y0, (x0, y0, x1 - x0, y1 - y0))
    append_nodes(tree, nodes, color)
    return tree


def svg_bytes(tree: ET.Element) -> bytes:
    return ET.tostring(tree, encoding='utf-8', xml_declaration=True) + b'\n'


def render(tree: ET.Element) -> Image.Image:
    program = ("const fs=require('fs');const {Resvg}=require('@resvg/resvg-js');"
               "process.stdout.write(new Resvg(fs.readFileSync(0)).render().asPng())")
    result = subprocess.run(['node', '-e', program], input=svg_bytes(tree),
                            capture_output=True, check=True, timeout=60)
    image = Image.open(BytesIO(result.stdout)).convert('RGBA')
    expected = (int(float(tree.attrib['width'])), int(float(tree.attrib['height'])))
    if image.size != expected:
        raise ValueError(f'The SVG renderer produced an unexpected size: {image.size}')
    image.info.clear()
    return image


def save_png(image: Image.Image, path: Path) -> None:
    image.info.clear()
    image.save(path, compress_level=9, optimize=False)


def expected_files() -> set[str]:
    result = {f'app-icon-{size}.png' for size in SIZES}
    result |= {'mark.svg', 'mark-inverse.svg', 'menubar-template.png', 'menubar-template@2x.png'}
    result |= {f'menubar-preview-{theme}{suffix}.png' for theme in ('light', 'dark') for suffix in ('', '@2x')}
    result |= {f'menubar-template-active{suffix}.png' for suffix in ('', '@2x')}
    result |= {f'menubar-preview-active-{theme}{suffix}.png' for theme in ('light', 'dark') for suffix in ('', '@2x')}
    return result


def verify(directory: Path, manifest: dict) -> None:
    if manifest['source']['sha256'] != sha256(SOURCE.read_bytes()):
        raise ValueError('The master changed; regenerate the exports')
    if manifest['generator']['sha256'] != sha256(Path(__file__).read_bytes()):
        raise ValueError('The exporter changed; regenerate the exports')
    listed = {entry['file'] for entry in manifest['files']}
    if listed != expected_files():
        raise ValueError('The manifest has missing or unexpected export entries')
    actual = {path.name for path in directory.iterdir() if path.is_file()} - {'manifest.json'}
    if actual != listed:
        raise ValueError('The export directory has missing or unmanaged files')
    for entry in manifest['files']:
        data = (directory / entry['file']).read_bytes()
        if sha256(data) != entry['sha256'] or len(data) != entry['bytes']:
            raise ValueError('An export differs from the SHA-256 manifest: ' + entry['file'])
        if entry['file'].endswith('.png'):
            with Image.open(BytesIO(data)) as image:
                if image.size != (entry['width'], entry['height']):
                    raise ValueError('An export has an incorrect pixel size')
    with Image.open(directory / 'app-icon-1024.png') as image:
        if image.getchannel('A').getbbox() != (100, 100, 924, 924):
            raise ValueError('The app icon must have exactly 100px of transparent padding')
    for scale, suffix in ((1, ''), (2, '@2x')):
        boxes = []
        for variant in ('', '-active'):
            with Image.open(directory / f'menubar-template{variant}{suffix}.png') as image:
                rgba = image.convert('RGBA')
                boxes.append(rgba.getchannel('A').getbbox())
                if image.size != (18 * scale, 18 * scale) or rgba.getchannel('A').getextrema() != (0, 255):
                    raise ValueError('A template lacks the required size or transparency')
                for red, green, blue, alpha in rgba.get_flattened_data():
                    if alpha and (red or green or blue):
                        raise ValueError('Menu templates must contain only black and transparency')
        if boxes[0] != boxes[1]:
            raise ValueError('Active state must preserve the default template bounds')


def versions() -> dict:
    pen = subprocess.run(['pen', 'version'], capture_output=True, text=True, check=True).stdout
    match = re.search(r'(?m)^pen ([0-9.]+)\s*$', ANSI.sub('', pen))
    resvg = subprocess.run(['node', '-e', "process.stdout.write(require('@resvg/resvg-js/package.json').version)"],
                           capture_output=True, text=True, check=True).stdout
    return {'pen_cli': match[1] if match else 'unknown',
            'resvg': resvg,
            'pillow': version('Pillow'), 'svgpathtools': version('svgpathtools')}


def export(work_dir: Path | None) -> dict:
    master, state, source_hash = read_source(work_dir)
    nodes = source_nodes(master)
    bounds = artwork_box(nodes)
    pupils = pupil_ids(nodes)
    active = active_nodes(state, master, bounds)
    if artwork_box([node for node in nodes if node['id'] not in pupils]) != bounds:
        raise ValueError('Removing pupils must not change the silhouette bounds')
    if DIST.exists() and {p.name for p in DIST.iterdir()} - expected_files() - {'manifest.json'}:
        raise ValueError('The export directory contains unmanaged files')
    with tempfile.TemporaryDirectory(prefix='cubbix-dist-', dir=work_dir) as temporary:
        staged = Path(temporary)
        app = render(app_svg(nodes, bounds))
        for size in SIZES:
            image = app if size == 1024 else app.resize((size, size), Image.Resampling.LANCZOS)
            save_png(image, staged / f'app-icon-{size}.png')
        for scale, suffix in ((1, ''), (2, '@2x')):
            for variant, overlay in (('', None), ('-active', active)):
                image = render(template_svg(nodes, bounds, pupils, scale, overlay))
                save_png(image, staged / f'menubar-template{variant}{suffix}.png')
                for theme, background, ink in (('light', '#F5F5F7', '#000000'), ('dark', '#242424', '#FFFFFF')):
                    preview = Image.new('RGBA', image.size, background)
                    tint = Image.new('RGBA', image.size, ink)
                    tint.putalpha(image.getchannel('A'))
                    preview.alpha_composite(tint)
                    save_png(preview, staged / f'menubar-preview{variant}-{theme}{suffix}.png')
        (staged / 'mark.svg').write_bytes(svg_bytes(mark_svg(nodes, bounds, INK)))
        (staged / 'mark-inverse.svg').write_bytes(svg_bytes(mark_svg(nodes, bounds, '#FFFFFF')))
        files = []
        for name in sorted(expected_files()):
            data = (staged / name).read_bytes()
            entry = {'file': name, 'sha256': sha256(data), 'bytes': len(data)}
            if name.endswith('.png'):
                with Image.open(BytesIO(data)) as image:
                    entry.update(width=image.width, height=image.height)
            files.append(entry)
        dot, clearance = active
        unit = (bounds[3] - bounds[1]) / 16
        manifest = {'schema': 2,
                    'source': {'file': 'brand/cubbix-logo.pen', 'sha256': source_hash,
                               'artboard_id': MASTER_ID, 'node_count': len(nodes),
                               'geometry_sha256': sha256(canonical(nodes)), 'pupil_node_ids': pupils,
                               'active_artboard_id': state['id'], 'active_artboard_name': STATE_NAME,
                               'active_geometry_sha256': sha256(canonical(list(active))),
                               'active_node_ids': [dot['id'], clearance['id']]},
                    'generator': {'file': 'brand/scripts/export.py', 'sha256': sha256(Path(__file__).read_bytes()),
                                  'versions': versions()},
                    'spec': {'ink': INK, 'background': ORANGE,
                             'source_mark_bounds': list(bounds),
                             'app': {'canvas': 1024, 'body': 824, 'transparent_padding': 100,
                                     'superellipse_n': 5, 'mark_height': 560, 'mark_center': [512, 500]},
                             'menu': {'canvas_pt': 18, 'mark_height_pt': 16, 'center_pt': [9, 9], 'pupils': False,
                                      'active': {'center_source': [dot['x'] + dot['width'] / 2,
                                                                   dot['y'] + dot['height'] / 2],
                                                 'dot_diameter_pt': dot['width'] / unit,
                                                 'clearance_width_pt': (clearance['width'] - dot['width']) / 2 / unit,
                                                 'preserve_bounds': True}}},
                    'files': files}
        verify(staged, manifest)
        if sha256(SOURCE.read_bytes()) != source_hash:
            raise RuntimeError('The source changed before export completion; rerun')
        DIST.mkdir(exist_ok=True)
        for name in sorted(expected_files()):
            shutil.copyfile(staged / name, DIST / name)
        (DIST / 'manifest.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + '\n')
    return manifest


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--work-dir', type=Path, help='Optional temporary directory outside the repository')
    parser.add_argument('--check', action='store_true', help='Verify the committed exports and source hashes without changing files')
    args = parser.parse_args()
    if args.check:
        manifest = json.loads((DIST / 'manifest.json').read_text())
        verify(DIST, manifest)
        print('Verified: source, exporter, SHA-256 manifest, sizes and transparency.')
    else:
        manifest = export(args.work_dir)
        print(f'Exported {len(manifest["files"])} files from {MASTER_ID}; source preserved.')
        print('Verified: 824px superellipse body, 100px padding, 18/36px templates and SHA-256 manifest.')


if __name__ == '__main__':
    main()
