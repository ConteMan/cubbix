#!/usr/bin/env python3
"""Author and reopen R12 use-feedback boards through the pinned headless CLI."""
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / 'design/cubbix.pen'
ROUND = ROOT / 'design/rounds/R12-2026-10-31-使用反馈'
CLI = ['npx', '-y', '@pen.dev/cli@0.3.13', 'interactive']

BOARD_FILES = {
    'R12 zh-CN light': 'zh-CN-light.png',
    'R12 zh-CN dark': 'zh-CN-dark.png',
    'R12 en light': 'en-light.png',
    'R12 en dark': 'en-dark.png',
    'R12 title states': 'title-states.png',
    'R12 preview states': 'preview-states.png',
}


def run(code, output, save=False):
    commands = 'execute(' + json.dumps({'description': 'Author R12 use feedback designs', 'input': code}) + ')\n'
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
    historical = {n['id']: n for n in before['children'] if not n.get('name', '').startswith('R12')}
    locales = {lang: json.loads((ROOT / f'design/locales/{lang}.json').read_text())
               for lang in ('zh-CN', 'en')}
    names = {'zh-CN': {'default': locales['zh-CN']['JSONWindowName'], 'custom': '订单查询',
                       'file': 'orders.json'},
             'en': {'default': locales['en']['JSONWindowName'], 'custom': 'request payload',
                    'file': 'orders.json'}}
    if not any(n.get('name', '').startswith('R12') for n in before['children']):
        code = ('const locales=' + json.dumps(locales, ensure_ascii=False)
                + ';const names=' + json.dumps(names, ensure_ascii=False) + ';\n' + r'''
const vars=GetVariables().variables;
const S=k=>vars[k].value;
const G={icon:S('SizeIcon'),small:S('SizeIconSmall'),button:S('SizeIconButton'),nav:S('SizeNavHeight'),
panel:S('SizeMenubarPanelWidth'),sidebar:S('SizeSidebarWidth'),w:S('SizeJSONWindowWidth'),
h:S('SizeJSONWindowHeight'),toolbar:S('SizeJSONToolbarHeight'),line:S('SizeJSONLineHeight'),
expression:S('SizeJSONExpressionHeight'),splitter:S('SizeJSONSplitter'),inset:S('SizeJSONTitleInset'),
seg:S('SizeSegmentHeight'),rowHeight:S('SizeRowHeight'),langWidth:S('SizeLanguageSelectorWidth'),
hair:S('StrokeHairline'),radiusW:S('RadiusWindow'),radiusCard:S('RadiusCard'),
radiusControl:S('RadiusControl'),edge:S('Space12'),brandMark:S('SizeIconButton'),
iconLarge:S('SizeIconLarge')};
G.split=Math.round(G.w*0.6);
const roots=Get((n,c)=>{c.skipChildren();return {name:n.name,bounds:c.bounds}});
const y=Math.max(...roots.map(n=>n.bounds.y+n.bounds.height))+240;
function add(parent,node){const children=node.children||[];delete node.children;const id=Insert(parent,node);for(const child of children)add(id,child);return id;}
function frame(name,more={}){return {type:'frame',name,layout:'vertical',gap:'$Space0',...more};}
function row(parent,name,more={}){return add(parent,frame(name,{layout:'horizontal',width:'fill_container',alignItems:'center',gap:'$Space8',...more}));}
function txt(content,more={}){return {type:'text',name:more.name||content,content,fontFamily:'$FontFamily',fontSize:'$FontSizeBody',fontWeight:'$FontWeightRegular',lineHeight:'$LineHeightBody',fill:'$ColorText',...more};}
function note(parent,content){return add(parent,txt(content,{fontSize:'$FontSizeSmall',fill:'$ColorTextSecondary'}));}
const icons={braces:'Hvppf',plus:'f312q',pin:'vnA76',file:'iakjU',down:'Lq8Qv',right:'MTEP9',alert:'bVN9P'};
function glyph(name,more={}){return {type:'ref',ref:icons[name],name:'Icon / '+name,width:G.icon,height:G.icon,...more};}
// 01 native window controls: close, minimize and zoom all carry the system symbol.
const controls=add(document,frame('R12 Window controls',{reusable:true,x:0,y:y+112,width:G.small*3+S('Space8')*2,height:G.small,layout:'horizontal',gap:'$Space8',alignItems:'center',placeholder:true}));
function control(name,symbols){
 const dot=add(controls,frame(name,{width:G.small,height:G.small,layout:'none',cornerRadius:'$RadiusControl',fill:'$ColorTextDisabled'}));
 for(const [key,geometry,fill,stroke] of symbols)add(dot,{type:'path',name:key,width:G.small,height:G.small,viewBox:[0,0,12,12],geometry,fill,stroke,strokeWidth:'$StrokeHairline',strokeLinecap:'round',strokeLinejoin:'round'});
}
control('Close',[['Close symbol left','M3.75 3.75 L8.25 8.25','$ColorTransparent','$ColorWindow'],['Close symbol right','M8.25 3.75 L3.75 8.25','$ColorTransparent','$ColorWindow']]);
control('Minimize',[['Minimize symbol','M3.75 6 L8.25 6','$ColorTransparent','$ColorWindow']]);
control('Zoom',[['Zoom symbol out','M2.9 2.9 L2.9 5.9 L5.9 2.9 Z','$ColorWindow','$ColorTransparent'],['Zoom symbol in','M9.1 9.1 L9.1 6.1 L6.1 9.1 Z','$ColorWindow','$ColorTransparent']]);
Update(controls,{placeholder:false});
// 02 sidebar brand: the mark sits above the name as one vertical lockup.
const brand=add(document,frame('R12 Brand lockup',{reusable:true,x:160,y:y+112,width:G.sidebar-G.edge*2,height:S('SizeBrandMark')+8+Math.round(S('FontSizeTitle')*S('LineHeightTitle')),layout:'vertical',gap:'$Space8',alignItems:'start',placeholder:true}));
add(brand,{type:'ref',ref:'r2xEJ',name:'Brand mark',width:S('SizeBrandMark'),height:S('SizeBrandMark')});
add(brand,txt('Cubbix',{name:'Brand name',fontSize:'$FontSizeBody',fontWeight:'$FontWeightSemibold',lineHeight:'$LineHeightBody'}));
Update(brand,{placeholder:false});
// Title inset: edge 12 + controls 52 + Space16, so the name never touches the controls.
function windowBar(parent,width,state,lang){
 const bar=row(parent,'Window title bar / '+state,{width,height:G.toolbar,padding:['$Space0','$Space12'],gap:'$Space8',fill:'$ColorCard',cornerRadius:['$RadiusWindow','$RadiusWindow','$Space0','$Space0'],stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline'});
 const left=row(bar,'Title group',{width:'fit_content',gap:'$Space16'});
 add(left,{type:'ref',ref:controls,name:'Window controls'});
 const name=names[lang][state==='file'?'file':state==='custom'?'custom':'default'];
 if(state==='editing'){
  const field=row(left,'Title field',{width:'fit_content',height:G.line,padding:['$Space0','$Space8'],gap:'$Space0',cornerRadius:'$RadiusControl',fill:'$ColorControl',stroke:'$ColorText',strokeWidth:'$StrokeHairline'});
  add(field,txt(name,{name:'Window name',fontWeight:'$FontWeightMedium'}));
  add(field,frame('Caret',{width:G.hair,height:G.icon,fill:'$ColorText'}));
 }else{
  add(left,txt(name,{name:'Window name',fontWeight:'$FontWeightMedium'}));
 }
 add(bar,frame('Title spacer',{width:'fill_container',height:'fill_container',layout:'none'}));
 const actions=row(bar,'Top actions',{width:'fit_content',gap:'$Space8'});
 for(const action of ['NMhjk','ICb8f','gPAiw'])add(actions,{type:'ref',ref:action,name:'Top action',width:G.button,height:G.button});
 return bar;
}
function sidebarWindow(parent,lang){
 const text=k=>locales[lang][k];
 const shell=add(parent,frame('Main window',{width:720,height:340,layout:'horizontal',fill:'$ColorWindow',cornerRadius:'$RadiusWindow',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline',clip:true}));
 const side=add(shell,frame('Sidebar',{width:G.sidebar,height:'fill_container',padding:'$Space12',gap:'$Space16',fill:'$ColorSidebar'}));
 add(side,{type:'ref',ref:controls,name:'Window controls'});
 add(side,{type:'ref',ref:brand,name:'Brand lockup'});
 const nav=add(side,frame('Navigation',{width:'fill_container',gap:'$Space4'}));
 for(const [key,ref,selected] of [['General','O4KJZC',true],['R05Tiles','Ectkj',false],['R05Shortcuts','H8TPWq',false],['About','uTgqT',false]]){
  const item=row(nav,'Navigation / '+key,{height:G.nav,padding:['$Space0','$Space8'],cornerRadius:'$RadiusCard',fill:selected?'$ColorAccentSoft':'$ColorTransparent'});
  add(item,{type:'ref',ref,name:'Navigation icon',width:G.icon,height:G.icon});
  add(item,txt(text(key),{name:'Copy / '+key}));
 }
 const content=add(shell,frame('Settings content',{width:'fill_container',height:'fill_container',padding:'$Space24',gap:'$Space16'}));
 const card=add(content,frame('Settings card',{width:'fill_container',padding:['$Space0','$Space16'],cornerRadius:'$RadiusCard',fill:'$ColorCard',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline'}));
 const line=row(card,'Language row',{height:G.rowHeight});
 add(line,txt(text('Language'),{width:'fill_container',textGrowth:'fixed-width'}));
 add(line,lang==='en'
  ?{type:'ref',ref:'Hs9NC',name:'Language selector',width:G.langWidth,height:G.seg,descendants:{RRg9B:{content:'System default'},X2e2V:{fill:'$ColorTransparent'},pYbgH:{fill:'$ColorText'},ZR7tO:{fill:'$ColorAccent'},ExLjG:{fill:'$ColorOnAccent'}}}
  :{type:'ref',ref:'Hs9NC',name:'Language selector',width:G.langWidth,height:G.seg});
 add(card,frame('Divider',{width:'fill_container',height:G.hair,fill:'$ColorSeparator'}));
 const login=row(card,'Open at login row',{height:G.rowHeight});
 add(login,txt(text('LaunchAtLogin'),{width:'fill_container',textGrowth:'fixed-width'}));
 add(login,{type:'ref',ref:'f23cLA',name:'Open at login switch',width:S('SizeSwitchWidth'),height:S('SizeSwitchHeight')});
}
function jsonCard(parent,lang,folded,rows){
 const text=k=>locales[lang][k];
 const panel=add(parent,frame('Menubar panel'+(folded?' / folded':' / list'),{width:G.panel,padding:'$Space12',gap:'$Space8',cornerRadius:'$RadiusWindow',fill:'$ColorMenuSurface',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline'}));
 const card=add(panel,frame('JSON card',{width:'fill_container',padding:'$Space10',gap:'$Space8',cornerRadius:'$RadiusCard',fill:'$ColorCard',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline'}));
 const title=row(card,'JSON title',{height:G.nav});
 add(title,{type:'ref',ref:'Hvppf',name:'Icon / braces',width:G.icon,height:G.icon,descendants:{VfDAN:{stroke:'$ColorText'}}});
 add(title,txt(text('JSON'),{name:'Copy / JSON',width:'fill_container',textGrowth:'fixed-width',fontWeight:'$FontWeightSemibold'}));
 const button=add(title,frame('Quick new JSON',{width:G.button,height:G.button,layout:'horizontal',alignItems:'center',justifyContent:'center',cornerRadius:'$RadiusControl'}));
 add(button,{type:'ref',ref:'f312q',name:'Icon / plus',width:G.icon,height:G.icon});
 const list=add(card,frame('JSON window list',{width:'fill_container',gap:'$Space2'}));
 if(folded){
  const count=row(list,'Window count',{height:G.nav,padding:['$Space0','$Space8']});
  add(count,glyph('right'));
  add(count,txt(text('PanelJSONWindowsCount').replace('%d','7'),{width:'fill_container',textGrowth:'fixed-width'}));
 }else{
  for(const [index,key] of rows.entries()){
   const line=row(list,'Window / '+names[lang][key],{height:G.nav,padding:['$Space0','$Space8'],cornerRadius:'$RadiusControl'});
   add(line,glyph('file'));
   add(line,txt(names[lang][key],{width:'fill_container',textGrowth:'fixed-width'}));
   if(index===0)add(line,glyph('pin'));
  }
 }
}
function tooltipCell(parent,content,label){
 const cell=add(parent,frame('Tooltip cell',{width:240,gap:'$Space8'}));
 add(cell,txt(label,{fontSize:'$FontSizeSmall',fill:'$ColorTextSecondary',fontWeight:'$FontWeightMedium'}));
 const bubble=row(cell,'Tooltip',{width:'fit_content',padding:['$Space6','$Space8'],gap:'$Space0',cornerRadius:'$RadiusControl',fill:'$ColorTooltip'});
 add(bubble,txt(content,{fontSize:'$FontSizeSmall',fill:'$ColorTooltipText'}));
}
const lines=[
 {n:'1',fold:true,t:[['{','$ColorTextSecondary']]},
 {n:'2',fold:false,t:[['  ','$ColorText'],['"name"','$ColorSyntaxKey'],[': ','$ColorTextSecondary'],['"cubbix"','$ColorSyntaxString'],[',','$ColorTextSecondary']]},
 {n:'3',fold:true,t:[['  ','$ColorText'],['"tabs"','$ColorSyntaxKey'],[': [','$ColorTextSecondary']]},
 {n:'4',fold:false,t:[['    1','$ColorSyntaxNumber'],[',','$ColorTextSecondary']]},
 {n:'5',fold:false,t:[['    2','$ColorSyntaxNumber']]},
 {n:'6',fold:false,t:[['  ],','$ColorTextSecondary']]},
 {n:'7',fold:false,t:[['  ','$ColorText'],['"ok"','$ColorSyntaxKey'],[': ','$ColorTextSecondary'],['true','$ColorSyntaxBool'],[',','$ColorTextSecondary']]},
 {n:'8',fold:false,t:[['  ','$ColorText'],['"none"','$ColorSyntaxKey'],[': ','$ColorTextSecondary'],['null','$ColorSyntaxNull']]},
 {n:'9',fold:false,t:[['}','$ColorTextSecondary']]},
];
// Every code row is one line height with centred gutters: number cell, fold cell, tokens.
function codeArea(parent,lines,width){
 const editor=add(parent,frame('Editor',{width:width||'fill_container',height:'fill_container',padding:['$Space8','$Space0']}));
 for(const line of lines){
  const codeRow=row(editor,'Code line '+line.n,{height:G.line,gap:'$Space0'});
  const numberCell=add(codeRow,frame('Line number cell',{width:44,height:'fill_container',layout:'horizontal',padding:['$Space0','$Space0','$Space0','$Space12'],alignItems:'center'}));
  add(numberCell,txt(line.n,{name:'Line number '+line.n,fontFamily:'$FontFamilyCode',fill:'$ColorTextSecondary'}));
  const foldCell=add(codeRow,frame('Fold gutter',{width:24,height:'fill_container',layout:'horizontal',padding:['$Space0','$Space4'],alignItems:'center',justifyContent:'center'}));
  if(line.fold)add(foldCell,glyph('down'));
  const tokens=add(codeRow,frame('Syntax tokens',{width:'fill_container',height:'fill_container',layout:'horizontal',padding:['$Space0','$Space0','$Space0','$Space12'],alignItems:'center'}));
  for(const [content,color] of line.t)add(tokens,txt(content,{name:'Token',fontFamily:'$FontFamilyCode',fill:color}));
 }
 return editor;
}
// The this row shares the editor width: full width without a preview, the editor
// pane once the separator and the preview exist.
function expressionBar(parent,value,fullWidth){
 const bar=row(parent,'this input',{height:G.expression,gap:'$Space0'});
 const group=row(bar,'this group',{width:fullWidth?'fill_container':G.split,height:'fill_container',padding:['$Space6','$Space12'],gap:'$Space12'});
 add(group,txt('this',{name:'this prefix',fontFamily:'$FontFamilyCode',fontWeight:'$FontWeightSemibold'}));
 const field=row(group,'Expression field',{width:'fill_container',height:G.nav,padding:['$Space0','$Space12'],gap:'$Space0',cornerRadius:'$RadiusControl',fill:'$ColorControl'});
 add(field,txt(value,{name:'Expression suffix',fontFamily:'$FontFamilyCode'}));
 add(field,frame('Caret',{width:G.hair,height:G.icon,fill:'$ColorText'}));
 const tools=add(bar,frame('Tools',{width:fullWidth?'fit_content':'fill_container',height:'fill_container',layout:'horizontal',gap:'$Space4',alignItems:'center',justifyContent:fullWidth?'start':'end',padding:['$Space0','$Space12']}));
 for(const tool of ['PByrK','Ng6hD','g3HzZW','VnixZ'])add(tools,{type:'ref',ref:tool,name:'Tool',width:G.button,height:G.button});
}
function jsonWindow(parent,lang,state,title){
 const text=k=>locales[lang][k];
 const shell=add(parent,frame('JSON window / '+state,{width:G.w,height:G.h,fill:'$ColorCard',cornerRadius:'$RadiusWindow',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline',clip:true}));
 windowBar(shell,G.w,title||'default',lang);
 add(shell,frame('Toolbar separator',{width:'fill_container',height:G.hair,fill:'$ColorSeparator'}));
 const work=add(shell,frame('Workspace',{width:'fill_container',height:'fill_container',layout:'horizontal',gap:'$Space0'}));
 if(state==='idle'){
  codeArea(work,lines);
 }else{
  codeArea(work,lines,G.split);
  add(work,frame('Draggable separator',{width:G.splitter,height:'fill_container',fill:'$ColorCard',stroke:'$ColorSeparator',strokeWidth:{left:'$StrokeHairline'}}));
  const result=add(work,frame('Right result',{width:'fill_container',height:'fill_container',padding:['$Space8','$Space12'],gap:'$Space0',justifyContent:state==='error'?'center':'start',alignItems:state==='error'?'center':'start'}));
  if(state==='error'){
   add(result,{type:'ref',ref:icons.alert,name:'Icon / triangle-alert',width:G.iconLarge,height:G.iconLarge,descendants:{U9Y0kM:{stroke:'$ColorDanger'},u8m4R:{stroke:'$ColorDanger'},YGLGO:{stroke:'$ColorDanger'}}});
  }else{
   for(const [content,color] of [['[','$ColorTextSecondary'],['  1,','$ColorSyntaxNumber'],['  2','$ColorSyntaxNumber'],[']','$ColorTextSecondary']]){
    const line=row(result,'Result line',{height:G.line,gap:'$Space0'});
    add(line,txt(content,{name:'Result token',fontFamily:'$FontFamilyCode',fill:color}));
   }
  }
 }
 expressionBar(shell,state==='error'?'.missing':'.tabs',state==='idle');
 return shell;
}
function gutterZoom(parent,lang){
 const zoom=4;
 const cell=add(parent,frame('Gutter zoom ×'+zoom,{width:'fill_container',gap:'$Space8'}));
 const boxWidth=44*zoom+G.icon*zoom+380;
 const box=add(cell,frame('Zoom box',{width:boxWidth,height:G.line*zoom,layout:'none',cornerRadius:'$RadiusCard',fill:'$ColorCard',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline',clip:true}));
 const textSize=Math.round(S('FontSizeBody')*zoom*2)/2;
 const top=Math.round((G.line*zoom-textSize*S('LineHeightBody'))/2);
 add(box,txt('12',{name:'Zoomed line number',x:Math.round(G.edge*2),y:top,fontFamily:'$FontFamilyCode',fontSize:textSize,lineHeight:'$LineHeightBody',fill:'$ColorTextSecondary'}));
 add(box,{type:'path',name:'Zoomed fold arrow',x:44*zoom+G.edge,y:Math.round((G.line*zoom-G.icon*zoom)/2),width:G.icon*zoom,height:G.icon*zoom,viewBox:[0,0,24,24],geometry:'M6 9 L12 15 L18 9',fill:'$ColorTransparent',stroke:'$ColorTextSecondary',strokeWidth:S('StrokeIcon')*zoom,strokeLinecap:'round',strokeLinejoin:'round'});
 add(box,txt('"tabs": [',{name:'Zoomed code',x:68*zoom+G.edge,y:top,fontFamily:'$FontFamilyCode',fontSize:textSize,lineHeight:'$LineHeightBody',fill:'$ColorText'}));
 add(box,frame('Centre line',{x:0,y:Math.round(G.line*zoom/2),width:boxWidth,height:G.hair,fill:'$ColorSeparator'}));
 note(cell,lang==='en'?'Line height 24 ×4. Number 13/1.5 and arrow 16 share one centre line.':'行高 24 放大 4 倍。行号 13/1.5 与 16 的箭头落在同一条中心线。');
}
add(document,frame('R12 round label',{x:0,y,width:840,height:G.nav,children:[txt('R12 · 使用反馈 / Use feedback',{fontSize:'$FontSizeTitle',fontWeight:'$FontWeightSemibold',lineHeight:'$LineHeightTitle'})]}));
const labels={zh:['主窗口侧栏 · 三个红绿灯，品牌整组在红绿灯下方','菜单栏 JSON 卡片 · 只保留标题行加号','JSON 窗口 · 尚无结果，编辑区占满宽度'],
 en:['Main window sidebar · three controls, brand lockup below them','Menubar JSON card · plus in the title row only','JSON window · no result yet, the editor takes the full width']};
for(const [index,lang,mode] of [[0,'zh-CN','light'],[1,'zh-CN','dark'],[2,'en','light'],[3,'en','dark']]){
 const label=lang==='en'?labels.en:labels.zh;
 const text=k=>locales[lang][k];
 const board=add(document,frame('R12 '+lang+' '+mode,{x:440+index*1096,y:y+112,width:976,padding:'$Space24',gap:'$Space24',theme:{language:lang,mode},fill:'$ColorWindow',clip:true,placeholder:true}));
 add(board,txt(lang==='en'?'Use feedback':'使用反馈',{fontSize:'$FontSizeDisplay',fontWeight:'$FontWeightSemibold',lineHeight:'$LineHeightTitle'}));
 add(board,txt(label[0],{fontWeight:'$FontWeightMedium'}));
 sidebarWindow(board,lang);
 add(board,txt(label[1],{fontWeight:'$FontWeightMedium'}));
 const cards=row(board,'JSON cards',{width:'fill_container',gap:'$Space24',alignItems:'start'});
 jsonCard(cards,lang,false,['default','file','custom']);
 jsonCard(cards,lang,true,[]);
 tooltipCell(cards,text('JSONShortcutLabel')+' (⌃⌥⌘J)',lang==='en'?'Plus tooltip · shortcut kept':'加号 tooltip · 保留快捷键');
 add(board,txt(label[2],{fontWeight:'$FontWeightMedium'}));
 jsonWindow(board,lang,'idle');
 Update(board,{placeholder:false});
}
// 05 four title states in both languages, one name across every surface.
{
 const board=add(document,frame('R12 title states',{x:440+4*1096,y:y+112,width:1752,padding:'$Space24',gap:'$Space24',theme:{language:'zh-CN',mode:'light'},fill:'$ColorWindow',clip:true,placeholder:true}));
 add(board,txt('JSON 窗口标题 · 四种状态与同一个名字 / Four title states, one name',{fontSize:'$FontSizeDisplay',fontWeight:'$FontWeightSemibold',lineHeight:'$LineHeightTitle'}));
 const columns=row(board,'Language columns',{width:'fill_container',gap:'$Space24',alignItems:'start'});
 for(const lang of ['zh-CN','en']){
  const column=add(columns,frame('Titles / '+lang,{width:G.w,gap:'$Space16'}));
  const states=[['default',lang==='en'?'Default · JSON for a window with no file':'默认 · 未打开文件时显示 JSON'],
   ['editing',lang==='en'?'Editing · Enter confirms, Esc cancels, empty restores the default':'正在编辑 · Enter 确认，Esc 取消，清空恢复默认'],
   ['custom',lang==='en'?'Custom name · kept after renaming':'自定义名 · 改名后保持'],
   ['file',lang==='en'?'File name · opening another file switches to its name':'文件名 · 换一个文件就换成新文件名']];
  for(const [state,label] of states){
   const cell=add(column,frame('State / '+state,{width:'fill_container',gap:'$Space8'}));
   add(cell,txt(label,{fontSize:'$FontSizeSmall',fill:'$ColorTextSecondary'}));
   windowBar(cell,G.w,state,lang);
  }
  const cardCell=add(column,frame('Card list / '+lang,{width:G.panel,gap:'$Space8'}));
  add(cardCell,txt(lang==='en'?'The window menu and the card list reuse this name':'窗口菜单与卡片列表复用这个名字',{fontSize:'$FontSizeSmall',fill:'$ColorTextSecondary'}));
  jsonCard(cardCell,lang,false,['default','custom','file']);
 }
 Update(board,{placeholder:false});
}
// 06 preview appears only after a result; the gutter row alignment.
{
 const board=add(document,frame('R12 preview states',{x:440+4*1096+1752+120,y:y+112,width:888,padding:'$Space24',gap:'$Space24',theme:{language:'zh-CN',mode:'light'},fill:'$ColorWindow',clip:true,placeholder:true}));
 add(board,txt('预览时机与行号对齐 / Preview and gutter',{fontSize:'$FontSizeDisplay',fontWeight:'$FontWeightSemibold',lineHeight:'$LineHeightTitle'}));
 const states=[['idle','尚无结果 · 不画预览与分隔条，编辑区占满宽度','No result yet · no preview, no separator, editor full width'],
  ['result','有结果 · 出现分隔条与预览','Result · separator and preview appear'],
  ['error','出错 · 预览只放 alert 图标，不写说明文字','Error · alert icon only, no explanatory text']];
 for(const [state,zh,en] of states){
  const cell=add(board,frame('Preview / '+state,{width:'fill_container',gap:'$Space8'}));
  add(cell,txt(zh,{fontSize:'$FontSizeSmall',fill:'$ColorTextSecondary'}));
  add(cell,txt(en,{fontSize:'$FontSizeSmall',fill:'$ColorTextDisabled'}));
  jsonWindow(cell,'zh-CN',state);
 }
 gutterZoom(board,'zh-CN');
 Update(board,{placeholder:false});
}
Print('R12_CREATED',true);
''')
        run(code, SOURCE, save=True)
    after = json.loads(SOURCE.read_text())
    unchanged = all(next(n for n in after['children'] if n['id'] == key) == value
                    for key, value in historical.items())
    if not unchanged:
        raise SystemExit('Historical canvas changed')
    code = r'''
const files=FILES;
const boards=Get((n,c)=>{c.skipChildren();if(files[n.name])return {id:n.id,name:n.name,file:files[n.name],theme:n.theme,bounds:c.bounds};});
const qa={nodes:0,problems:[]};
for(const board of boards){Get(board.id,(n,c)=>{qa.nodes++;if(c.problems&&n.enabled!==false)qa.problems.push({name:n.name,problem:c.problems,bounds:c.bounds});},{resolveInstances:true});Export([board.id],'png',OUTPUT,{scale:2});Print('R12_EXPORT',board);}
Print('R12_QA',{nodes:qa.nodes,problemCount:qa.problems.length,problems:qa.problems.slice(0,20)});
'''.replace('FILES', json.dumps(BOARD_FILES)).replace('OUTPUT', json.dumps(str(ROUND)))
    with tempfile.TemporaryDirectory(prefix='cubbix-r12-') as temporary:
        output = run(code, Path(temporary) / 'reopened.pen')
    records = lambda prefix: [json.loads(line[len(prefix):]) for line in output.splitlines()
                              if line.startswith(prefix)]
    qa = records('R12_QA ')[0]
    exports = []
    for board in records('R12_EXPORT '):
        target = ROUND / board['file']
        (ROUND / f'{board["id"]}.png').replace(target)
        exports.append({'nodeId': board['id'], 'name': board['name'], 'file': board['file'],
                        'scale': 2, 'sha256': hashlib.sha256(target.read_bytes()).hexdigest()})
    result = {'cliVersion': '0.3.13', 'historicalRootsUnchanged': unchanged,
              'nativeLayout': qa, 'exports': exports,
              'passed': len(exports) == len(BOARD_FILES) and not qa['problems'] and unchanged}
    (ROUND / 'render-validation.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result, ensure_ascii=False, indent=2))
    if not result['passed']:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
