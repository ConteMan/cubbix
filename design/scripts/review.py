#!/usr/bin/env python3
"""Verify R04 source logo mapping and expanded navigation state geometry."""
import hashlib
import json
import math
from pathlib import Path
import re
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
ROUND = ROOT / 'rounds' / 'R04-2026-10-28-收尾'


def walk(node):
    yield node
    for child in node.get('children', []):
        yield from walk(child)


def close(actual, expected):
    return len(actual) == len(expected) and all(math.isclose(a, b, abs_tol=1e-8) for a, b in zip(actual, expected))


def save(name, data):
    (ROUND / name).write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n')
    if not data['passed']:
        raise SystemExit(name + ' failed')


def main():
    document = json.loads((ROOT / 'cubbix.pen').read_text())
    nodes = [node for root in document['children'] for node in walk(root)]
    source = ROOT.parent / 'brand/dist/mark.svg'
    source_sha = hashlib.sha256(source.read_bytes()).hexdigest()
    svg = ET.parse(source).getroot()
    vx, vy, vw, vh = map(float, svg.attrib['viewBox'].split())
    mark = next(n for n in nodes if n.get('name') == 'Logo/Mark')
    size = document['variables']['SizeBrandMark']['value']
    scale, width = size / vh, size * vw / vh
    offset = (size - width) / 2
    checks = []
    for element, node in zip(svg, mark['children']):
        attrs = element.attrib
        if node['type'] == 'path':
            a, b, c, d, tx, ty = map(float, re.findall(r'[-+\d.e]+', attrs['transform'].replace('matrix', '')))
            expected_box = [(vx - tx) / a, (vy - ty) / d, vw / a, vh / d]
            geometry = node['geometry'] == attrs['d'] and b == c == 0
            geometry &= node['fillRule'] == attrs['fill-rule'] == 'evenodd'
            geometry &= close(node['viewBox'], expected_box)
            expected_geometry = [offset, 0, width, size]
        else:
            cx, cy, rx, ry = (float(attrs[k]) for k in ('cx', 'cy', 'rx', 'ry'))
            geometry = node['type'] == 'ellipse'
            expected_geometry = [offset + (cx - rx - vx) * scale, (cy - ry - vy) * scale, 2 * rx * scale, 2 * ry * scale]
        geometry &= close([node.get('x', 0), node.get('y', 0), node['width'], node['height']], expected_geometry)
        checks.append({'sourceId': attrs['id'], 'nodeId': node['id'], 'geometryMatches': bool(geometry),
                       'sourceHashMatches': node['metadata']['sha256'] == source_sha,
                       'sourceIdMatches': node['metadata']['sourceId'] == attrs['id'], 'themeFill': node['fill'] == '$ColorText'})
    render = json.loads((ROUND / 'render-validation.json').read_text())
    native = render['nativeLayout']
    expected_ids = {element.attrib['id'] for element in svg}
    native_brands = native['brands']
    fills = {part['fill'] for item in native_brands for part in item['parts']}
    native_logo_pass = len(native_brands) == 19 and fills == {'#202020', '#F5F5F5'} and all(
        item['width'] == item['height'] == size and len(item['parts']) == 6 and
        {part['sourceId'] for part in item['parts']} == expected_ids and
        len({part['fill'] for part in item['parts']}) == 1 for item in native_brands)
    save('logo-validation.json', {'source': 'brand/dist/mark.svg', 'sourceSha256': source_sha,
         'method': 'Compare source path data, inverse-transform viewBoxes, ellipse bounds, evenodd holes, provenance and native themed fills.',
         'markSize': size, 'parts': checks, 'nativeMarkInstances': len(native_brands), 'nativeForegroundColors': sorted(fills),
         'passed': len(svg) == len(mark['children']) == len(checks) == 6 and
         mark['width'] == mark['height'] == size and all(all(c[k] for k in ('geometryMatches', 'sourceHashMatches', 'sourceIdMatches', 'themeFill')) for c in checks) and native_logo_pass})
    navigation = native['navigation']
    hover = next(n for n in navigation if n['name'] == 'Navigation hover')
    selected = next(n for n in navigation if n['name'] == 'Navigation selected')
    shape_keys = ('width', 'height', 'cornerRadius', 'padding', 'gap', 'stroke', 'strokeWidth')
    shape_equal = all(hover.get(k) == selected.get(k) for k in shape_keys)
    bounds_equal = all(hover['bounds'][k] == selected['bounds'][k] for k in ('width', 'height'))
    no_stroke = all(n.get('stroke') == '$ColorTransparent' and n.get('strokeWidth') == '$Space0' for n in navigation)
    samples = [n for n in document['children'] if n.get('metadata', {}).get('type') == 'sample']
    sample_checks = []
    for sample in samples:
        shell = sample['children'][0]
        items = [n for n in navigation if n['id'].startswith(shell['id'] + '/')]
        brands = [n for n in native_brands if n['id'].startswith(shell['id'] + '/')]
        sample_checks.append({'sample': sample['name'], 'navigationItems': len(items), 'logoMarks': len(brands),
                              'passed': len(items) == 2 and len(brands) == 1 and sum(n['fill'] == '$ColorAccentSoft' for n in items) == 1})
    save('structure-validation.json', {'nativeExpandedNodes': native['nodes'], 'navigationItems': len(navigation),
         'hoverAndSelectedSameGeometry': shape_equal and bounds_equal, 'navigationWithoutStroke': no_stroke,
         'hoverAndSelectedDifferentFill': hover['fill'] != selected['fill'], 'samples': sample_checks,
         'passed': len(navigation) == 34 and shape_equal and bounds_equal and no_stroke and hover['fill'] != selected['fill'] and len(samples) == 12 and all(n['passed'] for n in sample_checks)})
    print('Logo provenance, theme fills and navigation geometry passed.')


if __name__ == '__main__':
    main()
