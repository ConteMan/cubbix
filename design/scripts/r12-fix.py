#!/usr/bin/env python3
"""Apply the R12 use-feedback fix pass through the pinned headless CLI."""
import json
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / 'design/cubbix.pen'
CLI = ['npx', '-y', '@pen.dev/cli@0.3.13', 'interactive']

FIX = r'''
const vars=GetVariables().variables;
const sizeOf=key=>vars[key].value;
const roots=Get((n,c)=>{c.skipChildren();return {id:n.id,name:n.name||''}});
const brand=roots.find(x=>x.name==='R12 Brand lockup');
if(brand){
 Update(brand.id,{layout:'horizontal',alignItems:'center',gap:'$Space8',height:sizeOf('SizeBrandMark')});
 for(const kid of Get(brand.id,{depth:1}).children){
  if(kid.name==='Brand mark')Update(kid.id,{width:sizeOf('SizeBrandMark'),height:sizeOf('SizeBrandMark')});
  if(kid.name==='Brand name')Update(kid.id,{width:'fill_container',height:sizeOf('SizeBrandMark'),textGrowth:'fixed-width-height',textAlignVertical:'middle'});
 }
}
function pick(node,name){if(node.name===name)return node;for(const child of node.children||[]){const found=pick(child,name);if(found)return found;}}
const stats={footers:0,guides:0,zoom:0};
const boards=roots.filter(x=>/^R12 (zh-CN|en) (light|dark)$/.test(x.name)||x.name==='R12 preview states');
for(const board of boards){
 const nodes=Get(board.id,(n,c)=>({id:n.id,name:n.name||'',parent:c.parentCtx?c.parentCtx.node.id:null}));
 for(const footer of nodes.filter(x=>x.name==='this input')){
  const shell=footer.parent;
  const shellKids=Get(shell,{depth:1}).children;
  const boundary=shellKids.find(k=>k.name==='Footer boundary');
  const grouped=Get(footer.id,{depth:1}).children.some(k=>k.name==='this group');
  if(boundary&&!grouped)continue;
  const value=(pick(Get(footer.id,{depth:4}),'Expression suffix')||{}).content||'';
  if(boundary)Delete(boundary.id);
  Delete(footer.id);stats.footers++;
  Insert(shell,{type:'frame',name:'Footer boundary',width:'fill_container',height:sizeOf('StrokeHairline'),fill:'$ColorSeparator'});
  const bar=Insert(shell,{type:'frame',name:'this input',width:'fill_container',height:sizeOf('SizeJSONExpressionHeight'),layout:'horizontal',alignItems:'center',gap:'$Space12',padding:['$Space6','$Space12']});
  Insert(bar,{type:'text',name:'this prefix',content:'this',fontFamily:'$FontFamilyCode',fontSize:'$FontSizeBody',fontWeight:'$FontWeightSemibold',lineHeight:'$LineHeightBody',fill:'$ColorText'});
  const field=Insert(bar,{type:'frame',name:'Expression field',width:'fill_container',height:sizeOf('SizeNavHeight'),layout:'horizontal',alignItems:'center',gap:'$Space0',cornerRadius:'$RadiusControl',fill:'$ColorControl',padding:['$Space0','$Space12']});
  Insert(field,{type:'text',name:'Expression suffix',content:value,fontFamily:'$FontFamilyCode',fontSize:'$FontSizeBody',lineHeight:'$LineHeightBody',fill:'$ColorText'});
  Insert(field,{type:'frame',name:'Caret',width:sizeOf('StrokeHairline'),height:sizeOf('SizeIcon'),fill:'$ColorText'});
  const tools=Insert(bar,{type:'frame',name:'Tools',width:'fit_content',height:sizeOf('SizeNavHeight'),layout:'horizontal',alignItems:'center',justifyContent:'end',gap:'$Space4'});
  for(const tool of ['PByrK','Ng6hD','g3HzZW','VnixZ'])Insert(tools,{type:'ref',ref:tool,name:'Tool',width:sizeOf('SizeIconButton'),height:sizeOf('SizeIconButton')});
 }
 for(const line of nodes.filter(x=>/^Code line \d+$/.test(x.name))){
  const kids=Get(line.id,{depth:1}).children;
  if(kids.some(k=>k.name==='Indent guide 1'))continue;
  const tokens=kids.find(k=>k.name==='Syntax tokens');
  if(!tokens)continue;
  const parts=Get(tokens.id,{depth:1}).children;
  const content=(parts[0]||{}).content||'';
  const spaces=content.length-content.replace(/^ +/,'').length;
  const levels=Math.floor(spaces/2);
  if(!levels)continue;
  const inset=sizeOf(String(tokens.padding[3]).replace('$',''));
  const contentLeft=(kids[0].width||0)+(kids[1].width||0)+inset;
  const step=Math.round(sizeOf('FontSizeBody')*0.6*2*10)/10;
  const hair=sizeOf('StrokeHairline');
  for(let level=1;level<=levels;level++){stats.guides++;Insert(line.id,{type:'frame',name:'Indent guide '+level,x:Math.round((contentLeft+step*level-hair)*10)/10,y:0,width:hair,height:sizeOf('SizeJSONLineHeight'),fill:'$ColorSyntaxIndent',layoutPosition:'absolute'});}
 }
 for(const box of nodes.filter(x=>x.name==='Zoom box')){
  const kids=Get(box.id,{depth:2}).children;
  if(kids.some(k=>k.name==='Zoomed indent guide'))continue;
  const code=kids.find(k=>k.name==='Zoomed code');
  if(code)Update(code.id,{x:Math.round((284+sizeOf('FontSizeBody')*0.6*2*4)*10)/10});
  Update(box.id,{width:660});
  const centre=kids.find(k=>k.name==='Centre line');
  if(centre)Update(centre.id,{width:660});
  Insert(box.id,{type:'frame',name:'Zoomed indent guide',x:342.4,y:0,width:4,height:96,fill:'$ColorSyntaxIndent'});stats.zoom++;
 }
}
Print('R12_FIX_STATS',JSON.stringify(stats));
'''



def run(code, output, save=False):
    commands = 'execute(' + json.dumps({'description': 'Apply R12 use-feedback fixes', 'input': code}) + ')\n'
    commands += ('save()\n' if save else '') + 'exit()\n'
    process = subprocess.run(CLI + ['--in', str(SOURCE), '--out', str(output)], input=commands,
                             text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, cwd=ROOT)
    if process.returncode or '[ERROR]' in process.stdout or 'Error:' in process.stdout:
        raise SystemExit(process.stdout[-8000:])
    return process.stdout


def main():
    apply = '--apply' in sys.argv
    with tempfile.TemporaryDirectory(prefix='cubbix-r12-fix-') as temporary:
        output = Path(temporary) / 'patched.pen'
        result = run(FIX, output)
        for line in result.splitlines():
            if line.startswith('R12_FIX'):
                print(line)
        if apply:
            run(FIX, SOURCE, save=True)


if __name__ == '__main__':
    main()
