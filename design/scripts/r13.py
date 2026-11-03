#!/usr/bin/env python3
"""Append and reopen spec 004's lid states through the headless CLI."""
import json
from pathlib import Path
import subprocess
import tempfile
import shutil
ROOT=Path(__file__).resolve().parents[2]
SOURCE=ROOT/'design/cubbix.pen'
ROUND=ROOT/'design/rounds/R13-2026-11-03-合盖保持'
CLI=['npx','-y','@pen.dev/cli@0.3.13','interactive']
def run(code,output,save=False):
    commands='execute('+json.dumps({'description':'Author R13 lid states','input':code})+')\n'+('save()\n' if save else '')+'exit()\n'
    p=subprocess.run(CLI+['--in',str(SOURCE),'--out',str(output)],input=commands,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,cwd=ROOT)
    if p.returncode or '[ERROR]' in p.stdout or 'Error:' in p.stdout:raise SystemExit(p.stdout[-5000:])
    return p.stdout

def main():
    ROUND.mkdir(parents=True,exist_ok=True)
    before=json.loads(SOURCE.read_text())
    locales={lang:json.loads((ROOT/f'design/locales/{lang}.json').read_text()) for lang in ('zh-CN','en')}
    if not any(n.get('name','').startswith('R13 ') for n in before['children']):
        run('const locales='+json.dumps(locales,ensure_ascii=False)+';\n'+r'''
const v=GetVariables().variables,S=k=>v[k].value;
const roots=Get((n,c)=>{c.skipChildren();return {bounds:c.bounds}});
const y=Math.max(...roots.map(n=>n.bounds.y+n.bounds.height))+240;
function add(p,n){return Insert(p,n)}
function frame(p,name,more={}){return add(p,{type:'frame',name,layout:'vertical',gap:'$Space8',...more})}
function text(p,content,more={}){return add(p,{type:'text',content,name:content,fontFamily:'$FontFamily',fontSize:'$FontSizeBody',lineHeight:'$LineHeightBody',fill:'$ColorText',...more})}
function row(p,name,more={}){return frame(p,name,{layout:'horizontal',width:'fill_container',alignItems:'center',...more})}
function toggle(p,on){const f=row(p,'Lid switch',{width:S('SizeSwitchWidth'),height:S('SizeSwitchHeight'),padding:'$Space2',cornerRadius:'$RadiusControl',fill:on?'$ColorAccent':'$ColorControl',justifyContent:on?'end':'start'});frame(f,'Thumb',{width:S('SizeSwitchThumb'),height:S('SizeSwitchThumb'),cornerRadius:'$RadiusControl',fill:on?'$ColorThumb':'$ColorTextSecondary'})}
for(const [i,lang,mode] of [[0,'zh-CN','light'],[1,'zh-CN','dark'],[2,'en','light'],[3,'en','dark']]){
 const t=k=>locales[lang][k];
 const b=frame(document,'R13 '+lang+' '+mode,{x:i*720,y:y+112,width:600,padding:'$Space24',gap:'$Space24',theme:{language:lang,mode},fill:'$ColorWindow',clip:true,placeholder:true});
 text(b,t('R05Lid'),{fontSize:'$FontSizeTitle',fontWeight:'$FontWeightSemibold'});
 for(const state of ['off','on','hint','alert']){
  const card=frame(b,'Awake card / '+state,{width:S('SizeMenubarPanelWidth')-24,padding:'$Space10',fill:'$ColorCard',cornerRadius:'$RadiusCard',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline'});
  const h=row(card,'Header',{height:S('SizeNavHeight')});text(h,t('Awake'),{width:'fill_container',textGrowth:'fixed-width',fontWeight:'$FontWeightSemibold'});toggle(h,state!=='off');
  const m=row(card,'Modes',{height:S('SizeSegmentHeight'),padding:'$Space2',fill:'$ColorControl',cornerRadius:'$RadiusControl'});text(m,t('R05Screen'),{fontSize:'$FontSizeSmall'});text(m,t('R05Tasks'),{fontSize:'$FontSizeSmall'});
  const pills=row(card,'Duration',{height:S('SizeButtonHeight')});for(const k of ['PanelDuration30m','PanelDuration1h','PanelDuration2h','PanelDurationInf'])text(pills,t(k));
  const lid=row(card,'Lid row',{height:S('SizeNavHeight'),justifyContent:'space_between'});
  add(lid,{type:'icon',library:'lucide',icon:'laptop',width:S('SizeIcon'),height:S('SizeIcon'),fill:'$ColorTextSecondary'});toggle(lid,state==='on');
  if(state!=='off'){
   const status=row(card,'Status',{height:S('Space20')});
   if(state==='alert')add(status,{type:'icon',library:'lucide',icon:'triangle-alert',width:S('SizeIcon'),height:S('SizeIcon'),fill:'$ColorDanger'});
   else {text(status,t('R05Tasks')+(state==='on'?' · '+t('R05Lid'):'')+' · 1:24',{fontSize:'$FontSizeSmall',textGrowth:'fixed-width',width:'fill_container',fill:'$ColorTextSecondary'});if(state==='hint')add(status,{type:'icon',library:'lucide',icon:'laptop',width:S('SizeIcon'),height:S('SizeIcon'),fill:'$ColorTextSecondary'})}
  }
 }
 const settings=frame(b,'Battery setting',{width:'fill_container',padding:'$Space16',fill:'$ColorCard',cornerRadius:'$RadiusCard',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline'});
 const r=row(settings,'Low battery limit',{height:S('SizeRowHeight')});text(r,t('LidBatteryLimit'),{width:'fill_container',textGrowth:'fixed-width'});for(const n of ['10%','20%','30%'])text(r,n,{fill:n==='20%'?'$ColorText':'$ColorTextSecondary'});
 const tip=frame(b,'Recovery tooltip',{width:'fill_container',padding:'$Space8',fill:'$ColorTooltip',cornerRadius:'$RadiusControl'});text(tip,t('LidRestoreFailed'),{fill:'$ColorTooltipText',fontSize:'$FontSizeSmall'});text(tip,t('LidForceRestore'),{fill:'$ColorTooltipText'});text(tip,t('LidForceRestoreHint'),{fill:'$ColorTooltipText',fontSize:'$FontSizeSmall',width:'fill_container',textGrowth:'fixed-width'});
 Update(b,{placeholder:false});
}
''',SOURCE,True)
    run(r"""
const boards=Get((n,c)=>{c.skipChildren();if(n.name?.startsWith('R13 '))return {id:n.id};});
for(const b of boards){Update(b.id,{placeholder:true});Get(b.id,(n,c)=>{
 if(n.name==='Recovery tooltip'||n.name==='Recovery popover'){
  Update(n.id,{name:'Recovery popover',fill:'$ColorCard',stroke:'$ColorSeparator',strokeWidth:'$StrokeHairline'});
 }
 if(n.type==='text'&&n.fill==='$ColorTooltipText')Update(n.id,{fill:'$ColorText'});
 if(n.type==='text'&&(n.content==='强制恢复睡眠'||n.content==='Restore sleep now'))Update(n.id,{fill:'$ColorDanger'});
});Update(b.id,{placeholder:false});}
""",SOURCE,True)
    after=json.loads(SOURCE.read_text())
    for node in before['children']:
        if not node.get('name','').startswith('R13 '):
            assert node==next(n for n in after['children'] if n['id']==node['id']),node['id']
    with tempfile.TemporaryDirectory(prefix='cubbix-r13-') as tmp:
        for node in after['children']:
            if node.get('name','').startswith('R13 '):
                filename=node['name'].replace('R13 ','').replace(' ','-')+'.png'
                export_dir=Path(tmp)/'exports'
                run('Export(['+json.dumps(node['id'])+'],"png",'+json.dumps(str(export_dir))+',{scale:2});',Path(tmp)/'reopen.pen')
                target=ROUND/filename
                if target.is_dir():
                    shutil.rmtree(target)
                shutil.copyfile(export_dir/(node['id']+'.png'),target)
    print('R13 authored and reopened; historical roots unchanged')
if __name__=='__main__':main()
