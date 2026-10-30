#!/usr/bin/env python3
"""Reopen the R08 source with the pinned headless CLI and export four themes."""
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
ROUND = ROOT / 'design/rounds/R08-2026-10-30-JSON修正'


def main():
    code = r'''
const roots=Get((n,c)=>{c.skipChildren();return {id:n.id,name:n.name,theme:n.theme}});
const boards=roots.filter(n=>/^R08 (zh-CN|en) (light|dark)$/.test(n.name));
const qa={nodes:0,problems:[],syntaxTokens:[],guides:0};
for(const board of boards){
 Get(board.id,(n,c)=>{
  qa.nodes++;
  if(c.problems&&n.enabled!==false)qa.problems.push({name:n.name,problem:c.problems,bounds:c.bounds});
  if(typeof n.fill==='string'&&n.fill.startsWith('$ColorSyntax'))qa.syntaxTokens.push(n.fill);
  if(n.name?.startsWith('Indent guide'))qa.guides++;
 },{resolveInstances:true});
 Export([board.id],'png',OUTPUT,{scale:2});
 Print('R08_EXPORT',{id:board.id,...board.theme});
}
Print('R08_QA',qa);
'''.replace('OUTPUT', json.dumps(str(ROUND)))
    commands = 'execute(' + json.dumps({'description': 'Validate and export R08 JSON themes', 'input': code}) + ')\nexit()\n'
    with tempfile.TemporaryDirectory(prefix='cubbix-r08-') as temporary:
        proc = subprocess.run(['npx', '-y', '@pen.dev/cli@0.3.13', 'interactive',
                               '--in', str(ROOT / 'design/cubbix.pen'),
                               '--out', str(Path(temporary) / 'reopened.pen')],
                              input=commands, text=True, stdout=subprocess.PIPE,
                              stderr=subprocess.STDOUT, cwd=ROOT)
    if proc.returncode or '[ERROR]' in proc.stdout or 'Error:' in proc.stdout:
        raise SystemExit(proc.stdout[-3000:])

    def records(prefix):
        return [json.loads(line[len(prefix):]) for line in proc.stdout.splitlines() if line.startswith(prefix)]

    qa = records('R08_QA ')[0]
    qa['syntaxTokens'] = sorted(set(qa['syntaxTokens']))
    exports = []
    for item in records('R08_EXPORT '):
        filename = f'{item["language"]}-{item["mode"]}.png'
        target = ROUND / filename
        (ROUND / f'{item["id"]}.png').replace(target)
        exports.append({'nodeId': item['id'], 'file': filename, 'scale': 2,
                        'sha256': hashlib.sha256(target.read_bytes()).hexdigest()})
    result = {'cliVersion': '0.3.13', 'nativeLayout': qa, 'exports': exports}
    result['passed'] = len(exports) == 4 and not qa['problems'] and qa['guides'] > 0 and len(qa['syntaxTokens']) == 6
    (ROUND / 'render-validation.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result, ensure_ascii=False, indent=2))
    if not result['passed']:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
