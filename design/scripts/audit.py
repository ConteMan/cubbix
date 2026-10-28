#!/usr/bin/env python3
"""Audit authored canvas values, including every nested instance override."""
import argparse
from collections import Counter
import json
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]


def audit():
    document = json.loads((ROOT / 'cubbix.pen').read_text())
    tokens = json.loads((ROOT / 'tokens.json').read_text())['variables']
    literals = {key: [] for key in ('colors', 'cornerRadius', 'fontSize', 'spacing')}
    missing = []
    components, refs, samples, placeholders = [], [], [], []
    authored_nodes = []

    def leaves(value):
        if isinstance(value, list):
            for item in value:
                yield from leaves(item)
        elif isinstance(value, dict):
            for item in value.values():
                yield from leaves(item)
        else:
            yield value

    def walk(value, path='children'):
        if isinstance(value, list):
            for index, child in enumerate(value):
                walk(child, f'{path}[{index}]')
        elif isinstance(value, dict):
            if value.get('type') in ('frame', 'ref', 'text') and value.get('name'):
                authored_nodes.append(value)
            if value.get('reusable'):
                components.append({'id': value['id'], 'name': value['name']})
            if value.get('type') == 'ref':
                refs.append({'id': value.get('id'), 'ref': value['ref']})
            if value.get('metadata', {}).get('type') == 'sample':
                samples.append(value['metadata'])
            if value.get('placeholder'):
                placeholders.append(path)
            for key, child in value.items():
                for scalar in leaves(child) if key in ('fill', 'stroke', 'color', 'cornerRadius', 'fontSize', 'padding', 'gap', 'letterSpacing') else []:
                    if isinstance(scalar, str) and re.fullmatch(r'#[0-9a-fA-F]{3,8}', scalar):
                        literals['colors'].append({'path': f'{path}.{key}', 'value': scalar})
                    if isinstance(scalar, (int, float)) and not isinstance(scalar, bool):
                        category = {'cornerRadius': 'cornerRadius', 'fontSize': 'fontSize', 'padding': 'spacing', 'gap': 'spacing', 'letterSpacing': 'spacing'}.get(key)
                        if category:
                            literals[category].append({'path': f'{path}.{key}', 'value': scalar})
                    if isinstance(scalar, str) and scalar.startswith('$') and scalar[1:] not in document['variables']:
                        missing.append({'path': f'{path}.{key}', 'variable': scalar})
                if isinstance(child, (dict, list)):
                    walk(child, f'{path}.{key}')

    walk(document['children'])
    component_ids = {item['id'] for item in components}
    dangling_refs = [item for item in refs if item['ref'] not in component_ids]
    core_samples = [s for s in samples if s['page'] in ('general', 'about')]
    prompt_samples = [s for s in samples if s['page'] == 'restart-confirmation']
    matrix = {(s['page'], s['language'], s['mode']) for s in core_samples}
    prompts = {(s['language'], s['mode']) for s in prompt_samples}
    expected = {(p, l, m) for p in ('general', 'about') for l in ('zh-CN', 'en') for m in ('light', 'dark')}
    token_drift = [key for key, value in tokens.items() if document['variables'].get(key) != value]
    by_name = {node['name']: node for node in authored_nodes if node.get('reusable')}
    segmented = by_name['Segmented control']
    brand = by_name['Brand placeholder · 待替换为 Logo']
    default_boards = [node for node in document['children'] if node.get('metadata', {}).get('type') == 'sample']
    r03_checks = {
        'noRepeatedPageHeaderOrCloseButton': not any(node['name'] in ('Page header', 'Page title', 'Close settings') for node in authored_nodes),
        'fixedWidthAndContentHeight': all(node['width'] == 720 and node['height'] == (176 if node['metadata']['page'] == 'general' else 316) for node in default_boards),
        'modeOnlySamples': all('accent' not in node['theme'] for node in default_boards),
        'languageOptionsInOrder': [node['name'] for node in segmented['children']] == ['System segment', 'Chinese segment', 'English segment'],
        'defaultLanguageIsChinese': segmented['children'][1].get('fill') == '$ColorAccent',
        'brandPlaceholderMarked': brand['metadata'].get('status') == '待替换为 Logo',
        'accentAliasesCentralized': all(tokens[key]['value'] == '$' + target for key, target in {'ColorAccent': 'AccentFill', 'ColorAccentSoft': 'AccentSoft', 'ColorOnAccent': 'AccentOnFill'}.items()),
        'noComparisonBoard': not any(node.get('name') == 'accent-comparison' for node in document['children']),
        'neutralSuccess': tokens['ColorSuccess']['value'] == '$ColorText',
        'selectedTextUsesOnAccent': segmented['children'][1]['children'][0].get('fill') == '$ColorOnAccent',
        'primaryButtonTextUsesOnAccent': by_name['Text button']['children'][0].get('fill') == '$ColorOnAccent',
        'accentUsesModeOnly': all(set(entry.get('theme', {})) == {'mode'} for key in ('AccentFill', 'AccentSoft', 'AccentOnFill') for entry in tokens[key]['value']),
    }
    result = {
        'schemaVersion': 1,
        'scope': 'All authored canvas children and descendants; variable definitions excluded; geometry x/y/width/height excluded from spacing.',
        'styleTokens': len(tokens),
        'styleTokensByType': dict(Counter(value['type'] for value in tokens.values())),
        'documentVariables': len(document['variables']),
        'copyResourceKeys': len(json.loads((ROOT / 'locales/zh-CN.json').read_text())),
        'reusableComponents': components,
        'componentCount': len(components),
        'authoredRefCount': len(refs),
        'samples': len(samples),
        'literalCounts': {key: {'occurrences': len(values), 'uniqueValues': len({str(item['value']) for item in values})} for key, values in literals.items()},
        'literalDetails': literals,
        'missingVariables': missing,
        'danglingRefs': dangling_refs,
        'tokenDrift': token_drift,
        'r03Checks': r03_checks,
        'unfinishedPlaceholders': placeholders,
        'completeSampleMatrix': matrix == expected and len(core_samples) == 8,
        'completePromptMatrix': prompts == {(l, m) for l in ('zh-CN', 'en') for m in ('light', 'dark')} and len(prompt_samples) == 4,
    }
    result['passed'] = not any(literals.values()) and not any((missing, dangling_refs, token_drift, placeholders)) and result['completeSampleMatrix'] and result['completePromptMatrix'] and all(r03_checks.values())
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--out', type=Path)
    args = parser.parse_args()
    result = audit()
    output = json.dumps(result, ensure_ascii=False, indent=2) + '\n'
    if args.out:
        args.out.write_text(output)
    print(output, end='')
    raise SystemExit(0 if result['passed'] else 1)
