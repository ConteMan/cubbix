#!/usr/bin/env python3
"""Author and reopen the translation tool window review boards."""
import json
import os
import select
import subprocess
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / 'design/cubbix.pen'
ROUND = ROOT / 'design/rounds/R17-2026-11-10-翻译工具窗口'
CLI = ['npx', '-y', '@pen.dev/cli@0.3.13', 'interactive']


def run(code, save=False):
    process = subprocess.Popen(CLI + ['--in', str(SOURCE), '--out', str(SOURCE)],
                               stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, cwd=ROOT)
    output = bytearray()

    def prompt():
        current = bytearray()
        deadline = time.monotonic() + 90
        while time.monotonic() < deadline:
            if select.select([process.stdout], [], [], 1)[0]:
                chunk = os.read(process.stdout.fileno(), 65536)
                if not chunk:
                    raise SystemExit(bytes(output + current).decode())
                current.extend(chunk)
                if b'>\x1b[22m ' in current:
                    output.extend(current)
                    return
        process.kill()
        raise SystemExit('Headless CLI response timed out')

    prompt()
    process.stdin.write(('execute(' + json.dumps({'description': 'Author translation tool windows', 'input': code}) + ')\n').encode())
    process.stdin.flush()
    prompt()
    if save:
        process.stdin.write(b'save()\n')
        process.stdin.flush()
        prompt()
    process.stdin.write(b'exit()\n')
    process.stdin.flush()
    process.communicate(timeout=15)
    result = output.decode()
    if process.returncode or '[ERROR]' in result or 'Error:' in result:
        raise SystemExit(result)
    return result


def main():
    ROUND.mkdir(parents=True, exist_ok=True)
    before = json.loads(SOURCE.read_text())
    locales = {language: json.loads((ROOT / f'design/locales/{language}.json').read_text())
               for language in ('zh-CN', 'en')}
    variables = json.loads((ROOT / 'design/tokens.json').read_text())['variables']
    code = r'''
const strings = __COPY__;
SetVariables(__VARIABLES__);
function add(parent, node) {
  const children = node.children || []; delete node.children;
  const id = Insert(parent, node); for (const child of children) add(id, child); return id;
}
function frame(name, more = {}) {
  return {type:'frame', name, layout:'vertical', width:'fill_container', ...more};
}
function text(content, more = {}) {
  return {type:'text', name:content, content, fontFamily:'$FontFamily',
    fontSize:'$FontSizeBody', lineHeight:'$LineHeightBody', fill:'$ColorText', ...more};
}
const existing = Get((node, context) => {
  context.skipChildren(); return {id:node.id, name:node.name, bottom:context.bounds.y+context.bounds.height};
});
const top = Math.max(...existing.map(node => node.bottom)) + 240;
for (const [index, [language, mode]] of [['zh-CN','light'],['zh-CN','dark'],['en','light'],['en','dark']].entries()) {
  const previous = existing.find(node => node.name === `R17 ${language} ${mode}`);
  if (previous) {
    Get(previous.id, node => {
      if (node.name === 'Native title area and toolbar') Update(node.id,{padding:['$Space8','$Space12','$Space8','$SizeJSONTitleInset']});
      if (node.name === 'Source input') Update(node.id,{height:36,padding:['$Space4','$Space12'],justifyContent:'center'});
    });
    continue;
  }
  const copy = strings[language];
  const toolbar = frame('Native title area and toolbar', {layout:'horizontal', height:44,
    padding:['$Space8','$Space12','$Space8','$SizeJSONTitleInset'], gap:'$Space8', alignItems:'center'});
  toolbar.children = [text(copy.Translate, {width:'fill_container', textGrowth:'fixed-width'})];
  for (const glyph of ['pin','copy','volume-2']) {
    toolbar.children.push(frame('Toolbar / '+glyph, {width:28,height:28,cornerRadius:'$RadiusControl',
      layout:'horizontal',alignItems:'center',justifyContent:'center',
      children:glyph === 'volume-2' ? [{type:'path',name:'Lucide / volume-2',width:16,height:16,viewBox:[0,0,24,24],geometry:'M11 4.702a.705.705 0 0 0-1.203-.498L6.413 7.587A1.4 1.4 0 0 1 5.416 8H3a1 1 0 0 0-1 1v6a1 1 0 0 0 1 1h2.416a1.4 1.4 0 0 1 .997.413l3.383 3.384A.705.705 0 0 0 11 19.298z M16 9a5 5 0 0 1 0 6 M19.364 18.364a9 9 0 0 0 0-12.728',fill:'$ColorTransparent',stroke:'$ColorTextSecondary',strokeWidth:'$StrokeIcon',strokeLinecap:'round',strokeLinejoin:'round'}] :
        [{type:'ref',name:'Lucide / '+glyph,ref:glyph === 'pin' ? 'vnA76' : 'YHZ39',width:16,height:16}]}));
  }
  add(document, frame(`R17 ${language} ${mode}`, {x:index*608,y:top,width:480,height:360,
    theme:{mode,language},fill:'$ColorCard',cornerRadius:'$RadiusWindow',clip:true,
    children:[toolbar,frame('Toolbar separator',{height:1,fill:'$ColorSeparator'}),
      frame('Content',{padding:'$Space12',gap:'$Space8',children:[
        frame('Source input',{height:36,padding:['$Space4','$Space12'],justifyContent:'center',fill:'$ColorControl',cornerRadius:'$RadiusControl',children:[text('serendipity')]}),
        text(copy.TranslateToChinese,{fontSize:'$FontSizeSmall',fill:'$ColorTextSecondary'}),
        frame('Translation',{height:80,children:[text('意外发现美好事物的幸运。')]}),
        frame('Dictionary separator',{height:1,fill:'$ColorSeparator'}),
        text(copy.TranslateDictionary,{fontSize:'$FontSizeSmall',fill:'$ColorTextSecondary'}),
        text('serendipity | ˌserənˈdipədē | noun\nAn unexpected but fortunate discovery.',
          {fontSize:'$FontSizeSmall',fill:'$ColorTextSecondary',width:'fill_container',textGrowth:'fixed-width'})
      ]})]}));
}
'''.replace('__COPY__', json.dumps(locales, ensure_ascii=False)).replace('__VARIABLES__', json.dumps({key: value for key, value in variables.items() if key.startswith('SizeTranslate')}))
    run(code, save=True)
    after = json.loads(SOURCE.read_text())
    for node in before['children']:
        if not node['name'].startswith('R17 '):
            assert node == next(item for item in after['children'] if item['id'] == node['id'])
    output = run('const boards=Get((node,context)=>{context.skipChildren();return node.name.startsWith("R17 ")?node.id:undefined;});'
                 'const problems=[];for(const id of boards)Get(id,(node,context)=>{if(context.problems)problems.push({name:node.name,problem:context.problems});},{resolveInstances:true});'
                 'Print("R17_QA",{boards:boards.length,problems});Export(boards,"png",' + json.dumps(str(ROUND)) + ',{scale:2});')
    result = next(json.loads(line.removeprefix('R17_QA ')) for line in output.splitlines() if line.startswith('R17_QA '))
    assert result['boards'] == 4 and not result['problems'], result
    for node in after['children']:
        if node['name'].startswith('R17 '):
            (ROUND / (node['id'] + '.png')).replace(ROUND / (node['name'].replace('R17 ', '').replace(' ', '-') + '.png'))
    (ROUND / 'render-validation.json').write_text(json.dumps(result, indent=2) + '\n')
    print(json.dumps(result))


if __name__ == '__main__':
    main()
