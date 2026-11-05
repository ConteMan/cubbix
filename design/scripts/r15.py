#!/usr/bin/env python3
"""Author spec R15's palette polish, then reopen and export it.

The pinned headless CLI keeps no variable binding in text content and no
variable binding for width and height: copy is written as localized text named
`Copy / <key>`, and geometry is resolved to numbers from the tokens.
"""
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import shutil

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / 'design/cubbix.pen'
ROUND = ROOT / 'design/rounds/R15-2026-11-05-命令面板打磨'
CLI = ['npx', '-y', '@pen.dev/cli@0.3.13', 'interactive']
BOARDS = [('zh-CN', 'light'), ('zh-CN', 'dark'), ('en', 'light'), ('en', 'dark')]

NEW_TOKENS = [
    'ColorKeycap', 'ColorKeycapStroke', 'ColorKeycapText',
    'SizeKeycapHeight', 'SizeKeycapMinWidth', 'SizeScrollbar',
    'SizePaletteScrollbarSlot', 'SizePalettePlaceholderInset',
]

NUM_KEYS = [
    'Space2', 'Space4', 'Space6', 'Space8', 'Space12', 'SizeIcon', 'SizeIconSmall',
    'SizePaletteWidth', 'SizePaletteHeight', 'SizePaletteRowHeight', 'SizePaletteGroupHeight',
    'SizePaletteSearchHeight', 'SizeKeycapHeight', 'SizeScrollbar', 'SizePaletteScrollbarSlot',
    'SizePalettePlaceholderInset', 'FontSizeBody',
]

# Canvas-only captions and the runtime keys the boards show.
COPY = [
    ('R15Title', '命令面板打磨', 'Palette polish'),
    ('R15List', '命令列表', 'Command list'),
    ('R15ListNote', '分类跟在标题后面，参数提示留在右侧；滚动条有自己的槽位',
     'The tile follows the title, the hint stays right, the bar has a slot'),
    ('R15Actions', '动作列表', 'Action list'),
    ('R15ActionsNote', '搜索行右上角是两个键帽，动作行不重复标分类',
     'Two keycaps in the search row, no tile on action rows'),
    ('R15Spec', '尺寸与颜色', 'Sizes and colours'),
    ('R15SpecKeys', '键帽：高 20 · 最小宽 20 · 内距 6 · 间隙 4 · 圆角 6',
     'keycap: 20 high, 20 wide at least, 6 in, 4 apart, radius 6'),
    ('R15SpecKeysNote', '颜色取自 ColorKeycap、ColorKeycapStroke、ColorKeycapText',
     'colours from ColorKeycap, ColorKeycapStroke and ColorKeycapText'),
    ('R15SpecCaret', '占位文字内缩 6：光标与第一个字之间留出空隙',
     'placeholder inset 6: a gap between the caret and its first word'),
    ('R15SpecCaretNote', '正文从光标处开始，与占位文字同一行',
     'typed text starts at the caret, on the placeholder\'s line'),
    ('R15SpecSlot', '滚动条槽位 12', 'scroll bar slot 12'),
    ('R15SpecSlotNote', '行右边距 12 = 条宽 6 + 边距 3 + 间隙 3：文字与高亮都不进槽位',
     'rows end 12 before the edge: 6 bar, 3 margins either side, so no text goes under it'),
    ('R15NoDivider', '搜索行与列表之间不再有分隔线', 'no divider between the search row and the list'),
]

LOCALE_KEYS = [
    'R05Start', 'R05Screen', 'R05Tasks', 'R05Half', 'R05Hour', 'R05Two', 'R05Four',
    'R05Unlimited', 'PaletteAwakeStop', 'R07NewWindow', 'MenuSettings', 'R05App', 'JSON',
    'Awake', 'PaletteShortcutLabel', 'PaletteSearch', 'PaletteFavorites', 'PaletteRecent',
    'PaletteAll', 'PaletteResetPosition',
]


def run(code, output, save=False):
    commands = 'execute(' + json.dumps({'description': 'Author R15 palette polish', 'input': code}) + ')\n'
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
function frame(name, more = {}) { return {type: 'frame', name, layout: 'vertical', gap: '$Space0', ...more}; }
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
// The keycap the view draws for one key: its own rounded box, padding Space6,
// the smallest of two keys 20 wide.
function keycap(content, width) {
  return frame('Keycap / ' + content, {
    height: N.SizeKeycapHeight, width: width, layout: 'horizontal', padding: ['$Space0', '$Space6'],
    justifyContent: 'center', alignItems: 'center', cornerRadius: '$RadiusControl',
    fill: '$ColorKeycap', stroke: '$ColorKeycapStroke', strokeWidth: '$StrokeHairline',
    children: [literal(content, {fontSize: '$FontSizeSmall', fill: '$ColorKeycapText'})],
  });
}
function keycapPair() {
  return row('Keycaps', {
    gap: '$Space4', alignItems: 'center', children: [keycap('⌘', 24), keycap('K', 24)],
  });
}
// The search row: icon, caret with the placeholder inset after it, and the
// ⌘K keycaps. No divider follows it.
function searchRow(language) {
  return row('Search row', {
    width: 'fill_container', height: N.SizePaletteSearchHeight, padding: ['$Space0', '$Space8'], gap: '$Space8',
    cornerRadius: '$RadiusControl', fill: '$ColorControl', children: [
      glyph('search'),
      row('Field', {
        width: 'fill_container', height: 'fill_container', gap: '$SizePalettePlaceholderInset', children: [
          frame('Caret', {width: 1, height: 16, fill: '$ColorText'}),
          copy(language, 'PaletteSearch', {width: 'fill_container', textGrowth: 'fixed-width', fill: '$ColorTextSecondary'}),
        ],
      }),
      keycapPair(),
    ],
  });
}
// A command row: icon, title, the tile it belongs to, the hint at the right.
// The row stops SizePaletteScrollbarSlot short of the list's edge.
function commandRow(language, key, iconName, tile, hint, selected = false, showTile = true) {
  const children = [glyph(iconName), copy(language, key)];
  if (showTile) children.push(copy(language, tile, {fill: '$ColorTextSecondary'}));
  children.push(frame('Spacer', {width: 'fill_container', height: 1, fill: '$ColorTransparent'}));
  if (hint) children.push(copy(language, hint, {fontSize: '$FontSizeSmall', fill: '$ColorTextSecondary'}));
  return row('Command / ' + key, {
    width: 'fill_container', height: N.SizePaletteRowHeight, padding: ['$Space0', '$Space8'], gap: '$Space8',
    cornerRadius: '$RadiusCard', fill: selected ? '$ColorAccentSoft' : '$ColorTransparent', children,
  });
}
function groupHeader(language, key) {
  return frame('Group / ' + key, {
    width: 'fill_container', height: N.SizePaletteGroupHeight, padding: ['$Space0', '$Space8'], justifyContent: 'center',
    children: [copy(language, key, {fontSize: '$FontSizeCaption', fill: '$ColorTextDisabled'})],
  });
}
// The list keeps its scroll bar slot on the right; the bar floats in it.
function paletteBody(language, rows) {
  return frame('Body', {
    width: 'fill_container', height: 'fill_container', padding: ['$Space0', '$SizePaletteScrollbarSlot', '$Space0', '$Space0'],
    gap: '$Space0', children: rows,
  });
}
function scrollBar(height) {
  // The list's box ends Space8 inside the panel; the bar sits in its slot, 3
  // DIPs from the list's right edge, just below the search row and its gap.
  return frame('Scroll bar', {
    layoutPosition: 'absolute',
    x: N.SizePaletteWidth - N.Space8 - N.SizeScrollbar - 3,
    y: N.Space8 + N.SizePaletteSearchHeight + N.Space8 + 2,
    width: N.SizeScrollbar, height: height, cornerRadius: '$RadiusControl', fill: '$ColorTextSecondary',
  });
}
function paletteMock(language, mode) {
  const panel = frame('Palette / ' + mode, {
    width: N.SizePaletteWidth, height: N.SizePaletteHeight, gap: '$Space8', padding: '$Space8',
    cornerRadius: '$RadiusWindow', clip: true, fill: '$ColorMenuSurface',
    stroke: '$ColorPanelStrokeOuter', strokeWidth: '$StrokeHairline',
    effect: shadow('$ColorPanelShadow', N.Space8, 24),
  });
  let rows;
  if (mode === 'commands') {
    rows = [
      groupHeader(language, 'PaletteFavorites'),
      commandRow(language, 'PaletteAwakeStop', 'power', 'Awake', '', true),
      groupHeader(language, 'PaletteRecent'),
      commandRow(language, 'R07NewWindow', 'braces', 'JSON', ''),
      commandRow(language, 'MenuSettings', 'settings-2', 'R05App', ''),
      groupHeader(language, 'PaletteAll'),
      commandRow(language, 'R05Start', 'sun', 'Awake', 'R05Hour'),
      commandRow(language, 'R05Screen', 'sun', 'Awake', ''),
      commandRow(language, 'PaletteResetPosition', 'move', 'PaletteShortcutLabel', ''),
    ];
  } else {
    rows = [
      row('Action title', {
        width: 'fill_container', height: N.SizePaletteRowHeight, padding: ['$Space0', '$Space8'], gap: '$Space8',
        children: [glyph('sun'), copy(language, 'R05Start')],
      }),
      frame('Title divider', {width: 'fill_container', height: 1, fill: '$ColorSeparator'}),
      commandRow(language, 'R05Screen', 'sun', 'Awake', '', true, false),
      commandRow(language, 'R05Tasks', 'gauge', 'Awake', '', false, false),
      commandRow(language, 'R05Half', 'clock-3', 'Awake', '', false, false),
      commandRow(language, 'R05Hour', 'clock-3', 'Awake', '', false, false),
      commandRow(language, 'R05Unlimited', 'clock-3', 'Awake', '', false, false),
    ];
  }
  panel.children = [
    searchRow(language),
    paletteBody(language, rows),
    scrollBar(150),
  ];
  panel.children.push(frame('Inner highlight', {
    layoutPosition: 'absolute', x: 1, y: 1, width: N.SizePaletteWidth - 2, height: N.SizePaletteHeight - 2,
    cornerRadius: '$RadiusWindow', stroke: '$ColorPanelStrokeInner', strokeWidth: '$StrokeHairline',
  }));
  return panel;
}
function stage(language, mode) {
  return frame('Stage / ' + mode, {
    width: 704, height: 464, layout: 'horizontal', justifyContent: 'center', alignItems: 'center',
    children: [paletteMock(language, mode)],
  });
}
function cell(language, name, titleKey, noteKey, body) {
  return frame('Cell / ' + name, {
    width: 704, gap: '$Space12', children: [
      copy(language, titleKey, {fontWeight: '$FontWeightMedium'}),
      copy(language, noteKey, {fontSize: '$FontSizeSmall', fill: '$ColorTextSecondary'}),
      body,
    ],
  });
}
// The spec cell: the keycaps, the caret's inset and the scroll bar's slot, each
// drawn at its real size and named by the token it comes from.
function specCell(language) {
  const keys = frame('Keycap spec', {
    width: 'fill_container', gap: '$Space8', padding: '$Space12', cornerRadius: '$RadiusCard',
    fill: '$ColorControl', children: [
      copy(language, 'R15SpecKeys', {fontSize: '$FontSizeSmall', fill: '$ColorTextSecondary'}),
      row('Keycap sample', {gap: '$Space8', children: [keycapPair()]}),
      copy(language, 'R15SpecKeysNote', {fontSize: '$FontSizeSmall', fill: '$ColorTextSecondary'}),
    ],
  });
  const caret = frame('Caret spec', {
    width: 'fill_container', gap: '$Space8', padding: '$Space12', cornerRadius: '$RadiusCard',
    fill: '$ColorControl', children: [
      copy(language, 'R15SpecCaret', {fontSize: '$FontSizeSmall', fill: '$ColorTextSecondary'}),
      row('Caret sample', {
        width: 'fill_container', height: N.SizePaletteSearchHeight, padding: ['$Space0', '$Space8'], gap: '$Space8',
        cornerRadius: '$RadiusControl', fill: '$ColorMenuSurface', children: [
          frame('Caret', {width: 1, height: 16, fill: '$ColorText'}),
          row('Inset', {
            gap: '$SizePalettePlaceholderInset', width: 'fill_container', children: [
              copy(language, 'PaletteSearch', {width: 'fill_container', textGrowth: 'fixed-width', fill: '$ColorTextSecondary'}),
            ],
          }),
        ],
      }),
      copy(language, 'R15SpecCaretNote', {fontSize: '$FontSizeSmall', fill: '$ColorTextSecondary'}),
    ],
  });
  const slot = frame('Slot spec', {
    width: 'fill_container', gap: '$Space8', padding: '$Space12', cornerRadius: '$RadiusCard',
    fill: '$ColorControl', children: [
      copy(language, 'R15SpecSlot', {fontSize: '$FontSizeSmall', fill: '$ColorTextSecondary'}),
      frame('Slot sample', {
        width: 'fill_container', height: N.SizePaletteRowHeight, padding: ['$Space0', '$SizePaletteScrollbarSlot', '$Space0', '$Space0'],
        children: [row('Row sample', {
          width: 'fill_container', height: 'fill_container', padding: ['$Space0', '$Space8'], gap: '$Space8',
          cornerRadius: '$RadiusCard', fill: '$ColorAccentSoft',
          children: [glyph('sun'), copy(language, 'R05Start'), copy(language, 'Awake', {fill: '$ColorTextSecondary'})],
        }), frame('Slot bar', {
          layoutPosition: 'absolute', x: 704 - 2 * N.Space12 - N.SizeScrollbar - 3, y: 5,
          width: N.SizeScrollbar, height: N.SizePaletteRowHeight - 10,
          cornerRadius: '$RadiusControl', fill: '$ColorTextSecondary',
        })],
      }),
      copy(language, 'R15SpecSlotNote', {fontSize: '$FontSizeSmall', fill: '$ColorTextSecondary'}),
      copy(language, 'R15NoDivider', {fontSize: '$FontSizeSmall', fill: '$ColorTextSecondary'}),
    ],
  });
  return frame('Cell / spec', {
    width: 704, gap: '$Space12', children: [
      copy(language, 'R15Spec', {fontWeight: '$FontWeightMedium'}),
      keys, caret, slot,
    ],
  });
}
const roots = Get((n, c) => { c.skipChildren(); return {bounds: c.bounds}; });
const y = Math.max(...roots.map(n => n.bounds.y + n.bounds.height)) + 240;
add(document, frame('R15 round label', {
  x: 0, y, width: 840, height: 32,
  children: [copy('zh-CN', 'R15Title', {fontSize: '$FontSizeTitle', fontWeight: '$FontWeightSemibold'})],
}));
const themes = [['zh-CN', 'light', 0], ['zh-CN', 'dark', 1], ['en', 'light', 2], ['en', 'dark', 3]];
for (const [language, mode, index] of themes) {
  add(document, frame('R15 ' + language + ' ' + mode, {
    x: index * 2336, y: y + 112, width: 2208, gap: '$Space24', padding: '$Space24',
    theme: {language, mode}, fill: '$ColorWindow', clip: true, children: [
      copy(language, 'R15Title', {fontSize: '$FontSizeDisplay', fontWeight: '$FontWeightSemibold'}),
      row('Cell row', {
        width: 'fill_container', gap: '$Space24', alignItems: 'start', children: [
          cell(language, 'commands', 'R15List', 'R15ListNote', stage(language, 'commands')),
          cell(language, 'actions', 'R15Actions', 'R15ActionsNote', stage(language, 'actions')),
          specCell(language),
        ],
      }),
    ],
  }));
}
Print('R15_CREATED', true);
'''

# Repair existing R15 boards as well as keeping AUTHOR token-bound. RadiusControl
# is clamped by the renderer to half the scrollbar's width.
REPAIR = r'''
const boards = Get((n,c) => { c.skipChildren(); return {id:n.id,name:n.name}; })
  .filter(n => n.name.startsWith('R15 '));
for (const board of boards) Get(board.id, (n,c) => {
  const patch = {};
  for (const key of ['gap', 'padding']) {
    function bind(v) {
      if (Array.isArray(v)) return v.map(bind);
      if (v === 0) return '$Space0';
      if (v === 6) return n.name === 'Field' ? '$SizePalettePlaceholderInset' : '$Space6';
      if (v === 12) return '$SizePaletteScrollbarSlot';
      return v;
    }
    if (n[key] !== undefined) {
      const bound = bind(n[key]);
      if (JSON.stringify(bound) !== JSON.stringify(n[key])) patch[key] = bound;
    }
  }
  if (['Field', 'Inset'].includes(n.name)) patch.gap = '$SizePalettePlaceholderInset';
  if (n.cornerRadius === 3) patch.cornerRadius = '$RadiusControl';
  if (Object.keys(patch).length) Update(n.id, patch);
});
'''

QA = r'''
const boards = Get((n, c) => { c.skipChildren(); return {id: n.id, name: n.name, theme: n.theme}; })
  .filter(n => /^R15 (zh-CN|en) (light|dark)$/.test(n.name));
const qa = {boards: boards.length, nodes: 0, problems: []};
for (const board of boards) {
  Get(board.id, (n, c) => {
    qa.nodes++;
    for (const key of ['cornerRadius', 'fontSize', 'gap', 'padding']) {
      const values = Array.isArray(n[key]) ? n[key].flat(Infinity) : [n[key]];
      if (values.some(v => typeof v === 'number')) qa.problems.push({name:n.name, problem:'Literal ' + key});
    }
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
Print('R15_QA', qa);
'''


def walk_nodes(nodes):
    for node in nodes:
        yield node
        yield from walk_nodes(node.get('children', []))


def main():
    ROUND.mkdir(parents=True, exist_ok=True)
    before = json.loads(SOURCE.read_text())
    tokens = json.loads((ROOT / 'design/tokens.json').read_text())['variables']
    locales = {lang: json.loads((ROOT / f'design/locales/{lang}.json').read_text()) for lang in ('zh-CN', 'en')}
    if not any(n.get('name', '').startswith('R15 ') for n in before['children']):
        variables = {}
        for key in NEW_TOKENS:
            variables[key] = tokens[key]
        for key in LOCALE_KEYS:
            variables[key] = {'type': 'string', 'value': [
                {'value': locales[lang][key], 'theme': {'language': lang}} for lang in locales]}
        for key, zh, en in COPY:
            variables[key] = {'type': 'string', 'value': [
                {'value': zh, 'theme': {'language': 'zh-CN'}}, {'value': en, 'theme': {'language': 'en'}}]}
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
    run(REPAIR, SOURCE, save=True)
    after = json.loads(SOURCE.read_text())
    for node in before['children']:
        if not node.get('name', '').startswith('R15 '):
            assert node == next(n for n in after['children'] if n['id'] == node['id']), node['id']
    if any(n.get('name', '').startswith('R15 ') for n in before['children']):
        assert sum(1 for _ in walk_nodes(before['children'])) == sum(1 for _ in walk_nodes(after['children']))
    result = {'cliVersion': '0.3.13', 'historicalRootsUnchanged': True, 'exports': []}
    with tempfile.TemporaryDirectory(prefix='cubbix-r15-') as tmp:
        tmp = Path(tmp)
        qa_path = tmp / 'qa.pen'
        output = run(QA, qa_path)
        qa = next(json.loads(line[len('R15_QA '):]) for line in output.splitlines() if line.startswith('R15_QA '))
        result['nativeLayout'] = qa
        for node in after['children']:
            if node.get('name') not in [f'R15 {lang} {mode}' for lang, mode in BOARDS]:
                continue
            export_dir = tmp / 'exports'
            run('Export([' + json.dumps(node['id']) + '],"png",' + json.dumps(str(export_dir)) + ',{scale:2});',
                tmp / 'reopen.pen')
            filename = node['name'].replace('R15 ', '').replace(' ', '-') + '.png'
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
