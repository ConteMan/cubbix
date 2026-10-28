#!/usr/bin/env python3
"""Check active UI palette pairs with the WCAG sRGB contrast formula."""
import argparse
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


def resolve(variables, name, mode, seen=()):
    if name in seen:
        raise ValueError('Variable alias cycle: ' + name)
    value = variables[name]['value']
    if isinstance(value, list):
        value = next(entry['value'] for entry in value if entry.get('theme', {}).get('mode') == mode)
    if value.startswith('$'):
        return resolve(variables, value[1:], mode, seen + (name,))
    return value


def luminance(color):
    rgb = [int(color[index:index + 2], 16) / 255 for index in (1, 3, 5)]
    linear = [x / 12.92 if x <= 0.04045 else ((x + 0.055) / 1.055) ** 2.4 for x in rgb]
    return sum(a * b for a, b in zip(linear, (0.2126, 0.7152, 0.0722)))


def contrast(a, b):
    light, dark = sorted((luminance(a), luminance(b)), reverse=True)
    return (light + 0.05) / (dark + 0.05)


def audit():
    variables = json.loads((ROOT / 'tokens.json').read_text())['variables']
    text_pairs = [
        ('primary button and selected segment', 'ColorOnAccent', 'ColorAccent'),
        ('selected navigation', 'ColorAccent', 'ColorAccentSoft'),
        ('tooltip', 'ColorTooltipText', 'ColorTooltip'),
    ]
    for surface in ('ColorWindow', 'ColorCard', 'ColorSidebar', 'ColorControl', 'ColorAccentSoft'):
        for foreground in ('ColorText', 'ColorTextSecondary'):
            text_pairs.append((foreground + ' on ' + surface, foreground, surface))
    graphic_pairs = [
        ('on switch thumb', 'ColorThumb', 'ColorAccent'),
        ('off switch thumb', 'ColorText', 'ColorControlHover'),
        ('off switch outline', 'ColorTextSecondary', 'ColorControlHover'),
        ('selected segment boundary', 'ColorAccent', 'ColorControl'),
        ('selected navigation outline', 'ColorTextSecondary', 'ColorAccentSoft'),
    ]
    for surface in ('ColorWindow', 'ColorCard', 'ColorControlHover'):
        for foreground in ('ColorAccent', 'ColorTextSecondary', 'ColorDanger', 'ColorSuccess'):
            graphic_pairs.append((foreground + ' icon on ' + surface, foreground, surface))
    pairs = []
    palette = []
    for mode in ('light', 'dark'):
        for category, items, threshold in (('text', text_pairs, 4.5), ('graphic', graphic_pairs, 3.0)):
            for purpose, foreground, background in items:
                fg, bg = resolve(variables, foreground, mode), resolve(variables, background, mode)
                ratio = contrast(fg, bg)
                pairs.append({'mode': mode, 'category': category, 'purpose': purpose,
                              'foregroundToken': foreground, 'backgroundToken': background,
                              'foreground': fg, 'background': bg, 'ratio': ratio,
                              'threshold': threshold, 'passed': ratio >= threshold})
        for key, definition in variables.items():
            if definition['type'] != 'color':
                continue
            color = resolve(variables, key, mode)
            rgb = [int(color[index:index + 2], 16) for index in (1, 3, 5)]
            palette.append({'mode': mode, 'token': key, 'color': color,
                            'allowed': key == 'ColorDanger' or len(set(rgb)) == 1})
    return {
        'method': 'WCAG 2.2 sRGB relative luminance; compare unrounded ratios against thresholds.',
        'sources': ['https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html',
                    'https://www.w3.org/WAI/WCAG22/Understanding/non-text-contrast.html'],
        'scope': 'Active token pairings; inactive controls and decorative separators excluded; not a runtime accessibility audit.',
        'pairs': pairs, 'palette': palette,
        'minimumTextRatio': min(item['ratio'] for item in pairs if item['category'] == 'text'),
        'minimumGraphicRatio': min(item['ratio'] for item in pairs if item['category'] == 'graphic'),
        'passed': all(item['passed'] for item in pairs) and all(item['allowed'] for item in palette),
    }


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--out', type=Path)
    args = parser.parse_args()
    result = audit()
    if args.out:
        args.out.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({key: result[key] for key in ('passed', 'minimumTextRatio', 'minimumGraphicRatio')}))
    raise SystemExit(0 if result['passed'] else 1)
