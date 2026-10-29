"""Regression checks for editable active-state geometry and transparent exports."""
import copy
import json
from pathlib import Path
import shutil
import tempfile
import unittest

import export as logo


class ActiveExportTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.master, cls.state, _ = logo.read_source(None)
        cls.nodes = logo.source_nodes(cls.master)
        cls.bounds = logo.artwork_box(cls.nodes)
        cls.pupils = logo.pupil_ids(cls.nodes)

    def test_only_named_state_artboard_is_allowed(self):
        roots = [{'id': logo.MASTER_ID, 'type': 'frame'},
                 {'id': 'state', 'type': 'frame', 'name': logo.STATE_NAME}]
        logo.validate_roots(roots)
        for invalid in (roots[:1], roots + [{'id': 'draft', 'type': 'frame'}],
                        [roots[0], {**roots[1], 'name': 'draft'}],
                        [roots[0], {**roots[1], 'type': 'group'}]):
            with self.subTest(roots=invalid), self.assertRaises(ValueError):
                logo.validate_roots(invalid)

    def test_rejects_invalid_overlay_geometry_and_styles(self):
        mutations = [
            lambda s: s.update(width=512),
            lambda s: s.update(fill=logo.ORANGE),
            lambda s: s.update(x=self.master['x'] + 1024),
            lambda s: s.update(rotation=15),
            lambda s: s['children'].append(copy.deepcopy(s['children'][0])),
            lambda s: s['children'][1].update(x=705),
            lambda s: s['children'][1].update(width=140, height=140, x=710, y=780),
            lambda s: s['children'][0].update(width=240, height=240, x=660, y=730),
            lambda s: s['children'][1].update(width=153),
            lambda s: s['children'][1].update(opacity=.5),
            lambda s: s['children'][1].update(enabled=False),
            lambda s: s['children'][1].update(effect=[{'type': 'blur', 'radius': 2}]),
            lambda s: s['children'][1].update(innerRadius=.5),
            lambda s: s['children'][1].update(x=float('nan')),
            lambda s: s['children'][1].update(fill='#FFFFFF'),
            lambda s: [n.update(x=n['x'] + 20) for n in s['children']],
        ]
        logo.active_nodes(self.state, self.master, self.bounds)
        for index, mutate in enumerate(mutations):
            invalid = copy.deepcopy(self.state)
            mutate(invalid)
            with self.subTest(mutation=index), self.assertRaises(ValueError):
                logo.active_nodes(invalid, self.master, self.bounds)

    def test_backdrops_are_white_and_clearance_fill_does_not_affect_export(self):
        self.assertEqual(self.master['fill'], logo.CANVAS_BG)
        self.assertEqual(self.state['fill'], logo.CANVAS_BG)
        invalid = copy.deepcopy(self.master)
        invalid['fill'] = logo.ORANGE
        with self.assertRaises(ValueError):
            logo.source_nodes(invalid)
        original = logo.active_nodes(self.state, self.master, self.bounds)
        expected = logo.svg_bytes(logo.template_svg(self.nodes, self.bounds, self.pupils, 1, original))
        for color in ('#FFFFFF', '#FF9A3D', '#123456'):
            state = copy.deepcopy(self.state)
            next(n for n in state['children'] if n['name'] == '常亮点留白')['fill'] = color
            with self.subTest(color=color):
                active = logo.active_nodes(state, self.master, self.bounds)
                actual = logo.svg_bytes(logo.template_svg(self.nodes, self.bounds, self.pupils, 1, active))
                self.assertEqual(actual, expected)

    def test_vector_cutout_is_transparent_and_dot_is_solid(self):
        # A solid synthetic mark makes the eraser's effect unambiguous.
        nodes = [{'type': 'ellipse', 'id': 'body', 'x': 0, 'y': 0, 'width': 1024, 'height': 1024}]
        dot = {'type': 'ellipse', 'id': 'dot', 'x': 704, 'y': 774, 'width': 152, 'height': 152}
        clearance = {'type': 'ellipse', 'id': 'clearance', 'x': 654, 'y': 724, 'width': 252, 'height': 252}
        image = logo.render(logo.template_svg(nodes, (0, 0, 1024, 1024), (), 16, (dot, clearance)))

        def pixel(x, y):
            return image.getpixel((int((1 + x / 64) * 16), int((1 + y / 64) * 16)))

        self.assertEqual(pixel(780, 850), (0, 0, 0, 255))
        self.assertEqual(pixel(680, 850)[3], 0)
        self.assertEqual(pixel(512, 512), (0, 0, 0, 255))

    def test_real_templates_preserve_eyes_placement_and_bounds(self):
        active = logo.active_nodes(self.state, self.master, self.bounds)
        for scale in (1, 2):
            base = logo.render(logo.template_svg(self.nodes, self.bounds, self.pupils, scale))
            on = logo.render(logo.template_svg(self.nodes, self.bounds, self.pupils, scale, active))
            self.assertEqual(on.size, (18 * scale, 18 * scale))
            self.assertEqual(base.getchannel('A').getbbox(), on.getchannel('A').getbbox())
            self.assertEqual(base.crop((0, 0, 18 * scale, 11 * scale)).tobytes(),
                             on.crop((0, 0, 18 * scale, 11 * scale)).tobytes())
            self.assertNotEqual(base.tobytes(), on.tobytes())

    def test_check_covers_both_new_templates_and_previews(self):
        manifest = json.loads((logo.DIST / 'manifest.json').read_text())
        logo.verify(logo.DIST, manifest)
        new_files = sorted(name for name in logo.expected_files() if '-active' in name)
        self.assertEqual(len(new_files), 6)
        with tempfile.TemporaryDirectory(prefix='cubbix-active-test-') as temporary:
            directory = Path(temporary) / 'dist'
            shutil.copytree(logo.DIST, directory)
            for name in new_files:
                path = directory / name
                original = path.read_bytes()
                path.unlink()
                with self.subTest(missing=name), self.assertRaises(ValueError):
                    logo.verify(directory, manifest)
                path.write_bytes(original + b'corruption')
                with self.subTest(corrupt=name), self.assertRaises(ValueError):
                    logo.verify(directory, manifest)
                path.write_bytes(original)


if __name__ == '__main__':
    unittest.main()
