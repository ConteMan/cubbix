#!/usr/bin/env python3
"""Author R09 through the headless CLI, then reopen and export review boards."""
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[2]
ROUND = ROOT / 'design/rounds/R09-2026-10-30-命令面板'
SOURCE = ROOT / 'design/cubbix.pen'
CLI = ['npx', '-y', '@pen.dev/cli@0.3.13', 'interactive']


def run(code, output, save=False):
    commands = 'execute(' + json.dumps({'description': 'R09 command palette design', 'input': code}) + ')\n'
    commands += ('save()\n' if save else '') + 'exit()\n'
    proc = subprocess.run(CLI + ['--in', str(SOURCE), '--out', str(output)], input=commands,
                          text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, cwd=ROOT)
    if proc.returncode or '[ERROR]' in proc.stdout or 'Error:' in proc.stdout:
        raise SystemExit(proc.stdout[-6000:])
    return proc.stdout


def main():
    ROUND.mkdir(parents=True, exist_ok=True)
    source = json.loads(SOURCE.read_text())
    historical = {n['id']: n for n in source['children'] if not n.get('name', '').startswith('R09')}
    if not any(n.get('name', '').startswith('R09') for n in source['children']):
        tokens = json.loads((ROOT / 'design/tokens.json').read_text())['variables']
        locales = {lang: json.loads((ROOT / f'design/locales/{lang}.json').read_text()) for lang in ('zh-CN', 'en')}
        variables = {key: value for key, value in tokens.items() if key not in source['variables']}
        for key in locales['zh-CN']:
            if key.startswith('Palette') or key in ('DurationError', 'R05ShortcutLabel', 'JSONShortcutLabel'):
                variables[key] = {'type': 'string', 'value': [
                    {'value': locales[lang][key], 'theme': {'language': lang}} for lang in locales]}
        # Canvas-only captions are localized independently of runtime copy.
        for key, zh, en in [
            ('R09Title', '命令面板', 'Command palette'),
            ('R09List', '01 · 收藏与最近', '01 · Favorites and recents'),
            ('R09Search', '02 · 搜索窗口', '02 · Search windows'),
            ('R09Params', '03 · 参数整段匹配', '03 · Whole argument'),
            ('R09Actions', '04 · ⌘K 动作', '04 · ⌘K actions'),
            ('R09Empty', '05 · 空结果', '05 · No matches'),
            ('R09Invalid', '06 · 参数错误', '06 · Invalid argument'),
            ('R09Settings', '07 · 快捷键与冲突', '07 · Shortcuts and conflicts'),
            ('R09Query', '常亮 1h30m', 'awake 1h30m'),
            ('R09Ninety', '90 分钟', '90 min'),
            ('R09InvalidQuery', '常亮 45x', 'awake 45x'),
            ('R09Feedback', '反馈方式', 'Feedback'),
            ('R09Position', '提示位置', 'Hint position'),
            ('R09FeedbackValue', '屏幕提示', 'Screen hint'),
            ('R09PositionValue', '右上', 'Top right'),
        ]:
            variables[key] = {'type': 'string', 'value': [{'value': zh, 'theme': {'language': 'zh-CN'}}, {'value': en, 'theme': {'language': 'en'}}]}
        code = 'SetVariables(' + json.dumps(variables, ensure_ascii=False) + ');\n' + r'''
const roots=Get((n,c)=>{c.skipChildren();return {id:n.id,name:n.name,bounds:c.bounds}});
const bottom=Math.max(...roots.map(n=>n.bounds.y+n.bounds.height));
const y=bottom+240;
function add(parent,node){const children=node.children||[];delete node.children;const id=Insert(parent,node);for(const child of children)add(id,child);return id;}
function txt(content,more={}){return {type:'text',name:content,content,fontFamily:'$FontFamily',fontSize:'$FontSizeBody',fontWeight:'$FontWeightRegular',lineHeight:'$LineHeightBody',fill:'$ColorText',...more};}
function frame(name,more={}){return {type:'frame',name,layout:'vertical',gap:'$Space0',...more};}
const icons={sun:'lPkvC',power:'s4LWGi',json:'Hvppf',settings:'O4KJZC',search:'GgZ43',clock:'tyiHF',tasks:'Faq6Y',file:'iakjU',alert:'sWO1O',check:'xgZOR'};
function glyph(name){return {type:'ref',ref:icons[name],name:'Icon / '+name,width:16,height:16,...(name==='alert'?{descendants:{QdblK:{stroke:'$ColorDanger'}}}:{})};}
function row(parent,title,icon='sun',hint='',selected=false,checked=false){const r=add(parent,frame('Command / '+title,{layout:'horizontal',width:'fill_container',height:36,padding:['$Space0','$Space8'],gap:'$Space8',alignItems:'center',cornerRadius:'$RadiusCard',fill:selected?'$ColorAccentSoft':'$ColorTransparent'}));add(r,glyph(icon));add(r,txt(title,{width:'fill_container',textGrowth:'fixed-width'}));if(hint)add(r,txt(hint,{fontSize:'$FontSizeSmall',fill:'$ColorTextSecondary'}));if(checked)add(r,glyph('check'));return r;}
function header(parent,title){add(parent,frame('Group / '+title,{width:'fill_container',height:24,padding:['$Space0','$Space8'],justifyContent:'center',children:[txt(title,{fontSize:'$FontSizeCaption',fill:'$ColorTextDisabled'})]}));}
function divider(parent){add(parent,frame('Divider',{width:'fill_container',height:1,fill:'$ColorSeparator'}));}
const panel=add(document,frame('R09 Palette shell',{x:0,y:y+112,reusable:true,placeholder:true,width:640,height:400,padding:'$Space8',gap:'$Space8',cornerRadius:'$RadiusWindow',fill:'$ColorMenuSurface',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline',clip:true}));
const search=add(panel,frame('Search row',{layout:'horizontal',width:'fill_container',height:36,padding:['$Space0','$Space8'],gap:'$Space8',alignItems:'center',cornerRadius:'$RadiusControl',fill:'$ColorControl'}));
add(search,glyph('search'));
const query=add(search,txt('$PaletteSearch',{name:'Query',width:'fill_container',textGrowth:'fixed-width',fill:'$ColorTextSecondary'}));
const error=add(search,glyph('alert'));Update(error,{enabled:false});
add(search,frame('Action keycap',{height:20,layout:'horizontal',padding:['$Space0','$Space6'],justifyContent:'center',alignItems:'center',cornerRadius:'$RadiusControl',fill:'$ColorControl',children:[txt('⌘K',{fontSize:'$FontSizeCaption',fill:'$ColorTextDisabled'})]}));
divider(panel);
const list=add(panel,frame('Results',{width:'fill_container',height:'fill_container'}));
Update(panel,{placeholder:false});
const variants={};
const make=(name,x,build)=>{
 const id=Insert(document,{type:'ref',ref:panel,name:'R09 '+name,x,y:y+112,placeholder:true});
 build(id);Update(id,{placeholder:false});variants[name]=id;return id;
};
function setQuery(id,value,invalid=false){Update(id+'/'+query,{content:value,fill:'$ColorText'});Update(id+'/'+error,{enabled:invalid});}
function results(id,build){const r=Replace(id+'/'+list,frame('Results',{width:'fill_container',height:'fill_container'}));build(r);}
make('Commands',760,id=>results(id,r=>{header(r,'$PaletteFavorites');row(r,'$R05Start','sun','$R05Hour',true);row(r,'$R07NewWindow','json');header(r,'$PaletteRecent');row(r,'$R05Tasks','tasks');row(r,'$CopyGeneral','settings');header(r,'$PaletteAll');row(r,'$PaletteAwakeStop','power');row(r,'$R05Until','clock');}));
make('Search',1520,id=>{setQuery(id,'json');results(id,r=>{header(r,'$PaletteAll');row(r,'$R07NewWindow','json','',true);row(r,'JSON','file');row(r,'JSON 2','file');});});
make('Parameters',2280,id=>{setQuery(id,'$R09Query');results(id,r=>{header(r,'$PaletteAll');row(r,'$R05Start','sun','$R09Ninety',true);});});
make('Actions',3040,id=>{setQuery(id,'$R09Query');results(id,r=>{row(r,'$R05Start','sun');divider(r);row(r,'$R05Screen','sun','',true,true);row(r,'$R05Tasks','tasks');divider(r);row(r,'$R05Half','clock');row(r,'$R05Hour','clock');row(r,'$R05Until','clock');divider(r);row(r,'$PaletteAddFavorite','check');});});
make('Empty',3800,id=>{setQuery(id,'zzzz');results(id,r=>{Update(r,{justifyContent:'center',alignItems:'center',gap:'$Space8'});add(r,glyph('search'));add(r,txt('$PaletteEmpty',{fontSize:'$FontSizeSmall',fill:'$ColorTextDisabled'}));});});
make('Invalid',4560,id=>{setQuery(id,'$R09InvalidQuery',true);results(id,r=>{header(r,'$PaletteAll');row(r,'$R05Start','sun','45x',true);});});
const settings=add(document,frame('R09 Shortcuts',{x:5320,y:y+112,reusable:true,placeholder:true,width:720,height:340,layout:'horizontal',fill:'$ColorWindow',cornerRadius:'$RadiusWindow',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline',clip:true}));
const sidebar=add(settings,frame('Sidebar',{width:144,height:'fill_container',fill:'$ColorSidebar',padding:['$Space40','$Space12','$Space12','$Space12'],gap:'$Space16'}));
add(sidebar,{type:'ref',ref:'ivtHk',name:'Cubbix brand'});
for(const [title,icon,active] of [['$CopyGeneral','settings',false],['$R05Tiles','json',false],['$R05Shortcuts','settings',true],['$CopyAbout','settings',false]]){const nav=add(sidebar,frame('Navigation',{layout:'horizontal',width:'fill_container',height:32,alignItems:'center',padding:'$Space8',gap:'$Space8',cornerRadius:'$RadiusCard',fill:active?'$ColorAccentSoft':'$ColorTransparent'}));add(nav,glyph(icon));add(nav,txt(title,{fontSize:'$FontSizeSmall'}));}
const content=add(settings,frame('Settings content',{width:'fill_container',height:'fill_container',padding:'$Space24'}));
const card=add(content,frame('Settings card',{width:'fill_container',fill:'$ColorCard',cornerRadius:'$RadiusCard',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline'}));
for(const [i,title,key] of [[0,'$R05ShortcutLabel','⌃⌥⌘K'],[1,'$JSONShortcutLabel','⌃⌥⌘J'],[2,'$PaletteShortcutLabel','⌥.'],[3,'$R09Feedback','$R09FeedbackValue'],[4,'$R09Position','$R09PositionValue']]){
 if(i)divider(card);
 const r=add(card,frame('Shortcut row '+i,{layout:'horizontal',width:'fill_container',height:48,padding:['$Space0','$Space16'],alignItems:'center',gap:'$Space8'}));
 add(r,txt(title,{width:'fill_container',textGrowth:'fixed-width'}));
 add(r,frame('Recorder',{width:164,height:28,layout:'horizontal',padding:['$Space0','$Space12'],gap:'$Space2',alignItems:'center',cornerRadius:'$RadiusControl',fill:'$ColorControl',children:[txt(key,{fontSize:'$FontSizeSmall',fill:i>2?'$ColorTextDisabled':'$ColorText'})]}));
 if(i===2)add(r,glyph('alert'));
}
Update(settings,{placeholder:false});
add(document,frame('R09 round label',{x:0,y,width:840,height:32,children:[txt('R09 · 命令面板 / Command palette',{fontSize:'$FontSizeTitle',fontWeight:'$FontWeightSemibold'})]}));
for(const [index,language,mode] of [[0,'zh-CN','light'],[1,'zh-CN','dark'],[2,'en','light'],[3,'en','dark']]){
 const board=add(document,frame('R09 '+language+' '+mode,{x:6200+index*2336,y:y+112,width:2216,height:1460,padding:'$Space24',gap:'$Space24',theme:{language,mode},fill:'$ColorWindow',placeholder:true,clip:true}));
 add(board,txt('$R09Title',{fontSize:'$FontSizeDisplay',fontWeight:'$FontWeightSemibold'}));
 for(const states of [['Commands','Search','Parameters'],['Actions','Empty','Invalid']]){
  const line=add(board,frame('Review row',{layout:'horizontal',width:'fill_container',gap:'$Space24'}));
  for(const state of states){const cell=add(line,frame('Review '+state,{width:640,gap:'$Space12'}));const title={Commands:'R09List',Search:'R09Search',Parameters:'R09Params',Actions:'R09Actions',Empty:'R09Empty',Invalid:'R09Invalid'}[state];add(cell,txt('$'+title,{fontWeight:'$FontWeightMedium'}));const n=Get(variants[state]);delete n.id;delete n.x;delete n.y;add(cell,n);}
 }
 const settingCell=add(board,frame('Review shortcuts',{width:720,gap:'$Space12'}));add(settingCell,txt('$R09Settings',{fontWeight:'$FontWeightMedium'}));add(settingCell,{type:'ref',ref:settings,name:'Shortcut settings',width:720,height:340});
 Update(board,{placeholder:false});
}
Print('R09_CREATED',true);
'''
        run(code, SOURCE, save=True)
    star = ET.parse(ROOT / 'design/assets/lucide-0.468.0/star.svg').getroot()[0].attrib['d']
    repair = r'''
const roots=Get((n,c)=>{c.skipChildren();return {id:n.id,name:n.name,bounds:c.bounds}});
let star=roots.find(n=>n.name==='R09 Icon/star')?.id;
if(!star){star=Insert(document,{type:'frame',name:'R09 Icon/star',reusable:true,width:16,height:16,layout:'none',x:Math.max(...roots.map(n=>n.bounds.x+n.bounds.width))+120,y:roots.find(n=>n.name==='R09 Palette shell').bounds.y});Insert(star,{type:'path',name:'Lucide star',width:16,height:16,viewBox:[0,0,24,24],geometry:STAR,fill:'$ColorTransparent',stroke:'$ColorTextSecondary',strokeWidth:'$StrokeIcon',strokeLinecap:'round',strokeLinejoin:'round'});}
for(const root of roots.filter(n=>n.name?.startsWith('R09'))){
 Get(root.id,(n,c)=>{
  if(n.name==='Icon / check'&&c.parentCtx?.node.name?.includes('PaletteAddFavorite'))Replace(n.id,{type:'ref',ref:star,name:'Icon / star',width:16,height:16});
  if(n.type==='text'&&n.content==='⌥.'&&c.parentCtx?.parentCtx?.node.name==='Shortcut row 2')Update(n.id,{content:'⌃⌥⌘K'});
  if(n.type==='text'&&n.content!=='⌘K'&&c.parentCtx?.node.name==='Search row'){
   let state='Commands';for(let ctx=c.parentCtx;ctx;ctx=ctx.parentCtx){const match=/^(?:R09 |Review )(Parameters|Actions|Invalid|Search|Empty)$/.exec(ctx.node.name||'');if(match){state=match[1];break;}}
   const key=state==='Invalid'?'R09InvalidQuery':state==='Parameters'||state==='Actions'?'R09Query':state==='Commands'?'PaletteSearch':'';
   Update(n.id,{name:key?'Copy / '+key:'Query / literal',content:key?'$'+key:state==='Empty'?'zzzz':'json'});
  }
  if(n.type==='text'&&c.parentCtx?.node.name==='Recorder'){
   const row=c.parentCtx?.parentCtx?.node.name;
   if(row==='Shortcut row 3'||row==='Shortcut row 4'){
    const key=row==='Shortcut row 3'?'R09FeedbackValue':'R09PositionValue';Update(n.id,{name:'Copy / '+key,content:'$'+key});
   }
  }

 },{resolveInstances:true});
 if(/^R09 (zh-CN|en) (light|dark)$/.test(root.name))Update(root.id,{width:2016});
}
'''.replace('STAR', json.dumps(star))
    run(repair, SOURCE, save=True)
    # The pinned renderer does not resolve string variables in text content.
    # Keep the resource key in the layer name and author localized instance overrides.
    localize = r'''
const vars=GetVariables().variables;
const roots=Get((n,c)=>{c.skipChildren();return {id:n.id,name:n.name,theme:n.theme}}).filter(n=>n.name?.startsWith('R09'));
for(const root of roots){
 const language=root.theme?.language||'zh-CN';
 Get(root.id,(n,c)=>{
  if(n.type!=='text')return;
  let key=n.name?.startsWith('Copy / ')?n.name.slice(7):(n.content?.startsWith('$')?n.content.slice(1):'');
  if(!key)key=Object.keys(vars).find(k=>vars[k].type==='string'&&Array.isArray(vars[k].value)&&vars[k].value.some(v=>v.value===n.content))||'';
  if(key==='R09Reserved')key=c.parentCtx?.parentCtx?.node.name==='Shortcut row 3'?'R09FeedbackValue':'R09PositionValue';
  if(!key)return;
  const spec=vars[key];if(!spec)throw Error('Missing copy '+key);
  const value=Array.isArray(spec.value)?spec.value.find(v=>v.theme?.language===language)?.value:spec.value;
  if(typeof value!=='string')throw Error('Missing translation '+key);
  Update(n.id,{content:value,name:'Copy / '+key});
 },{resolveInstances:true});
}
'''
    run(localize, SOURCE, save=True)
    reopened = json.loads(SOURCE.read_text())
    unchanged = all(next(n for n in reopened['children'] if n['id'] == key) == value for key, value in historical.items())
    if not unchanged:
        raise SystemExit('Historical canvas nodes changed')
    code = r'''
const roots=Get((n,c)=>{c.skipChildren();return {id:n.id,name:n.name,theme:n.theme}});
const boards=roots.filter(n=>/^R09 (zh-CN|en) (light|dark)$/.test(n.name));
const qa={nodes:0,problems:[]};
for(const board of boards){Get(board.id,(n,c)=>{qa.nodes++;if(n.type==='text'&&n.name?.startsWith('Copy / ')){const spec=GetVariables().variables[n.name.slice(7)];const expected=Array.isArray(spec.value)?spec.value.find(v=>v.theme?.language===board.theme.language)?.value:spec.value;if(n.content!==expected)qa.problems.push({name:n.name,problem:'Wrong language'});}if(n.type==='text'&&n.content?.startsWith('$'))qa.problems.push({name:n.name,problem:'Unresolved copy'});if(c.problems&&n.enabled!==false)qa.problems.push({name:n.name,problem:c.problems,bounds:c.bounds});},{resolveInstances:true});Export([board.id],'png',OUTPUT,{scale:2});Print('R09_EXPORT',board);}
Print('R09_QA',qa);
'''.replace('OUTPUT', json.dumps(str(ROUND)))
    with tempfile.TemporaryDirectory(prefix='cubbix-r09-') as temporary:
        output = run(code, Path(temporary) / 'reopened.pen')
    def records(prefix):
        return [json.loads(line[len(prefix):]) for line in output.splitlines() if line.startswith(prefix)]
    qa = records('R09_QA ')[0]
    exports = []
    for item in records('R09_EXPORT '):
        filename = f'{item["theme"]["language"]}-{item["theme"]["mode"]}.png'
        target = ROUND / filename
        (ROUND / f'{item["id"]}.png').replace(target)
        exports.append({'nodeId': item['id'], 'file': filename, 'scale': 2,
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
