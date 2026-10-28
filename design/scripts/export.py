#!/usr/bin/env python3
"""Reopen the native design and export its review boards with the headless CLI."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path(__file__).resolve().parents[1]
ROUND = ROOT / 'rounds' / 'R03-2026-10-28-黑白灰'
CLI_VERSION = '0.3.13'


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--cli-file', type=Path, help='Use an existing CLI JavaScript entry point.')
    args = parser.parse_args()
    document = json.loads((ROOT / 'cubbix.pen').read_text())
    boards = [node for node in document['children'] if not node.get('reusable')]
    samples = [node['id'] for node in boards]
    ids = [node['id'] for node in boards]
    code = ('qa={nodes:0,problems:[]};for(const root of ' + json.dumps(samples)
            + ')Get(root,(n,c)=>{qa.nodes++;if(c.problems&&n.enabled!==false)'
              'qa.problems.push({id:n.id,name:n.name,problem:c.problems,bounds:c.bounds})},'
              '{resolveInstances:true});Print(qa);Export('
            + json.dumps(ids) + ',"png",' + json.dumps(str(ROUND)) + ',{scale:2});')
    commands = 'execute(' + json.dumps({'description': '复开检查并导出评审 PNG', 'input': code}, ensure_ascii=False) + ')\nexit()\n'
    ROUND.mkdir(parents=True, exist_ok=True)
    cli = ['node', str(args.cli_file)] if args.cli_file else ['npx', '-y', f'@pen.dev/cli@{CLI_VERSION}']
    result = subprocess.run(cli + ['interactive',
                             '--in', str(ROOT / 'cubbix.pen'), '--out', str(ROOT / 'cubbix.pen')],
                            input=commands, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    if result.returncode or '[ERROR]' in result.stdout or 'Error:' in result.stdout:
        raise SystemExit('Headless export failed; inspect the CLI locally before publishing.')
    native_qa = next((json.loads(line) for line in result.stdout.splitlines() if line.startswith('{"nodes":')), None)
    if native_qa is None or native_qa['nodes'] == 0 or native_qa['problems']:
        raise SystemExit('Native layout verification did not pass.')
    exports = []
    for board in boards:
        filename = {'01 Foundations': 'foundations', '02 Components and states': 'components'}.get(board['name'], board['name']) + '.png'
        source = ROUND / (board['id'] + '.png')
        target = ROUND / filename
        source.replace(target)
        exports.append({'nodeId': board['id'], 'name': board['name'], 'file': filename, 'sha256': sha(target)})
    validation = {'cliVersion': CLI_VERSION, 'scale': 2, 'sourceSha256': sha(ROOT / 'cubbix.pen'),
                  'nativeLayout': native_qa, 'exports': exports}
    (ROUND / 'render-validation.json').write_text(json.dumps(validation, ensure_ascii=False, indent=2) + '\n')
    print(f'Exported {len(exports)} PNGs; inspected {native_qa["nodes"]} board nodes; no clipping.')


if __name__ == '__main__':
    main()
