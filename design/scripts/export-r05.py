#!/usr/bin/env python3
"""Reopen and export the four R05 review boards with pen.dev CLI 0.3.13."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
ROUND = ROOT / 'rounds/R05-2026-10-29-常亮'

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--cli-file', type=Path, required=True)
    args = parser.parse_args()
    source = ROOT / 'cubbix.pen'
    doc = json.loads(source.read_text())
    boards = [n for n in doc['children'] if n.get('metadata', {}).get('type') == 'r05-review']
    ids = [n['id'] for n in boards]
    code = ('qa={nodes:0,problems:[],settings:[],sections:[]};'
            'for(const root of ' + json.dumps(ids) + ')Get(root,(n,c)=>{qa.nodes++;'
            'if(c.problems&&n.enabled!==false)qa.problems.push({id:n.id,name:n.name,problem:c.problems,bounds:c.bounds});'
            'if(n.name==="R04 settings shell")qa.settings.push({id:n.id,width:n.width,height:n.height});'
            'if(n.name&&n.name.startsWith("Section / "))qa.sections.push(n.name);'
            '},{resolveInstances:true});Print("R05_QA",qa);'
            'Export(' + json.dumps(ids) + ',"png",' + json.dumps(str(ROUND)) + ',{scale:1});')
    commands = 'execute(' + json.dumps({'description': '复开检查 R05 并导出四张样张', 'input': code}) + ')\nexit()\n'
    result = subprocess.run(['node', str(args.cli_file), 'interactive', '--in', str(source), '--out', str(source)], input=commands, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    if result.returncode or '[ERROR]' in result.stdout or 'Error:' in result.stdout:
        raise SystemExit('Headless export failed: ' + result.stdout[-2000:])
    qa = next((json.loads(line.split('R05_QA ', 1)[1]) for line in result.stdout.splitlines() if line.startswith('R05_QA ')), None)
    if not qa:
        raise SystemExit('No native layout evidence returned.')
    exports = []
    for b in boards:
        filename = b['theme']['language'] + '-' + b['theme']['mode'] + '.png'
        target = ROUND / filename
        (ROUND / (b['id'] + '.png')).replace(target)
        exports.append({'nodeId': b['id'], 'file': filename, 'sha256': hashlib.sha256(target.read_bytes()).hexdigest()})
    (ROUND / 'render-validation.json').write_text(json.dumps({'cliVersion': '0.3.13', 'scale': 1, 'sourceSha256': hashlib.sha256(source.read_bytes()).hexdigest(), 'nativeLayout': qa, 'exports': exports}, ensure_ascii=False, indent=2) + '\n')
    print(f'Exported {len(exports)} PNGs; inspected {qa["nodes"]} nodes; layout problems: {len(qa["problems"])}.')
    if qa['problems']:
        print(json.dumps(qa['problems'], ensure_ascii=False, indent=2))
        raise SystemExit(1)

if __name__ == '__main__':
    main()
