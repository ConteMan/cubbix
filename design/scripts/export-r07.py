#!/usr/bin/env python3
"""Reopen R07 using the headless CLI, validate its layouts, and export previews."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
ROUND = ROOT / 'design/rounds/R07-2026-10-30-JSON'


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--cli-file', type=Path, required=True)
    args = parser.parse_args()
    source = ROOT / 'design/cubbix.pen'
    code = r'''
const roots=Get((n,c)=>{c.skipChildren();return {id:n.id,name:n.name,metadata:n.metadata,theme:n.theme,reusable:n.reusable,bounds:c.bounds}});
const boards=roots.filter(n=>n.metadata?.type==='r07-review');
let qa={nodes:0,problems:[],emptyWindowTextCount:0,localizationProblems:[],windowSizes:[]};
for(const b of boards){
 Get(b.id,(n,c)=>{
  qa.nodes++;
  if(c.problems&&n.enabled!==false)qa.problems.push({id:n.id,name:n.name,problem:c.problems,bounds:c.bounds});
  if(n.name==='JSON number result')qa.windowSizes.push({language:b.theme.language,mode:b.theme.mode,width:c.bounds.width,height:c.bounds.height});
  if(n.type==='text'&&n.enabled!==false&&b.theme.language==='en'&&/[\u4e00-\u9fff]/.test(n.content||''))qa.localizationProblems.push({id:n.id,content:n.content});
 },{resolveInstances:true});
 const empty=Get(b.id,n=>n.name==='Empty JSON window'?n.id:undefined)[0];
 Get(empty,n=>{if(n.type==='text'&&n.enabled!==false&&(n.content||'').length)qa.emptyWindowTextCount++},{resolveInstances:true});
 Export([b.id],'png',OUTPUT,{scale:1});
 const main=Get(b.id,n=>n.name==='JSON number result'?n.id:undefined)[0];
 Export([main],'png',OUTPUT,{scale:2});
 Print('R07_EXPORT',{id:b.id,windowId:main,language:b.theme.language,mode:b.theme.mode});
}
Print('R07_QA',qa);
Print('R07_COMPONENTS',roots.filter(n=>n.reusable&&n.name?.startsWith('R07 ')).map(n=>({id:n.id,name:n.name})));
'''.replace('OUTPUT', json.dumps(str(ROUND)))
    commands = 'execute(' + json.dumps({'description': 'Reopen and export R07 JSON previews', 'input': code}) + ')\nexit()\n'
    with tempfile.TemporaryDirectory(prefix='cubbix-r07-export-') as tmp:
        proc = subprocess.run(['node', str(args.cli_file), 'interactive', '--in', str(source), '--out', str(Path(tmp) / 'reopened.pen')], input=commands, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, cwd=ROOT)
    if proc.returncode or '[ERROR]' in proc.stdout or 'Error:' in proc.stdout:
        raise SystemExit(proc.stdout[-2500:])
    def records(prefix):
        return [json.loads(line[len(prefix):]) for line in proc.stdout.splitlines() if line.startswith(prefix)]
    qa = records('R07_QA ')[0]
    exports = []
    for item in records('R07_EXPORT '):
        for kind, node in [('', item['id']), ('window-', item['windowId'])]:
            filename = f'{kind}{item["language"]}-{item["mode"]}.png'
            target = ROUND / filename
            (ROUND / f'{node}.png').replace(target)
            exports.append({'nodeId': node, 'file': filename, 'scale': 2 if kind else 1, 'sha256': hashlib.sha256(target.read_bytes()).hexdigest()})
    result = {'cliVersion': '0.3.13', 'sourceSha256': hashlib.sha256(source.read_bytes()).hexdigest(), 'nativeLayout': qa, 'components': records('R07_COMPONENTS ')[0], 'exports': exports}
    result['passed'] = len(exports) == 8 and not qa['problems'] and not qa['localizationProblems'] and qa['emptyWindowTextCount'] == 0
    (ROUND / 'render-validation.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'passed': result['passed'], 'exports': len(exports), 'nodes': qa['nodes'], 'clipping': len(qa['problems']), 'localizationProblems': qa['localizationProblems']}, ensure_ascii=False))
    if not result['passed']:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
