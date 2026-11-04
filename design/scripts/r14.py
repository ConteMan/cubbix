#!/usr/bin/env python3
"""Author spec 005's panel look and drag guides, then reopen and export them.

The pinned headless CLI keeps no variable binding in text content and no
variable binding for width and height: copy is written as localized text
named `Copy / <key>`, and geometry is resolved to numbers from the tokens.
"""
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import shutil

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / 'design/cubbix.pen'
ROUND = ROOT / 'design/rounds/R14-2026-11-04-面板位置与外观'
CLI = ['npx', '-y', '@pen.dev/cli@0.3.13', 'interactive']
BOARDS = [('zh-CN', 'light'), ('zh-CN', 'dark'), ('en', 'light'), ('en', 'dark')]

NEW_TOKENS = [
    'ColorPanelStrokeOuter', 'ColorPanelStrokeInner', 'ColorPanelShadow',
    'ColorGuideLine', 'ColorGuideLineSnap', 'SizePaletteShadowMargin', 'SizePaletteSnap',
]

NUM_KEYS = [
    'Space2', 'Space6', 'Space8', 'Space20', 'SizeIcon', 'SizePaletteWidth', 'SizePaletteHeight',
    'SizePaletteRowHeight', 'SizePaletteGroupHeight', 'SizePaletteSearchHeight',
    'SizePaletteTopOffset', 'SizeRowHeight', 'SizeSegmentHeight',
]

# Canvas-only captions and the runtime keys the boards show.
COPY = [
    ('R14Title', '面板位置与外观', 'Panel position and look'),
    ('R14SchemeA', 'A · 自绘阴影 + 内外双层描边（推荐）', 'A · Drawn shadow, two strokes (recommended)'),
    ('R14SchemeB', 'B · 系统阴影 + 单层描边（现状）', 'B · System shadow, one stroke (today)'),
    ('R14SchemeC', 'C · 材质表面 + 外描边', 'C · Material surface with strokes'),
    ('R14NumbersA', '圆角 12 · 外描边 1 · 内高光 1 · 阴影 0/8/24 · 窗口留白 32',
     'radius 12 · stroke 1 · highlight 1 · shadow 0/8/24 · margin 32'),
    ('R14NumbersB', '圆角 12 · 描边 1 · 系统阴影（半径与透明度不可调）',
     'radius 12 · stroke 1 · system shadow (radius and alpha fixed)'),
    ('R14NumbersC', '圆角 12 · 材质 · 外描边 1 · 内高光 1 · 系统阴影',
     'radius 12 · material · stroke 1 · highlight 1 · system shadow'),
    ('R14NoteA', 'A 的阴影与描边全部由应用自绘，数值来自 Token',
     'A draws its own shadow and strokes, all values from tokens'),
    ('R14NoteB', 'B、C 的阴影由系统绘制，画板里只作示意，调不了半径与透明度',
     'B and C keep the system shadow: the boards only stand in for it'),
    ('R14Guides', '拖动参考线与吸附', 'Guides while dragging'),
    ('R14GuidesBefore', '拖动中：三条参考线出现，面板尚未吸附',
     'Dragging: three guides show, the panel is not snapped'),
    ('R14GuidesAfter', '进入 8pt：该线加粗变实，面板吸附对齐',
     'Within 8pt: that guide thickens, the panel snaps'),
    ('R14GuidesNote', '工作区示意，面板按比例缩小', 'Work area schematic, panel drawn smaller'),
    ('R14Shortcuts', '快捷键页 6 行', 'Shortcuts page, six rows'),
    ('R14ShortcutsNote', '新增「卡片面板」一行：6 × 48 + 5 × 1 = 293',
     'One more row for the menu bar panel: 6 × 48 + 5 × 1 = 293'),
]

LOCALE_KEYS = [
    'Awake', 'JSON', 'R05ShortcutLabel', 'JSONShortcutLabel', 'PaletteShortcutLabel',
    'PanelShortcutLabel', 'PanelShortcutSuffix', 'PanelRecordShortcut', 'FeedbackMode',
    'FeedbackHint', 'FeedbackPosition', 'FeedbackTopRight', 'PaletteSearch', 'R05Start',
    'R05Hour', 'R07NewWindow', 'PaletteAwakeStop', 'PaletteFavorites', 'PaletteRecent',
    'PaletteAll', 'MenuSettings',
]


def run(code, output, save=False):
    commands = 'execute(' + json.dumps({'description': 'Author R14 panel look', 'input': code}) + ')\n'
    commands += ('save()\n' if save else '') + 'exit()\n'
    proc = subprocess.run(CLI + ['--in', str(SOURCE), '--out', str(output)], input=commands,
                          text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, cwd=ROOT)
    if proc.returncode or '[ERROR]' in proc.stdout or 'Error:' in proc.stdout:
        raise SystemExit(proc.stdout[-6000:])
    return proc.stdout


AUTHOR = r'''
const N = __NUM__, S = __STR__;
function add(parent, node) {
  const children = node.children || [];
  delete node.children;
  const id = Insert(parent, node);
  for (const child of children) add(id, child);
  return id;
}
function frame(name, more = {}) { return {type: 'frame', name, layout: 'vertical', gap: 0, ...more}; }
function row(name, more = {}) { return frame(name, {layout: 'horizontal', alignItems: 'center', ...more}); }
function copy(language, key, more = {}) {
  return {type: 'text', name: 'Copy / ' + key, content: S[language][key], fontFamily: '$FontFamily',
          fontSize: '$FontSizeBody', fontWeight: '$FontWeightRegular', lineHeight: '$LineHeightBody',
          fill: '$ColorText', ...more};
}
function literal(content, more = {}) {
  return {type: 'text', name: content, content, fontFamily: '$FontFamily', fontSize: '$FontSizeBody',
          fontWeight: '$FontWeightRegular', lineHeight: '$LineHeightBody', fill: '$ColorText', ...more};
}
function glyph(name, fill = '$ColorTextSecondary') {
  return {type: 'icon', library: 'lucide', icon: name, width: N.SizeIcon, height: N.SizeIcon,
          fill, name: 'Icon / ' + name};
}
function shadow(color, y, blur) {
  return [{type: 'shadow', shadowType: 'outer', color, offset: {x: 0, y}, blur}];
}
// The CLI keeps no dash pattern, so a guide is drawn as the dashes the view
// paints with ui.BorderDashed.
function dashLine(name, x, y, length, vertical, color, width) {
  const dash = 14, gap = 12, period = dash + gap;
  const count = Math.floor((length + gap) / period);
  const children = [];
  for (let i = 0; i < count; i++) {
    children.push(vertical
      ? frame(name + ' dash ' + i, {width, height: dash, fill: color})
      : frame(name + ' dash ' + i, {width: dash, height: width, fill: color}));
  }
  return frame(name, {
    layoutPosition: 'absolute', x, y, width: vertical ? width : length, height: vertical ? length : width,
    layout: vertical ? 'vertical' : 'horizontal', gap, alignItems: vertical ? 'center' : 'start',
    children,
  });
}
function commandRow(language, key, iconName, hint, selected = false) {
  const children = [glyph(iconName), copy(language, key, {width: 'fill_container', textGrowth: 'fixed-width'})];
  if (hint) children.push(copy(language, hint, {fontSize: '$FontSizeSmall', fill: '$ColorTextSecondary'}));
  return row('Command / ' + key, {
    width: 'fill_container', height: N.SizePaletteRowHeight, padding: [0, '$Space8'], gap: '$Space8',
    cornerRadius: '$RadiusCard', fill: selected ? '$ColorAccentSoft' : '$ColorTransparent', children,
  });
}
function groupHeader(language, key) {
  return frame('Group / ' + key, {
    width: 'fill_container', height: N.SizePaletteGroupHeight, padding: [0, '$Space8'], justifyContent: 'center',
    children: [copy(language, key, {fontSize: '$FontSizeCaption', fill: '$ColorTextDisabled'})],
  });
}
// The palette with one scheme's surface, strokes and shadow.
function panelMock(language, scheme) {
  const panel = frame('Palette / ' + scheme, {
    width: N.SizePaletteWidth, height: N.SizePaletteHeight, gap: '$Space8', padding: '$Space8',
    cornerRadius: '$RadiusWindow', clip: true,
    fill: scheme === 'C' ? '$R14Material' : '$ColorMenuSurface',
    stroke: scheme === 'B' ? '$ColorSeparator' : '$ColorPanelStrokeOuter', strokeWidth: '$StrokeHairline',
  });
  panel.effect = scheme === 'A'
    ? shadow('$ColorPanelShadow', N.Space8, 24)
    : shadow('$ColorOverlay', 4, 12);
  const search = row('Search row', {
    width: 'fill_container', height: N.SizePaletteSearchHeight, padding: [0, '$Space8'], gap: '$Space8',
    cornerRadius: '$RadiusControl', fill: '$ColorControl', children: [
      glyph('search'),
      copy(language, 'PaletteSearch', {width: 'fill_container', textGrowth: 'fixed-width', fill: '$ColorTextSecondary'}),
      frame('Action keycap', {
        height: N.Space20, layout: 'horizontal', padding: [0, '$Space6'], justifyContent: 'center',
        alignItems: 'center', cornerRadius: '$RadiusControl', fill: '$ColorControl',
        children: [literal('⌘K', {fontSize: '$FontSizeCaption', fill: '$ColorTextDisabled'})],
      }),
    ],
  });
  const list = frame('Results', {
    width: 'fill_container', height: 'fill_container', children: [
      groupHeader(language, 'PaletteFavorites'),
      commandRow(language, 'R05Start', 'sun', 'R05Hour', true),
      groupHeader(language, 'PaletteRecent'),
      commandRow(language, 'R07NewWindow', 'braces'),
      commandRow(language, 'PaletteAwakeStop', 'power'),
      groupHeader(language, 'PaletteAll'),
      commandRow(language, 'MenuSettings', 'settings-2'),
    ],
  });
  panel.children = [search, frame('Divider', {width: 'fill_container', height: 1, fill: '$ColorSeparator'}), list];
  if (scheme !== 'B') {
    // InsetShadow(0, 0, 0, 1, ColorPanelStrokeInner) in the view: here the same
    // hairline, inset by 1 and rounded by 12 - 1.
    panel.children.push(frame('Inner highlight', {
      layoutPosition: 'absolute', x: 1, y: 1, width: 638, height: 398, cornerRadius: 11,
      stroke: '$ColorPanelStrokeInner', strokeWidth: '$StrokeHairline',
    }));
  }
  return panel;
}
function schemeCell(language, scheme, title, numbers) {
  return frame('Scheme / ' + scheme, {
    width: 704, gap: '$Space12', children: [
      copy(language, title, {fontWeight: '$FontWeightMedium'}),
      copy(language, numbers, {fontSize: '$FontSizeSmall', fill: '$ColorTextSecondary'}),
      frame('Stage / ' + scheme, {
        width: 704, height: 464, layout: 'horizontal', justifyContent: 'center', alignItems: 'center',
        children: [panelMock(language, scheme)],
      }),
    ],
  });
}
// One work area schematic: the three guides and where the panel lands.
function guideCell(language, name, title, snapped) {
  // Only the guides the panel can snap to thicken; the screen middle stays quiet.
  const active = snapped ? '$ColorGuideLineSnap' : '$ColorGuideLine';
  const width = snapped ? 2 : 1;
  const screen = frame('Work area', {
    width: 640, height: 400, cornerRadius: '$RadiusWindow', fill: '$ColorCard',
    stroke: '$ColorSeparator', strokeWidth: '$StrokeHairline', children: [
      frame('Palette outline', {
        layoutPosition: 'absolute', x: snapped ? 160 : 132, y: snapped ? N.SizePaletteTopOffset : 120,
        width: 320, height: 200, cornerRadius: '$RadiusCard', fill: '$ColorMenuSurface',
        stroke: '$ColorPanelStrokeOuter', strokeWidth: '$StrokeHairline',
      }),
      literal('Cubbix', {
        layoutPosition: 'absolute', x: snapped ? 264 : 236, y: snapped ? 186 : 210,
        fontSize: '$FontSizeSmall', fill: '$ColorTextSecondary',
      }),
      // Guides draw behind the panel: its translucent surface faintly shows them.
      dashLine('Guide / screen middle', 0, 200, 640, false, '$ColorGuideLine', 1),
      dashLine('Guide / top offset', 0, N.SizePaletteTopOffset, 640, false, active, width),
      dashLine('Guide / horizontal center', 319, 0, 400, true, active, width),
    ],
  });
  return frame('Guide / ' + name, {
    width: 704, gap: '$Space12', children: [
      copy(language, title, {fontWeight: '$FontWeightMedium'}),
      copy(language, 'R14GuidesNote', {fontSize: '$FontSizeSmall', fill: '$ColorTextSecondary'}),
      frame('Screen stage', {width: 704, height: 440, layout: 'horizontal', justifyContent: 'center', children: [screen]}),
    ],
  });
}
function shortcutsCell(language) {
  const entries = [
    ['R05ShortcutLabel', '⌃⌥⌘K'], ['JSONShortcutLabel', '⌃⌥⌘J'],
    ['PaletteShortcutLabel', '⌥.'], ['PanelShortcutLabel', '⌃⌥⌘P'],
    ['FeedbackMode', 'FeedbackHint'], ['FeedbackPosition', 'FeedbackTopRight'],
  ];
  const rows = [];
  for (let i = 0; i < entries.length; i++) {
    if (i) rows.push(frame('Divider', {width: 'fill_container', height: 1, fill: '$ColorSeparator'}));
    rows.push(row('Shortcut row ' + i, {
      width: 'fill_container', height: N.SizeRowHeight, padding: [0, '$Space16'], gap: '$Space8', children: [
        copy(language, entries[i][0], {width: 'fill_container', textGrowth: 'fixed-width'}),
        frame('Recorder', {
          width: 164, height: N.SizeSegmentHeight, layout: 'horizontal', padding: [0, '$Space12'], gap: N.Space2,
          alignItems: 'center', cornerRadius: '$RadiusControl', fill: '$ColorControl', children: [
            i > 3 ? copy(language, entries[i][1], {fontSize: '$FontSizeSmall', fill: '$ColorTextDisabled'})
                  : literal(entries[i][1], {fontSize: '$FontSizeSmall'}),
          ],
        }),
      ],
    }));
  }
  return frame('Shortcuts / six rows', {
    width: 704, gap: '$Space12', children: [
      copy(language, 'R14Shortcuts', {fontWeight: '$FontWeightMedium'}),
      copy(language, 'R14ShortcutsNote', {fontSize: '$FontSizeSmall', fill: '$ColorTextSecondary'}),
      frame('Settings card', {
        width: 'fill_container', cornerRadius: '$RadiusCard', fill: '$ColorCard',
        stroke: '$ColorSeparator', strokeWidth: '$StrokeHairline', children: rows,
      }),
    ],
  });
}
const roots = Get((n, c) => { c.skipChildren(); return {bounds: c.bounds}; });
const y = Math.max(...roots.map(n => n.bounds.y + n.bounds.height)) + 240;
add(document, frame('R14 round label', {
  x: 0, y, width: 840, height: 32,
  children: [copy('zh-CN', 'R14Title', {fontSize: '$FontSizeTitle', fontWeight: '$FontWeightSemibold'})],
}));
const themes = [['zh-CN', 'light', 0], ['zh-CN', 'dark', 1], ['en', 'light', 2], ['en', 'dark', 3]];
for (const [language, mode, index] of themes) {
  add(document, frame('R14 ' + language + ' ' + mode, {
    x: index * 2336, y: y + 112, width: 2208, gap: '$Space24', padding: '$Space24',
    theme: {language, mode}, fill: '$ColorWindow', clip: true, children: [
      copy(language, 'R14Title', {fontSize: '$FontSizeDisplay', fontWeight: '$FontWeightSemibold'}),
      row('Scheme row', {
        width: 'fill_container', gap: '$Space24', alignItems: 'start', children: [
          schemeCell(language, 'A', 'R14SchemeA', 'R14NumbersA'),
          schemeCell(language, 'B', 'R14SchemeB', 'R14NumbersB'),
          schemeCell(language, 'C', 'R14SchemeC', 'R14NumbersC'),
        ],
      }),
      copy(language, 'R14NoteB', {fontSize: '$FontSizeSmall', fill: '$ColorTextSecondary'}),
      row('Guide row', {
        width: 'fill_container', gap: '$Space24', alignItems: 'start', children: [
          guideCell(language, 'before', 'R14GuidesBefore', false),
          guideCell(language, 'after', 'R14GuidesAfter', true),
          shortcutsCell(language),
        ],
      }),
    ],
  }));
}
Print('R14_CREATED', true);
'''

QA = r'''
const boards = Get((n, c) => { c.skipChildren(); return {id: n.id, name: n.name, theme: n.theme}; })
  .filter(n => /^R14 (zh-CN|en) (light|dark)$/.test(n.name));
const qa = {boards: boards.length, nodes: 0, problems: []};
for (const board of boards) {
  Get(board.id, (n, c) => {
    qa.nodes++;
    if (n.type === 'text' && typeof n.content === 'string' && n.content.startsWith('$')) {
      qa.problems.push({name: n.name, problem: 'Unresolved copy'});
    }
    if ((n.type === 'text' || n.type === 'frame') && typeof n.width === 'string' &&
        n.width.startsWith('$')) {
      qa.problems.push({name: n.name, problem: 'Variable width'});
    }
    if ((n.type === 'text' || n.type === 'frame') && typeof n.height === 'string' &&
        n.height.startsWith('$')) {
      qa.problems.push({name: n.name, problem: 'Variable height'});
    }
    if (c.problems && n.enabled !== false) qa.problems.push({name: n.name, problem: c.problems, bounds: c.bounds});
  }, {resolveInstances: true});
}
Print('R14_QA', qa);
'''


def main():
    ROUND.mkdir(parents=True, exist_ok=True)
    before = json.loads(SOURCE.read_text())
    tokens = json.loads((ROOT / 'design/tokens.json').read_text())['variables']
    locales = {lang: json.loads((ROOT / f'design/locales/{lang}.json').read_text()) for lang in ('zh-CN', 'en')}
    if not any(n.get('name', '').startswith('R14 ') for n in before['children']):
        variables = {key: tokens[key] for key in NEW_TOKENS}
        for key in LOCALE_KEYS:
            variables[key] = {'type': 'string', 'value': [
                {'value': locales[lang][key], 'theme': {'language': lang}} for lang in locales]}
        for key, zh, en in COPY:
            variables[key] = {'type': 'string', 'value': [
                {'value': zh, 'theme': {'language': 'zh-CN'}}, {'value': en, 'theme': {'language': 'en'}}]}
        # A material surface is canvas only: the view takes it from Vibrancy.
        variables['R14Material'] = {'type': 'color', 'value': [
            {'value': '#FFFFFFCC', 'theme': {'mode': 'light'}}, {'value': '#2C2C2CCC', 'theme': {'mode': 'dark'}}]}
        variables = {key: value for key, value in variables.items() if key not in before['variables']}
        strings = {lang: {key: locales[lang][key] for key in LOCALE_KEYS} for lang in locales}
        for lang in strings:
            strings[lang].update({key: zh if lang == 'zh-CN' else en for key, zh, en in COPY})
        numbers = {key: tokens[key]['value'] for key in NUM_KEYS if key in tokens}
        code = 'SetVariables(' + json.dumps(variables, ensure_ascii=False) + ');\n'
        code += AUTHOR.replace('__NUM__', json.dumps(numbers)).replace('__STR__', json.dumps(strings, ensure_ascii=False))
        run(code, SOURCE, save=True)
    after = json.loads(SOURCE.read_text())
    for node in before['children']:
        assert node == next(n for n in after['children'] if n['id'] == node['id']), node['id']
    result = {'cliVersion': '0.3.13', 'historicalRootsUnchanged': True, 'exports': []}
    with tempfile.TemporaryDirectory(prefix='cubbix-r14-') as tmp:
        tmp = Path(tmp)
        qa_path = tmp / 'qa.pen'
        output = run(QA, qa_path)
        qa = next(json.loads(line[len('R14_QA '):]) for line in output.splitlines() if line.startswith('R14_QA '))
        result['nativeLayout'] = qa
        for node in after['children']:
            if node.get('name') not in [f'R14 {lang} {mode}' for lang, mode in BOARDS]:
                continue
            export_dir = tmp / 'exports'
            run('Export([' + json.dumps(node['id']) + '],"png",' + json.dumps(str(export_dir)) + ',{scale:2});',
                tmp / 'reopen.pen')
            filename = node['name'].replace('R14 ', '').replace(' ', '-') + '.png'
            target = ROUND / filename
            shutil.copyfile(export_dir / (node['id'] + '.png'), target)
            result['exports'].append({'nodeId': node['id'], 'file': filename, 'scale': 2,
                                      'sha256': hashlib.sha256(target.read_bytes()).hexdigest()})
    result['passed'] = len(result['exports']) == 4 and not qa['problems']
    (ROUND / 'render-validation.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result, ensure_ascii=False, indent=2))
    if not result['passed']:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
