#!/usr/bin/env python3
"""Author and reopen R10 through the pinned headless pen.dev CLI."""
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[2]
ROUND = ROOT / 'design/rounds/R10-2026-10-30-卡片面板'
SOURCE = ROOT / 'design/cubbix.pen'
CLI = ['npx', '-y', '@pen.dev/cli@0.3.13', 'interactive']


def run(code, output, save=False):
    commands = 'execute(' + json.dumps({'description': 'Synchronize R10 card panel designs', 'input': code}) + ')\n'
    commands += ('save()\n' if save else '') + 'exit()\n'
    proc = subprocess.run(CLI + ['--in', str(SOURCE), '--out', str(output)], input=commands,
                          text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, cwd=ROOT)
    if proc.returncode or '[ERROR]' in proc.stdout or 'Error:' in proc.stdout:
        raise SystemExit(proc.stdout[-6000:])
    return proc.stdout


def main():
    ROUND.mkdir(parents=True, exist_ok=True)
    before = json.loads(SOURCE.read_text())
    historical = {n['id']: n for n in before['children'] if not n.get('name', '').startswith('R10')}
    locales = {lang: json.loads((ROOT / f'design/locales/{lang}.json').read_text()) for lang in ('zh-CN', 'en')}
    version = re.search(r'const appVersion = "([^"]+)"', (ROOT / 'main.go').read_text())[1]
    paths = {key: [n.attrib['d'] for n in ET.parse(ROOT / f'design/assets/lucide-0.468.0/{key}.svg').getroot()]
             for key in ('plus', 'command')}
    if not any(n.get('name', '').startswith('R10') for n in before['children']):
        code = 'const locales=' + json.dumps(locales, ensure_ascii=False) + ';const paths=' + json.dumps(paths) + ';const version=' + json.dumps(version) + ';\n' + r'''
const roots=Get((n,c)=>{c.skipChildren();return {name:n.name,bounds:c.bounds}});
const y=Math.max(...roots.map(n=>n.bounds.y+n.bounds.height))+240;
function add(parent,node){const children=node.children||[];delete node.children;const id=Insert(parent,node);for(const child of children)add(id,child);return id;}
function frame(name,more={}){return {type:'frame',name,layout:'vertical',gap:'$Space0',...more};}
function txt(content,more={}){return {type:'text',name:content,content,fontFamily:'$FontFamily',fontSize:'$FontSizeBody',fontWeight:'$FontWeightRegular',lineHeight:'$LineHeightBody',fill:'$ColorText',...more};}
const icons={sun:'lPkvC',json:'Hvppf',file:'iakjU',pin:'vnA76',alert:'sWO1O',right:'MTEP9',down:'Lq8Qv',settings:'O4KJZC',power:'s4LWGi'};
let iconX=0;
for(const key of ['plus','command']){
 const id=add(document,frame('R10 Icon/'+key,{reusable:true,x:iconX,y:y+112,width:'$SizeIcon',height:'$SizeIcon',layout:'none',placeholder:true}));
 for(const path of paths[key])add(id,{type:'path',name:'Lucide '+key,width:'$SizeIcon',height:'$SizeIcon',viewBox:[0,0,24,24],geometry:path,fill:'$ColorTransparent',stroke:'$ColorTextSecondary',strokeWidth:'$StrokeIcon',strokeLinecap:'round',strokeLinejoin:'round'});
 Update(id,{placeholder:false});icons[key]=id;iconX+=120;
}
function glyph(name,more={}){return {type:'ref',ref:icons[name],name:'Icon / '+name,width:'$SizeIcon',height:'$SizeIcon',...more};}
const switchID=add(document,frame('R10 Switch',{reusable:true,x:240,y:y+112,width:'$SizeSwitchWidth',height:'$SizeSwitchHeight',layout:'horizontal',padding:'$Space2',justifyContent:'end',alignItems:'center',cornerRadius:'$RadiusControl',fill:'$ColorAccent',placeholder:true,children:[{type:'ellipse',name:'Thumb',width:'$SizeSwitchThumb',height:'$SizeSwitchThumb',fill:'$ColorThumb'}]}));
Update(switchID,{placeholder:false});
add(document,frame('R10 round label',{x:0,y,width:840,height:'$SizeNavHeight',children:[txt('R10 · 卡片面板 / Card panel',{fontSize:'$FontSizeTitle',fontWeight:'$FontWeightSemibold'})]}));
const scenarios=['off','30m','1h','unlimited','until','custom','follow','error','json0','json3','json7','expanded','empty','context'];
const captions={off:['未开启','Off'],'30m':['30 分钟','30 minutes'],'1h':['1 小时','1 hour'],unlimited:['不限时','Unlimited'],until:['截止时刻','Until'],custom:['自定义时长','Custom duration'],follow:['跟随进程','Following process'],error:['目标进程结束','Process ended'],json0:['JSON · 无窗口','JSON · No windows'],json3:['JSON · 3 个窗口','JSON · 3 windows'],json7:['JSON · 7 个窗口折叠','JSON · 7 windows folded'],expanded:['JSON · 7 个窗口展开','JSON · 7 windows expanded'],empty:['全部格子关闭','All tiles disabled'],context:['右键 · 更新就绪','Context · Update ready']};
function panel(parent,state,lang){
 const text=k=>locales[lang][k];
 const shell=add(parent,frame('Panel / '+state,{width:'$SizeMenubarPanelWidth',padding:'$Space12',gap:'$Space8',cornerRadius:'$RadiusWindow',fill:'$ColorMenuSurface',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline'}));
 const row=(parent,name,more={})=>add(parent,frame(name,{layout:'horizontal',width:'fill_container',height:'$SizeNavHeight',gap:'$Space8',alignItems:'center',...more}));
 const label=(parent,key,more={})=>add(parent,txt(text(key),{name:'Copy / '+key,...more}));
 const button=(parent,name,icon)=>{const b=add(parent,frame(name,{layout:'horizontal',width:'$SizeIconButton',height:'$SizeIconButton',alignItems:'center',justifyContent:'center',cornerRadius:'$RadiusControl'}));add(b,glyph(icon));};
 const card=(name)=>add(shell,frame(name,{width:'fill_container',padding:'$Space10',gap:'$Space8',cornerRadius:'$RadiusCard',fill:'$ColorCard',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline'}));
 if(state==='context'){
  for(const key of ['MenuSettings','MenuRestartUpdate','Quit']){const r=row(shell,'Context action / '+key);label(r,key);}
  return;
 }
 if(!state.startsWith('json')&&state!=='expanded'&&state!=='empty'){
  const active=!['off','error'].includes(state),tasks=state==='follow';
  const awake=card('Keep awake card');const title=row(awake,'Awake title');add(title,glyph('sun'));label(title,'Awake',{width:'fill_container',textGrowth:'fixed-width',fontWeight:'$FontWeightSemibold'});
  add(title,{type:'ref',ref:switchID,name:'Master switch',...(active?{}:{justifyContent:'start',fill:'$ColorControl',descendants:{Thumb:{fill:'$ColorTextSecondary'}}})});
  const modes=row(awake,'Modes',{height:'$SizeSegmentHeight',padding:'$Space2',gap:'$Space0',cornerRadius:'$RadiusControl',fill:'$ColorControl'});
  for(const [i,key] of ['R05Screen','R05Tasks'].entries()){
   const selected=tasks?i===1:i===0;const part=add(modes,frame('Mode / '+key,{layout:'horizontal',width:'fill_container',height:'fill_container',justifyContent:'center',alignItems:'center',cornerRadius:'$RadiusControl',fill:selected?'$ColorAccent':'$ColorTransparent'}));
   label(part,key,{fill:selected?'$ColorOnAccent':'$ColorText'});
  }
  const pills=row(awake,'Duration pills',{height:'$SizeButtonHeight',gap:'$Space6'});
  for(const [value,key] of [['30m','PanelDuration30m'],['1h','PanelDuration1h'],['2h','PanelDuration2h'],['unlimited','PanelDurationInf']]){
   const selected=state===value||state==='off'&&value==='1h';
   const pill=add(pills,frame('Pill / '+value,{layout:'horizontal',width:'fill_container',height:'fill_container',justifyContent:'center',alignItems:'center',cornerRadius:'$RadiusControl',fill:selected?(active?'$ColorAccent':'$ColorAccentSoft'):'$ColorControl'}));label(pill,key,{fill:selected&&active?'$ColorOnAccent':'$ColorText'});
  }
  if(state!=='off'){
   const status=row(awake,'Status / '+state,{height:'$Space20',gap:'$Space6'});
   if(state==='error')add(status,glyph('alert',{descendants:{QdblK:{stroke:'$ColorDanger'}}}));
   else {const mode=text(tasks?'R05Tasks':'R05Screen');const suffix=state==='until'?text('AwakeUntil').replace('%s','18:30'):state==='follow'?text('AwakeFollowing').replace('%s','codex'):state==='unlimited'?text('R05Unlimited'):text('AwakeRemaining').replace('%d',state==='30m'?'0':state==='custom'?'3':'1').replace('%02d',state==='30m'?'30':state==='custom'?'45':'00');add(status,txt(mode+' · '+suffix,{fontSize:'$FontSizeSmall',fill:'$ColorTextSecondary'}));}
  }
 }
 if(state!=='empty'){
  const json=card('JSON card');const title=row(json,'JSON title');add(title,glyph('json'));label(title,'JSON',{width:'fill_container',textGrowth:'fixed-width',fontWeight:'$FontWeightSemibold'});button(title,'Quick new JSON','plus');
  const count=state==='json3'?3:['json7','expanded'].includes(state)?7:0;
  if(count){const list=add(json,frame('JSON window list',{width:'fill_container',gap:'$Space2'}));
   if(count>=6){const fold=row(list,'Window count',{padding:['$Space0','$Space8']});add(fold,glyph(state==='expanded'?'down':'right'));add(fold,txt(text('PanelJSONWindowsCount').replace('%d','7')));}
   if(state!=='json7')for(const [i,name] of ['orders.json','settings.json','response.json','items-1.json','items-2.json','draft.json','payload.json'].slice(0,count).entries()){
    const r=row(list,'Window / '+name,{padding:['$Space0','$Space8'],cornerRadius:'$RadiusControl'});add(r,glyph('file'));add(r,txt(name,{width:'fill_container',textGrowth:'fixed-width'}));if(i===0)add(r,glyph('pin',{width:'$SizeIconSmall',height:'$SizeIconSmall'}));
   }
  }
  const action=row(json,'New JSON',{height:'$SizeButtonHeight',gap:'$Space6',justifyContent:'center',cornerRadius:'$RadiusControl',fill:'$ColorControl'});add(action,glyph('plus'));label(action,'JSONShortcutLabel');
 }else{const r=row(shell,'Empty',{justifyContent:'center'});label(r,'PanelEmpty',{fontSize:'$FontSizeSmall',fill:'$ColorTextDisabled'});}
 const footer=add(shell,frame('Footer',{width:'fill_container'}));add(footer,frame('Divider',{width:'fill_container',height:'$StrokeHairline',fill:'$ColorSeparator'}));const bar=row(footer,'System actions',{gap:'$Space0'});add(bar,txt('Cubbix v'+version,{fontSize:'$FontSizeCaption',fill:'$ColorTextDisabled',width:'fill_container',textGrowth:'fixed-width'}));for(const key of ['command','settings','power'])button(bar,'System / '+key,key);
}
for(const [index,lang,mode] of [[0,'zh-CN','light'],[1,'zh-CN','dark'],[2,'en','light'],[3,'en','dark']]){
 const board=add(document,frame('R10 '+lang+' '+mode,{x:440+index*1520,y:y+112,width:1400,padding:'$Space24',gap:'$Space24',theme:{language:lang,mode},fill:'$ColorWindow',clip:true,placeholder:true}));
 add(board,txt(lang==='en'?'Card panel · states and actions':'卡片面板 · 状态与操作',{fontSize:'$FontSizeDisplay',fontWeight:'$FontWeightSemibold'}));
 for(let i=0;i<scenarios.length;i+=4){const line=add(board,frame('Review row',{layout:'horizontal',width:'fill_container',gap:'$Space24'}));for(const state of scenarios.slice(i,i+4)){const cell=add(line,frame('Review / '+state,{width:'$SizeMenubarPanelWidth',gap:'$Space12'}));add(cell,txt(captions[state][lang==='en'?1:0],{fontWeight:'$FontWeightMedium'}));panel(cell,state,lang);}}
 Update(board,{placeholder:false});
}
Print('R10_CREATED',true);
'''
        run(code, SOURCE, save=True)
    # The pinned renderer treats variable dimensions as sizing strings. Resolve
    # geometry only; all paint, radius, typography and spacing remain bound.
    repair = r"""
const vars=GetVariables().variables;
const roots=Get((n,c)=>{c.skipChildren();if(n.name?.startsWith('R10'))return n.id;});
function dimensions(n){
 const name=n.name||'';
 if(/^R10 Icon\//.test(name)||/^Lucide /.test(name))return {width:name.startsWith('Lucide pin')?vars.SizeIconSmall.value:vars.SizeIcon.value,height:name.startsWith('Lucide pin')?vars.SizeIconSmall.value:vars.SizeIcon.value};
 if(name==='R10 Switch')return {width:vars.SizeSwitchWidth.value,height:vars.SizeSwitchHeight.value};
 if(name==='Thumb')return {width:vars.SizeSwitchThumb.value,height:vars.SizeSwitchThumb.value};
 if(name==='R10 round label')return {height:32};
 if(name.startsWith('Review / ')||name.startsWith('Panel / '))return {width:vars.SizeMenubarPanelWidth.value};
 if(name.startsWith('Icon / '))return {width:name.endsWith('pin')?vars.SizeIconSmall.value:vars.SizeIcon.value,height:name.endsWith('pin')?vars.SizeIconSmall.value:vars.SizeIcon.value};
 if(name==='Master switch')return {width:vars.SizeSwitchWidth.value,height:vars.SizeSwitchHeight.value};
 if(name==='Quick new JSON'||name.startsWith('System / '))return {width:28,height:28};
 if(['Awake title','JSON title','Window count','Empty','System actions'].includes(name)||name.startsWith('Window / ')||name.startsWith('Context action / '))return {height:32};
 if(['Modes','Duration pills','New JSON'].includes(name))return {height:28};
 if(name.startsWith('Status / '))return {height:20};
 if(name==='Divider')return {height:1};
 return {};
}
for(const root of roots)Get(root,(n,c)=>{const patch=dimensions(n);if(Object.keys(patch).length)Update(n.id,patch);},{resolveInstances:true});
"""
    run(repair, SOURCE, save=True)
    after = json.loads(SOURCE.read_text())
    unchanged = all(next(n for n in after['children'] if n['id'] == key) == value for key, value in historical.items())
    if not unchanged:
        raise SystemExit('Historical canvas changed')
    code = r'''
const boards=Get((n,c)=>{c.skipChildren();if(/^R10 (zh-CN|en) (light|dark)$/.test(n.name))return {id:n.id,name:n.name,theme:n.theme,bounds:c.bounds};});
const qa={nodes:0,problems:[]};
for(const board of boards){Get(board.id,(n,c)=>{qa.nodes++;if(c.problems&&n.enabled!==false)qa.problems.push({name:n.name,problem:c.problems,bounds:c.bounds});},{resolveInstances:true});Export([board.id],'png',OUTPUT,{scale:2});Print('R10_EXPORT',board);}
Print('R10_QA',{nodes:qa.nodes,problemCount:qa.problems.length,problems:qa.problems.slice(0,20)});
'''.replace('OUTPUT', json.dumps(str(ROUND)))
    with tempfile.TemporaryDirectory(prefix='cubbix-r10-') as temporary:
        output = run(code, Path(temporary) / 'reopened.pen')
    records = lambda prefix: [json.loads(line[len(prefix):]) for line in output.splitlines() if line.startswith(prefix)]
    qa = records('R10_QA ')[0]
    exports = []
    for board in records('R10_EXPORT '):
        filename = f'{board["theme"]["language"]}-{board["theme"]["mode"]}.png'
        target = ROUND / filename
        (ROUND / f'{board["id"]}.png').replace(target)
        exports.append({'nodeId': board['id'], 'file': filename, 'scale': 2,
                        'sha256': hashlib.sha256(target.read_bytes()).hexdigest()})
    result = {'cliVersion': '0.3.13', 'historicalRootsUnchanged': unchanged,
              'nativeLayout': qa, 'exports': exports, 'passed': len(exports) == 4 and not qa['problems'] and unchanged}
    (ROUND / 'render-validation.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result, ensure_ascii=False, indent=2))
    if not result['passed']:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
