#!/usr/bin/env python3
"""Second feedback pass on R15: sync the boards with the polished panel.

The maintainer's second review asked for four things the boards did not show:
the selected highlight symmetric on both sides with the scroll bar in the
panel's own margin, the tile a step smaller than the title and sitting on its
baseline, a search field that reads as a field in the dark theme too, and no
repeated tile name after an unnamed JSON window's title.

Tokens are replaced wholesale: `SetVariables` keeps no delete, and replacing
the whole table retires `SizePaletteScrollbarSlot` in one step. Copy is written
as literal text (`Copy / <key>`), as the pinned CLI keeps no variable binding
in text content.
"""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / 'design/cubbix.pen'
ROUND = ROOT / 'design/rounds/R15-2026-11-05-命令面板打磨'
CLI = ['npx', '-y', '@pen.dev/cli@0.3.13', 'interactive']
BOARDS = [('zh-CN', 'light'), ('zh-CN', 'dark'), ('en', 'light'), ('en', 'dark')]

# Tokens the second pass adds; SizePaletteScrollbarSlot is retired.
NEW_TOKENS = ['ColorPaletteSearch', 'SizePaletteScrollbarOverhang']
RETIRED_TOKENS = ['SizePaletteScrollbarSlot']

TILES = ['Awake', 'JSON', 'R05App', 'PaletteShortcutLabel']

# Copy this pass rewrites, in the variable table and in the text nodes.
COPY = [
    ('R15ListNote',
     '分类比标题小一号、底部对齐；参数提示留在右侧；滚动条不占行宽',
     'The tile is smaller than the title and on its baseline; the hint stays right; the bar takes no row width'),
    ('R15ListNote2',
     '未命名的 JSON 窗口标题就是功能名，分类不再重复它',
     'An unnamed JSON window is titled with its tile name, so the tile is not printed twice'),
    ('R15SpecSlot', '滚动条落在面板外边距里', 'the bar lands in the panel margin'),
    ('R15SpecSlotNote',
     '行宽＝面板内容宽，高亮左右各内缩 Space8；条落在面板外边距里，只在悬停时出现',
     'rows keep the panel content width, the highlight inset Space8 either side; the bar sits in the margin'),
    ('R15SpecTile', '分类比标题小一号并底部对齐', 'the tile is a step smaller, on the title baseline'),
    ('R15SpecTileNote',
     '标题 FontSizeBody 13，分类 FontSizeSmall 12、ColorTextSecondary，两者底部对齐',
     'title FontSizeBody 13, tile FontSizeSmall 12 in ColorTextSecondary, aligned to the title\'s bottom'),
    ('R15SpecField', '搜索框有自己的底色', 'the field has a fill of its own'),
    ('R15SpecFieldNote',
     'ColorPaletteSearch 浅色 #F0F0F0、深色 #464646：在面板表面上仍看得出是输入框，占位文字仍达 AA',
     'ColorPaletteSearch: #F0F0F0 light, #464646 dark, still a field on the panel surface, placeholder still AA'),
]

# Copy the new cards show, from the locale resources.
LOCALE_KEYS = ['JSONWindowName', 'R05Start', 'Awake', 'PaletteSearch']


def run(code, output, save=False):
    commands = 'execute(' + json.dumps({'description': 'Sync the R15 boards with the polished panel', 'input': code}) + ')\n'
    commands += ('save()\n' if save else '') + 'exit()\n'
    proc = subprocess.run(CLI + ['--in', str(SOURCE), '--out', str(output)], input=commands,
                          text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, cwd=ROOT)
    if proc.returncode or '[ERROR]' in proc.stdout or 'Error:' in proc.stdout:
        raise SystemExit(proc.stdout[-6000:])
    return proc.stdout


PATCH = r'''
const N = __NUM__, S = __STR__, COPY = __COPY__, TILES = __TILES__;
const boards = Get((n, c) => { c.skipChildren(); return {id: n.id, name: n.name, theme: n.theme}; })
  .filter(n => /^R15 (zh-CN|en) (light|dark)$/.test(n.name));
const report = {boards: boards.length, bodies: 0, fields: 0, bars: 0, groups: 0, tiles: 0, rows: 0, cards: 0};
function text(language, key, fill, fontSize) {
  return {type: 'text', name: 'Copy / ' + key, content: S[language][key], fill,
          lineHeight: '$LineHeightBody', fontFamily: '$FontFamily', fontSize,
          fontWeight: '$FontWeightRegular'};
}
for (const board of boards) {
  const language = board.theme.language;
  const nodes = Get(board.id, (n) => ({id: n.id, name: n.name, type: n.type, fill: n.fill,
    children: (n.children || []).map(k => ({id: k.id, name: k.name, type: k.type, fill: k.fill}))}));
  const index = (name, from) => nodes.findIndex((n, i) => (from === undefined || i > from) && n.name === name);
  const commands = nodes[index('Cell / commands')];
  const spec = nodes[index('Cell / spec')];
  // The rewritten copy, in the nodes that show it.
  for (const [key, zh, en] of COPY) {
    const node = nodes.find(n => n.name === 'Copy / ' + key);
    if (node) Update(node.id, {content: language === 'zh-CN' ? zh : en});
  }
  for (const node of nodes) {
    // Rows fill the panel's content box: the highlight is symmetric.
    if (node.name === 'Body') {
      Update(node.id, {padding: '$Space0'});
      report.bodies++;
    }
    // The field's own fill, so it separates from the translucent panel.
    if (node.name === 'Search row') {
      Update(node.id, {fill: '$ColorPaletteSearch'});
      report.fields++;
    }
    // The bar sits in the panel's margin, past the rows' right edge: rows end
    // one border and one padding inside the panel, and the bar fills the gap
    // between them and the last three DIPs of the panel.
    if (node.name === 'Scroll bar') {
      Update(node.id, {x: N.SizePaletteWidth - N.Space8 - N.StrokeHairline});
      report.bars++;
    }
    // The slot sample is a strip of panel: its padding is the margin the rows
    // leave on both sides, and the bar sits in it, past the highlight.
    if (node.name === 'Slot sample') {
      Update(node.id, {padding: '$Space8', height: N.SizePaletteRowHeight + 2 * N.Space8,
                       fill: '$ColorMenuSurface', cornerRadius: '$RadiusCard'});
    }
    if (node.name === 'Slot bar') {
      Update(node.id, {x: N.CellWidth - 2 * N.Space12 - N.Space8, y: 13});
    }
    // The tile is a step smaller, on the title's baseline: the two sit in a
    // group aligned by their bottoms, as the view aligns them by baseline.
    if (node.name && node.name.indexOf('Command / ') === 0) {
      if (node.children.some(k => k.name === 'Title group')) continue;
      const tile = node.children.find(k => TILES.some(t => k.name === 'Copy / ' + t));
      if (!tile) continue;
      const title = node.children.find(k => k.type === 'text' && k.fill === '$ColorText');
      if (!title) continue;
      Update(tile.id, {fontSize: '$FontSizeSmall'});
      const group = Insert(node.id, {type: 'frame', name: 'Title group', gap: '$Space8', alignItems: 'end'});
      Move(title.id, group, 0);
      Move(tile.id, group, 1);
      Move(group, node.id, 1);
      report.groups++;
      report.tiles++;
      report.rows++;
    }
  }
  // An unnamed JSON window is titled with its tile's name: the row shows the
  // title alone, next to the note that says why.
  if (!nodes.some(n => n.name === 'Command / JSONWindowName')) {
    const body = nodes[index('Body', index('Palette / commands'))];
    if (!body) throw new Error('no list body in ' + board.name);
    const row = Insert(body.id, {
      type: 'frame', name: 'Command / JSONWindowName', width: 'fill_container',
      height: N.SizePaletteRowHeight, gap: '$Space8', padding: ['$Space0', '$Space8'],
      cornerRadius: '$RadiusCard', fill: '$ColorTransparent', alignItems: 'center',
      children: [
        {type: 'icon', name: 'Icon / file-type', library: 'lucide', icon: 'file-type',
         width: N.SizeIcon, height: N.SizeIcon, fill: '$ColorTextSecondary'},
        text(language, 'JSONWindowName', '$ColorText', '$FontSizeBody'),
      ]});
    Move(row, body.id, (body.children || []).length);
    report.rows++;
  }
  if (commands && !nodes.some(n => n.name === 'Copy / R15ListNote2')) {
    const extra = Insert(commands.id, text(language, 'R15ListNote2', '$ColorTextSecondary', '$FontSizeSmall'));
    Move(extra, commands.id, 2);
  }
  // Two more spec cards: the tile and the field's fill. Rebuilt when present,
  // so a rerun after a copy fix picks the new text up.
  for (const name of ['Tile spec', 'Field spec']) {
    const old = nodes.find(n => n.name === name);
    if (old) Delete(old.id);
  }
  if (spec) {
    const tileCard = Insert(spec.id, {
      type: 'frame', name: 'Tile spec', width: 'fill_container', layout: 'vertical',
      gap: '$Space8', padding: '$Space12', cornerRadius: '$RadiusCard', fill: '$ColorControl',
      children: [
        text(language, 'R15SpecTile', '$ColorTextSecondary', '$FontSizeSmall'),
        {type: 'frame', name: 'Tile sample', width: 'fill_container', height: N.SizePaletteRowHeight,
         padding: ['$Space0', '$Space8'], gap: '$Space8', alignItems: 'center',
         cornerRadius: '$RadiusCard', fill: '$ColorAccentSoft', children: [
           {type: 'icon', name: 'Icon / sun', library: 'lucide', icon: 'sun',
            width: N.SizeIcon, height: N.SizeIcon, fill: '$ColorTextSecondary'},
           {type: 'frame', name: 'Title group', gap: '$Space8', alignItems: 'end', children: [
             text(language, 'R05Start', '$ColorText', '$FontSizeBody'),
             text(language, 'Awake', '$ColorTextSecondary', '$FontSizeSmall'),
           ]},
         ]},
        text(language, 'R15SpecTileNote', '$ColorTextSecondary', '$FontSizeSmall'),
      ]});
    const fieldCard = Insert(spec.id, {
      type: 'frame', name: 'Field spec', width: 'fill_container', layout: 'vertical',
      gap: '$Space8', padding: '$Space12', cornerRadius: '$RadiusCard', fill: '$ColorControl',
      children: [
        text(language, 'R15SpecField', '$ColorTextSecondary', '$FontSizeSmall'),
        {type: 'frame', name: 'Field sample', width: 'fill_container',
         height: N.SizePaletteSearchHeight + 2 * N.Space8, padding: '$Space8',
         layout: 'vertical', cornerRadius: '$RadiusCard', fill: '$ColorMenuSurface', children: [
           {type: 'frame', name: 'Field', width: 'fill_container', height: N.SizePaletteSearchHeight,
            padding: ['$Space0', '$Space8'], gap: '$Space8', alignItems: 'center',
            cornerRadius: '$RadiusControl', fill: '$ColorPaletteSearch', children: [
              {type: 'icon', name: 'Icon / search', library: 'lucide', icon: 'search',
               width: N.SizeIcon, height: N.SizeIcon, fill: '$ColorTextSecondary'},
              text(language, 'PaletteSearch', '$ColorTextSecondary', '$FontSizeBody'),
            ]},
         ]},
        text(language, 'R15SpecFieldNote', '$ColorTextSecondary', '$FontSizeSmall'),
      ]});
    Move(tileCard, spec.id, 3);
    Move(fieldCard, spec.id, 4);
    report.cards += 2;
  }
}
Print('R15_FIX', report);
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
      if (values.some(v => typeof v === 'number')) qa.problems.push({name: n.name, problem: 'Literal ' + key});
    }
    if (n.type === 'text' && typeof n.content === 'string' && n.content.startsWith('$')) {
      qa.problems.push({name: n.name, problem: 'Unresolved copy'});
    }
    if ((n.type === 'text' || n.type === 'frame') && typeof n.width === 'string' && n.width.startsWith('$')) {
      qa.problems.push({name: n.name, problem: 'Variable width'});
    }
    if ((n.type === 'text' || n.type === 'frame') && typeof n.height === 'string' && n.height.startsWith('$')) {
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
        raise SystemExit('the R15 boards are missing: run design/scripts/r15.py first')

    variables = {key: value for key, value in before['variables'].items() if key not in RETIRED_TOKENS}
    for key in NEW_TOKENS:
        variables[key] = tokens[key]
    for key, zh, en in COPY:
        variables[key] = {'type': 'string', 'value': [
            {'value': zh, 'theme': {'language': 'zh-CN'}}, {'value': en, 'theme': {'language': 'en'}}]}
    for key in LOCALE_KEYS:
        variables[key] = {'type': 'string', 'value': [
            {'value': locales[lang][key], 'theme': {'language': lang}} for lang in locales]}
    numbers = {key: tokens[key]['value'] for key in
               ['Space0', 'Space8', 'Space12', 'SizeIcon', 'SizeScrollbar', 'SizePaletteWidth',
                'SizePaletteRowHeight', 'SizePaletteSearchHeight', 'StrokeHairline']}
    numbers['CellWidth'] = 704
    strings = {lang: {key: locales[lang][key] for key in LOCALE_KEYS} for lang in locales}
    for key, zh, en in COPY:
        for lang in strings:
            strings[lang][key] = zh if lang == 'zh-CN' else en
    code = 'SetVariables(' + json.dumps(variables, ensure_ascii=False) + ', true);\n'
    code += PATCH.replace('__NUM__', json.dumps(numbers)) \
        .replace('__STR__', json.dumps(strings, ensure_ascii=False)) \
        .replace('__COPY__', json.dumps(COPY, ensure_ascii=False)) \
        .replace('__TILES__', json.dumps(TILES))
    run(code, SOURCE, save=True)

    after = json.loads(SOURCE.read_text())
    assert set(after['variables']) == set(variables), 'unexpected variables'
    for key, value in variables.items():
        assert after['variables'][key] == value, key
    for node in before['children']:
        if not node.get('name', '').startswith('R15 '):
            assert node == next(n for n in after['children'] if n['id'] == node['id']), node['id']

    result = {'cliVersion': '0.3.13', 'historicalRootsUnchanged': True, 'exports': []}
    with tempfile.TemporaryDirectory(prefix='cubbix-r15-fix-') as tmp:
        tmp = Path(tmp)
        output = run(QA, tmp / 'qa.pen')
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
