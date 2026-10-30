#!/usr/bin/env python3
"""Synchronize feedback tokens and author/reopen R11 with the headless CLI."""
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / 'design/cubbix.pen'
ROUND = ROOT / 'design/rounds/R11-2026-10-31-快捷键反馈'
CLI = ['npx', '-y', '@pen.dev/cli@0.3.13', 'interactive']


def run(code, output, save=False):
    commands = 'execute(' + json.dumps({'description': 'Synchronize R11 feedback designs', 'input': code}) + ')\n'
    commands += ('save()\n' if save else '') + 'exit()\n'
    process = subprocess.run(CLI + ['--in', str(SOURCE), '--out', str(output)],
                             input=commands, text=True, stdout=subprocess.PIPE,
                             stderr=subprocess.STDOUT, cwd=ROOT)
    if process.returncode or '[ERROR]' in process.stdout or 'Error:' in process.stdout:
        raise SystemExit(process.stdout[-8000:])
    return process.stdout


def main():
    ROUND.mkdir(parents=True, exist_ok=True)
    before = json.loads(SOURCE.read_text())
    historical = {n['id']: n for n in before['children'] if not n.get('name', '').startswith('R11')}
    locales = {lang: json.loads((ROOT / f'design/locales/{lang}.json').read_text()) for lang in ('zh-CN', 'en')}
    tokens = json.loads((ROOT / 'design/tokens.json').read_text())['variables']
    variables = {key: value for key, value in tokens.items() if before['variables'].get(key) != value}
    for key in locales['zh-CN']:
        if key.startswith(('Feedback', 'Toast')):
            variables[key] = {'type': 'string', 'value': [
                {'value': locales[lang][key], 'theme': {'language': lang}} for lang in locales]}
    def svg_path(node):
        if 'd' in node.attrib:
            return node.attrib['d']
        if node.tag.endswith('circle'):
            x, y, r = (float(node.attrib[key]) for key in ('cx', 'cy', 'r'))
            return f'M{x-r},{y}a{r},{r} 0 1,0 {2*r},0a{r},{r} 0 1,0 {-2*r},0'
        raise ValueError(f'Unsupported SVG element: {node.tag}')

    paths = {key: [svg_path(node) for node in ET.parse(ROOT / f'design/assets/lucide-0.468.0/{key}.svg').getroot()]
             for key in ('sun', 'power', 'triangle-alert')}
    code = 'const vars=' + json.dumps(variables, ensure_ascii=False) + ';SetVariables(vars);\n'
    if not any(n.get('name', '').startswith('R11') for n in before['children']):
        code += 'const locales=' + json.dumps(locales, ensure_ascii=False) + ';const paths=' + json.dumps(paths) + ';\n' + r'''
const v=GetVariables().variables;
const size=key=>v[key].value;
const roots=Get((n,c)=>{c.skipChildren();return {name:n.name,bounds:c.bounds}});
const y=Math.max(...roots.map(n=>n.bounds.y+n.bounds.height))+240;
function add(parent,node){const children=node.children||[];delete node.children;const id=Insert(parent,node);for(const child of children)add(id,child);return id;}
function frame(name,more={}){return {type:'frame',name,layout:'vertical',gap:'$Space0',...more};}
function txt(content,more={}){return {type:'text',name:content,content,fontFamily:'$FontFamily',fontSize:'$FontSizeBody',fontWeight:'$FontWeightRegular',lineHeight:'$LineHeightBody',fill:'$ColorText',...more};}
function row(parent,name,more={}){return add(parent,frame(name,{layout:'horizontal',width:'fill_container',alignItems:'center',gap:'$Space8',...more}));}
const icons={}, iconPaths={};
for(const [i,key] of ['sun','power','triangle-alert'].entries()){
 const id=add(document,frame('R11 Icon/'+key,{reusable:true,x:i*120,y:y+112,width:size('SizeIcon'),height:size('SizeIcon'),layout:'none',placeholder:true}));
 iconPaths[key]=[];
 for(const path of paths[key])iconPaths[key].push(add(id,{type:'path',name:'Lucide '+key,width:size('SizeIcon'),height:size('SizeIcon'),viewBox:[0,0,24,24],geometry:path,fill:'$ColorTransparent',stroke:'$ColorText',strokeWidth:'$StrokeIcon',strokeLinecap:'round',strokeLinejoin:'round'}));
 Update(id,{placeholder:false});icons[key]=id;
}
function glyph(key,color='$ColorText'){
 const descendants={};for(const id of iconPaths[key])descendants[id]={stroke:color};
 return {type:'ref',ref:icons[key],name:'Icon / '+key,width:size('SizeIcon'),height:size('SizeIcon'),descendants};
}
add(document,frame('R11 round label',{x:0,y,width:840,height:32,children:[txt('R11 · 快捷键反馈 / Shortcut feedback',{fontSize:'$FontSizeTitle',fontWeight:'$FontWeightSemibold'})]}));
function capsule(parent,message,key='sun',color='$ColorText',corner='top-right'){
 const slot=row(parent,'Toast slot / '+corner,{width:size('SizeToastSlotWidth'),height:size('SizeToastHeight'),justifyContent:corner.endsWith('right')?'end':corner==='center'?'center':'start'});
 const pill=row(slot,'Capsule',{width:'fit_content',height:'fill_container',padding:['$Space0','$Space12'],gap:'$Space8',cornerRadius:'$RadiusWindow',fill:'$ColorMenuSurface'});
 add(pill,glyph(key,color));add(pill,txt(message));
}
function settings(parent,mode,lang){
 const text=k=>locales[lang][k],editable=mode==='hint'||mode==='denied',selected=mode==='hint'?0:mode==='off'?2:1;
 const shell=add(parent,frame('Shortcuts / '+mode,{width:720,height:340,layout:'horizontal',fill:'$ColorWindow',cornerRadius:'$RadiusWindow',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline',clip:true}));
 const sidebar=add(shell,frame('Sidebar',{width:144,height:'fill_container',padding:['$Space40','$Space12','$Space12','$Space12'],gap:'$Space16',fill:'$ColorSidebar'}));
 add(sidebar,{type:'ref',ref:'ivtHk',name:'Cubbix brand'});
 for(const [key,active] of [['General',false],['R05Tiles',false],['R05Shortcuts',true],['About',false]]){
  const nav=row(sidebar,'Navigation / '+key,{height:32,padding:'$Space8',cornerRadius:'$RadiusCard',fill:active?'$ColorAccentSoft':'$ColorTransparent'});
  const navIcons={General:'O4KJZC',R05Tiles:'Ectkj',R05Shortcuts:'H8TPWq',About:'uTgqT'};
  add(nav,{type:'ref',ref:navIcons[key],name:'Navigation icon',width:16,height:16});add(nav,txt(text(key)));
 }
 const content=add(shell,frame('Settings content',{width:'fill_container',height:'fill_container',padding:'$Space24'}));
 const card=add(content,frame('Five rows',{width:'fill_container',cornerRadius:'$RadiusCard',fill:'$ColorCard',padding:['$Space0','$Space16'],stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline'}));
 for(const [i,key,shortcut] of [[0,'R05ShortcutLabel','⌃⌥⌘K'],[1,'JSONShortcutLabel','⌃⌥⌘J'],[2,'PaletteShortcutLabel','⌥.'],[3,'FeedbackMode',''],[4,'FeedbackPosition','']]){
  if(i)add(card,frame('Divider',{width:'fill_container',height:1,fill:'$ColorSeparator'}));
  const r=row(card,'Settings row / '+key,{height:size('SizeRowHeight'),...(i===4&&!editable?{opacity:size('OpacityDisabled')}:{})});
  add(r,txt(text(key),{width:'fill_container',textGrowth:'fixed-width'}));
  if(i<3){const recorder=row(r,'Recorder',{width:164,height:size('SizeSegmentHeight'),padding:['$Space0','$Space12'],cornerRadius:'$RadiusControl',fill:'$ColorControl'});add(recorder,txt(shortcut,{fontSize:'$FontSizeSmall'}));}
  else if(i===3){
   const track=row(r,'Feedback mode',{width:size('SizeLanguageSelectorWidth'),height:size('SizeSegmentHeight'),gap:'$Space0',padding:'$Space2',cornerRadius:'$RadiusControl',fill:'$ColorControl'});
   for(const [index,key] of ['FeedbackHint','FeedbackNotification','FeedbackOff'].entries()){
    const part=row(track,'Mode / '+key,{width:'fill_container',height:'fill_container',gap:'$Space2',justifyContent:'center',cornerRadius:'$RadiusControl',fill:index===selected?'$ColorAccent':'$ColorTransparent'});
    add(part,txt(text(key),{fill:index===selected?'$ColorOnAccent':'$ColorText'}));
   }
   if(mode==='denied')add(r,glyph('triangle-alert','$ColorDanger'));
  }else{
   const track=row(r,'Hint positions',{width:size('SizeFeedbackPositionWidth'),height:size('SizeSegmentHeight'),gap:'$Space0',padding:'$Space2',cornerRadius:'$RadiusControl',fill:'$ColorControl'});
   for(const [index,key] of ['FeedbackTopLeft','FeedbackTopRight','FeedbackBottomLeft','FeedbackBottomRight','FeedbackCenter'].entries()){
    const part=row(track,'Position / '+key,{width:'fill_container',height:'fill_container',justifyContent:'center',cornerRadius:'$RadiusControl',fill:index===1?'$ColorAccent':'$ColorTransparent'});
    const screen=add(part,frame('Screen',{width:16,height:16,layout:'none',cornerRadius:'$RadiusControl',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline'}));
    add(screen,{type:'ellipse',name:'Position dot',x:[3,9,3,9,6][index],y:[3,3,9,9,6][index],width:4,height:4,fill:index===1?'$ColorOnAccent':'$ColorText'});
   }
  }
 }
}
for(const [index,lang,mode] of [[0,'zh-CN','light'],[1,'zh-CN','dark'],[2,'en','light'],[3,'en','dark']]){
 const text=k=>locales[lang][k];
 const board=add(document,frame('R11 '+lang+' '+mode,{x:440+index*936,y:y+112,width:816,padding:'$Space24',gap:'$Space24',theme:{language:lang,mode},fill:'$ColorWindow',clip:true,placeholder:true}));
 add(board,txt(lang==='en'?'Shortcut feedback':'快捷键反馈',{fontSize:'$FontSizeDisplay',fontWeight:'$FontWeightSemibold'}));
 const states=[[text('ToastAwakeOn').replace('%s',text('R05Hour')),'sun','$ColorText'],[text('ToastAwakeOn').replace('%s',text('PaletteMinutes').replace('%d','45')),'sun','$ColorText'],[text('ToastAwakeOnUnlimited'),'sun','$ColorText'],[text('ToastAwakeOnUntil').replace('%s','02:30'),'sun','$ColorText'],[text('ToastAwakeOnFollow').replace('%s','Google Chrome'),'sun','$ColorText'],[text('ToastAwakeOff'),'power','$ColorTextSecondary'],[text('ToastAwakeError'),'triangle-alert','$ColorDanger']];
 for(let i=0;i<states.length;i+=2){const line=row(board,'Capsule states',{gap:'$Space24'});for(const [message,key,color] of states.slice(i,i+2))capsule(line,message,key,color);}
 add(board,txt(lang==='en'?'Hint position · work area inset 24':'提示位置 · 工作区边距 24',{fontWeight:'$FontWeightSemibold'}));
 const positions=row(board,'Five positions',{gap:'$Space12'});
 for(const [index,key] of ['FeedbackTopLeft','FeedbackTopRight','FeedbackBottomLeft','FeedbackBottomRight','FeedbackCenter'].entries()){
  const cell=add(positions,frame('Placement / '+key,{width:'fill_container',gap:'$Space8'}));
  add(cell,txt(text(key),{fontSize:'$FontSizeSmall'}));
  const area=add(cell,frame('Work area',{width:'fill_container',height:100,layout:'none',cornerRadius:'$RadiusCard',fill:'$ColorCard',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline'}));
  add(area,frame('Hint marker',{x:[12,86,12,86,49][index],y:[12,12,68,68,40][index],width:44,height:20,cornerRadius:'$RadiusWindow',fill:'$ColorAccent'}));
 }
 for(const state of ['hint','notification','denied','off']){
  const key=state==='denied'?'FeedbackDenied':state==='hint'?'FeedbackHint':state==='off'?'FeedbackOff':'FeedbackNotification';
  add(board,txt(text(key),{fontWeight:'$FontWeightSemibold'}));settings(board,state,lang);
 }
 Update(board,{placeholder:false});
}
Print('R11_CREATED',true);
'''
    run(code, SOURCE, save=True)
    # Keep exported disabled rows visible: the pinned renderer does not resolve
    # variable opacity. Geometry and opacity use resolved token values; paint,
    # typography, radii and spacing stay bound to variables.
    repair = 'const locales=' + json.dumps(locales, ensure_ascii=False) + ';\n' + r"""
const boards=Get((n,c)=>{c.skipChildren();if(/^R11 (zh-CN|en) (light|dark)$/.test(n.name))return {id:n.id,theme:n.theme};});
const navIcons={General:'O4KJZC',R05Tiles:'Ectkj',R05Shortcuts:'H8TPWq',About:'uTgqT'};
for(const board of boards){
 Get(board.id,(n,c)=>{
  if(n.name==='Five rows')Update(n.id,{padding:['$Space0','$Space16']});
  if(n.name?.startsWith('Settings row / '))Update(n.id,{padding:'$Space0',...(n.opacity?{opacity:GetVariables().variables.OpacityDisabled.value}:{})});
  if(n.name?.startsWith('Navigation / ')){
   const key=n.name.slice(13).replace('CopyGeneral','General').replace('CopyAbout','About');
   Update(n.id,{name:'Navigation / '+key});
   const label=n.children.find(child=>child.type==='text');
   if(label)Update(label.id,{content:locales[board.theme.language][key],fontSize:'$FontSizeBody'});
   if(!n.children.some(child=>child.name==='Navigation icon')){
    const id=Insert(n.id,{type:'ref',ref:navIcons[key],name:'Navigation icon',width:16,height:16});
    Move(id,n.id,0);
   }
  }
 });
}
"""
    run(repair, SOURCE, save=True)
    after = json.loads(SOURCE.read_text())
    unchanged = all(next(n for n in after['children'] if n['id'] == key) == value for key, value in historical.items())
    if not unchanged:
        raise SystemExit('Historical canvas changed')
    code = r'''
const boards=Get((n,c)=>{c.skipChildren();if(/^R11 (zh-CN|en) (light|dark)$/.test(n.name))return {id:n.id,name:n.name,theme:n.theme,bounds:c.bounds};});
const qa={nodes:0,problems:[]};
for(const board of boards){Get(board.id,(n,c)=>{qa.nodes++;if(c.problems&&n.enabled!==false)qa.problems.push({name:n.name,problem:c.problems,bounds:c.bounds});},{resolveInstances:true});Export([board.id],'png',OUTPUT,{scale:2});Print('R11_EXPORT',board);}
Print('R11_QA',qa);
'''.replace('OUTPUT', json.dumps(str(ROUND)))
    with tempfile.TemporaryDirectory(prefix='cubbix-r11-') as temporary:
        output = run(code, Path(temporary) / 'reopened.pen')
    records = lambda prefix: [json.loads(line[len(prefix):]) for line in output.splitlines() if line.startswith(prefix)]
    qa = records('R11_QA ')[0]
    exports = []
    for board in records('R11_EXPORT '):
        filename = f'{board["theme"]["language"]}-{board["theme"]["mode"]}.png'
        target = ROUND / filename
        (ROUND / f'{board["id"]}.png').replace(target)
        exports.append({'nodeId': board['id'], 'file': filename, 'scale': 2,
                        'sha256': hashlib.sha256(target.read_bytes()).hexdigest()})
    result = {'cliVersion': '0.3.13', 'historicalRootsUnchanged': unchanged,
              'nativeLayout': qa, 'exports': exports,
              'passed': len(exports) == 4 and not qa['problems'] and unchanged}
    (ROUND / 'render-validation.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result, ensure_ascii=False, indent=2))
    if not result['passed']:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
