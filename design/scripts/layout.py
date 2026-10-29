#!/usr/bin/env python3
"""Check resolved top-level canvas bounds, including labels and reusable refs."""
import argparse
import json
import math
import os
from pathlib import Path
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
MIN_GAP = 80


def check(nodes):
    errors = []
    valid = []
    for node in nodes:
        bounds = node.get('bounds', {})
        values = [bounds.get(key) for key in ('x', 'y', 'width', 'height')]
        if (not all(isinstance(v, (int, float)) and not isinstance(v, bool)
                    and math.isfinite(v) for v in values)
                or values[2] <= 0 or values[3] <= 0):
            errors.append({'node': node['id'], 'reason': 'Invalid resolved bounds'})
        else:
            valid.append(node)
    minimum = None
    for index, first in enumerate(valid):
        a = first['bounds']
        for second in valid[index + 1:]:
            b = second['bounds']
            dx = max(a['x'] - b['x'] - b['width'], b['x'] - a['x'] - a['width'], 0)
            dy = max(a['y'] - b['y'] - b['height'], b['y'] - a['y'] - a['height'], 0)
            gap = math.hypot(dx, dy)
            minimum = gap if minimum is None else min(minimum, gap)
            if gap < MIN_GAP:
                errors.append({'first': first['id'], 'second': second['id'],
                               'names': [first['name'], second['name']], 'gap': gap})
    if not nodes:
        errors.append({'reason': 'No top-level nodes returned'})
    return {'minimumRequiredGap': MIN_GAP, 'minimumGap': minimum,
            'nodeCount': len(nodes), 'errors': errors, 'passed': not errors}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--source', type=Path, default=ROOT / 'design/cubbix.pen')
    parser.add_argument('--cli-file', type=Path, default=os.environ.get('PEN_CLI_FILE'))
    parser.add_argument('--out', type=Path)
    args = parser.parse_args()
    cli = ['node', str(args.cli_file)] if args.cli_file else ['npx', '-y', '@pen.dev/cli@0.3.13']
    code = ('Print("LAYOUT_BOUNDS",Get((n,c)=>{c.skipChildren();'
            'return {id:n.id,name:n.name,bounds:c.bounds}}));')
    commands = 'execute(' + json.dumps({'description': 'Check top-level canvas spacing', 'input': code}) + ')\nexit()\n'
    with tempfile.TemporaryDirectory(prefix='cubbix-layout-') as temporary:
        process = subprocess.run(cli + ['interactive', '--in', str(args.source.resolve()),
                                        '--out', str(Path(temporary) / 'reopened.pen')],
                                 input=commands, text=True, stdout=subprocess.PIPE,
                                 stderr=subprocess.STDOUT, cwd=ROOT)
    if process.returncode or '[ERROR]' in process.stdout or 'Error:' in process.stdout:
        raise SystemExit('Native canvas bounds could not be read; layout check failed.')
    rows = [line.removeprefix('LAYOUT_BOUNDS ') for line in process.stdout.splitlines()
            if line.startswith('LAYOUT_BOUNDS ')]
    if len(rows) != 1:
        raise SystemExit('Expected exactly one native canvas bounds result.')
    result = check(json.loads(rows[0]))
    output = json.dumps(result, ensure_ascii=False, indent=2) + '\n'
    if args.out:
        args.out.write_text(output)
    print(output, end='')
    raise SystemExit(0 if result['passed'] else 1)


if __name__ == '__main__':
    main()
