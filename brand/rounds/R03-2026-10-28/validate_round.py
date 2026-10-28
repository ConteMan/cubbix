#!/usr/bin/env python3
"""Check the controlled variants, actual G2 joins, and rendered layer spacing.

Requires Pillow 13.0.0, svgpathtools 1.7.2, SciPy 1.18.1 and the renderer's
Node dependency. No temporary image files are created.
"""
from copy import deepcopy
from io import BytesIO
from pathlib import Path
import json
from math import acos, degrees
import subprocess
import xml.etree.ElementTree as ET

import numpy as np
from PIL import Image, ImageChops
from scipy.ndimage import label, distance_transform_edt
from scipy.optimize import minimize_scalar
from svgpathtools import parse_path, Line, CubicBezier

ROOT = Path(__file__).resolve().parent
BASE = ROOT.parent / 'R02-2026-10-28/a-box-cat'
VARIANTS = ('a1-head', 'a2-layer', 'a3-combined')
NS = '{http://www.w3.org/2000/svg}'
REPORT = {'variants': {}}
ET.register_namespace('', NS[1:-1])


def get(tree, name):
    return tree.find(f'.//*[@id="{name}"]')


def contour(node):
    return parse_path(node.attrib['d']).continuous_subpaths()[0]


def curvature(segment, t):
    v = segment.derivative(t)
    a = segment.derivative(t, 2)
    return (v.conjugate() * a).imag / abs(v) ** 3


def join_error(a, ta, b, tb):
    u, v = a.derivative(ta), b.derivative(tb)
    angle = degrees(acos(np.clip((u.conjugate() * v).real / (abs(u) * abs(v)), -1, 1)))
    return angle, abs(curvature(a, ta) - curvature(b, tb))


def g2(path):
    segments = [s for s in path if s.length() > 1e-7]
    errors = [join_error(a, 1, b, 0) for a, b in zip(segments, segments[1:] + segments[:1])]
    maximum = (max(x[0] for x in errors), max(x[1] for x in errors))
    assert maximum[0] < .001 and maximum[1] < 1e-6, maximum
    return {'joins': len(errors), 'max_tangent_degrees': maximum[0], 'max_curvature_jump': maximum[1]}


def minimum_radius(path, sign=None):
    maximum = 0
    for segment in path:
        if not isinstance(segment, CubicBezier):
            continue
        def value(t):
            k = curvature(segment, t)
            return abs(k) if sign is None else max(0, sign * k)
        fit = minimize_scalar(lambda t: -value(t), bounds=(0, 1), method='bounded')
        maximum = max(maximum, value(0), value(1), -fit.fun)
    return 1 / maximum


def rendered(tree, part=None, width=1024):
    tree = deepcopy(tree)
    for node in list(tree):
        if node.tag == NS + 'rect':
            tree.remove(node)
    if part:
        mark = get(tree, 'cat-mark')
        for node in list(mark):
            if node.attrib.get('id') != part:
                mark.remove(node)
    tree.set('color', '#000000')
    job = [{'file': None, 'svg': ET.tostring(tree, encoding='unicode'), 'width': width}]
    result = subprocess.run(['node', str(ROOT / 'render_svg.mjs')], input=json.dumps(job).encode(), capture_output=True, check=True)
    return Image.open(BytesIO(result.stdout)).convert('RGBA')


def opening_image(tree):
    d = parse_path(get(tree, 'box-frame').attrib['d']).continuous_subpaths()[1].d()
    simple = ET.Element(NS + 'svg', {'width': '128', 'height': '128', 'viewBox': '0 0 128 128'})
    ET.SubElement(simple, NS + 'path', {'d': d, 'fill': '#000000'})
    return rendered(simple)


def cap_continuity(tree):
    head = contour(get(tree, 'cat-head'))
    caps = [s for s in contour(get(tree, 'rounded-head-visibility')) if isinstance(s, CubicBezier)]
    output = []
    for index, cap in enumerate(caps):
        endpoint = 0 if index == 0 else 1
        point = cap.point(endpoint)
        matches = []
        for segment in head:
            if not isinstance(segment, CubicBezier) and not isinstance(segment, Line):
                continue
            roots = np.roots((np.poly1d(segment.poly().coeffs.imag) - np.poly1d([point.imag])).coeffs)
            for t in roots:
                if abs(t.imag) < 1e-7 and -1e-7 <= t.real <= 1 + 1e-7:
                    t = float(np.clip(t.real, 0, 1))
                    if abs(segment.point(t) - point) < 1e-5:
                        matches.append(join_error(segment, t, cap, endpoint))
        assert matches, ('head cap does not join cheek', point)
        error = min(matches)
        assert error[0] < .001 and error[1] < 1e-6, error
        assert abs(curvature(cap, 1 - endpoint)) < 1e-7
        output.append({'tangent_degrees': error[0], 'curvature_jump': error[1]})
    return output


def pixel_check(folder, slug):
    output = []
    for scale in (1, 2):
        light = Image.open(folder / 'previews' / f'template-light-transparent-{scale}x.png').convert('RGBA')
        dark = Image.open(folder / 'previews' / f'template-dark-transparent-{scale}x.png').convert('RGBA')
        assert ImageChops.difference(light.getchannel('A'), dark.getchannel('A')).getbbox() is None
        alpha = np.array(light.getchannel('A'))
        solid = alpha >= 128
        components, count = label(solid, structure=np.ones((3, 3)))
        empty, _ = label(~solid)
        exterior = set(np.r_[empty[0], empty[-1], empty[:, 0], empty[:, -1]])
        holes = set(np.unique(empty)) - exterior - {0}
        expected = 3 if slug == 'a1-head' else 2
        assert len(holes) == expected, (slug, scale, 'aperture count', len(holes))
        centers = [(48, 56), (80, 56)] if slug == 'a1-head' else [(49, 52.75), (79, 52.75)]
        eye_labels = []
        for x, y in centers:
            px, py = int(x * scale / 8), int(y * scale / 8)
            assert alpha[py, px] < 64, (slug, scale, 'eye unclear')
            eye_labels.append(empty[py, px])
        assert len(set(eye_labels)) == 2 and all(x in holes for x in eye_labels), (slug, scale, 'eyes merged')
        border = np.r_[alpha[0], alpha[-1], alpha[:, 0], alpha[:, -1]]
        assert border.max() < 128, (slug, scale, 'clipped template')
        output.append({'scale': scale, 'ink_components': count, 'enclosed_apertures': len(holes)})
    return output


def main():
    old = ET.fromstring((BASE / 'source.svg').read_text())
    for slug in VARIANTS:
        folder = ROOT / slug
        tree = ET.fromstring((folder / 'source.svg').read_text())
        info = {}
        for name in ('source.svg', 'mono.svg', 'menu-template.svg'):
            vector = ET.fromstring((folder / name).read_text())
            for node in vector.iter():
                assert node.tag not in {NS + 'image', NS + 'filter', NS + 'script', NS + 'text'}
            for part in ('cat-head', 'box-frame', 'left-paw', 'right-paw'):
                node = get(vector, part)
                assert node is not None and node.tag == NS + 'path', (slug, part)
                assert all(s.isclosed() for s in parse_path(node.attrib['d']).continuous_subpaths())
        head = contour(get(tree, 'cat-head'))
        if slug != 'a2-layer':
            info['head_g2'] = g2(head)
            assert not any(isinstance(s, Line) and max(s.start.imag, s.end.imag) > 36.00001 for s in head)
            info['head_convex_min_radius'] = minimum_radius(head, 1)
            info['head_concave_min_radius'] = minimum_radius(head, -1)
            assert abs(info['head_concave_min_radius'] - 4) < 1e-4
        else:
            assert parse_path(get(tree, 'cat-head').attrib['d']).d() == parse_path(get(old, 'cat-head-and-eye-cutouts').attrib['d']).d()
        if slug == 'a1-head':
            assert parse_path(get(tree, 'box-frame').attrib['d']).d() == parse_path(get(old, 'single-box-frame').attrib['d']).d()
            for part, x in (('left-paw', 36), ('right-paw', 72)):
                assert np.allclose(contour(get(tree, part)).bbox(), (x, x + 20, 70, 90))
        else:
            info['visible_cap_g2'] = cap_continuity(tree)
            opening = parse_path(get(tree, 'box-frame').attrib['d']).continuous_subpaths()[1]
            assert np.allclose(contour(get(tree, 'box-frame')).bbox(), (20, 108, 76, 120))
            assert np.allclose(opening.bbox(), (28, 100, 84, 112))
            for part in ('left-paw', 'right-paw'):
                paw = contour(get(tree, part))
                info[part + '_g2'] = g2(paw)
                assert abs(minimum_radius(paw) - 4) < 1e-4
                assert not paw.intersect(opening), (slug, part, 'paw enters opening')
            head_ink = np.array(rendered(tree, 'cat-head').getchannel('A')) >= 128
            box_ink = np.array(rendered(tree, 'box-frame').getchannel('A')) >= 128
            void = np.array(opening_image(tree).getchannel('A')) >= 128
            assert not head_ink[576:].any(), (slug, 'head extends below occlusion edge')
            head_distance = distance_transform_edt(~head_ink)
            box_distance = distance_transform_edt(~box_ink)
            distances = []
            for part in ('left-paw', 'right-paw'):
                paw = np.array(rendered(tree, part).getchannel('A')) >= 128
                assert not (paw & void).any(), (slug, 'paw fills box aperture')
                distances.append({'part': part, 'head_gap': head_distance[paw].min() / 8,
                                  'box_gap': box_distance[paw].min() / 8})
            info['rendered_clearance_units'] = distances
            assert all(x['head_gap'] >= 3.9 and x['box_gap'] >= 3.9 for x in distances), distances
            assert not head_ink[576:608, 512].any() and not box_ink[576:608, 512].any()
        info['templates'] = pixel_check(folder, slug)
        REPORT['variants'][slug] = info
        print(slug, json.dumps(info, ensure_ascii=False))
    (ROOT / 'validation.json').write_text(json.dumps(REPORT, indent=2) + '\n')
    print('PASS: controls, separate paths, G2 contours, optical templates, and rendered clearances.')


if __name__ == '__main__':
    main()
