#!/usr/bin/env python3
import json
import pathlib
import subprocess
import sys

root = pathlib.Path(__file__).resolve().parent.parent
variables = json.loads((root / "design/tokens.json").read_text())["variables"]
lines = ["package main", "", "type tokenValues struct {"]
for name, spec in variables.items():
    kind = "float32" if spec["type"] == "number" else "string"
    lines.append(f"\t{name} {kind}")
lines += ["}", "", "func tokens(dark bool, language string) tokenValues {", "\tt := tokenValues{"]
for name, spec in variables.items():
    value = spec["value"]
    if isinstance(value, list):
        value = value[0]["value"]
    if isinstance(value, str) and value.startswith("$"):
        continue
    lines.append(f"\t\t{name}: {json.dumps(value, ensure_ascii=False)},")
lines += ["\t}", "\tif dark {"]
for name, spec in variables.items():
    value = spec["value"]
    if isinstance(value, list) and value[0].get("theme", {}).get("mode"):
        lines.append(f"\t\tt.{name} = {json.dumps(value[1]['value'])}")
lines += ["\t}", "\tif language == \"en\" {"]
for name, spec in variables.items():
    value = spec["value"]
    if isinstance(value, list) and value[0].get("theme", {}).get("language"):
        lines.append(f"\t\tt.{name} = {json.dumps(value[1]['value'])}")
lines += ["\t}"]
for name, spec in variables.items():
    value = spec["value"]
    if isinstance(value, str) and value.startswith("$"):
        lines.append(f"\tt.{name} = t.{value[1:]}")
lines += ["\treturn t", "}", ""]
source = subprocess.run(["gofmt"], input="\n".join(lines), text=True, capture_output=True, check=True).stdout
target = root / "tokens.go"
if "--check" in sys.argv:
    if not target.exists() or target.read_text() != source:
        sys.exit("tokens.go differs from design/tokens.json")
    print("tokens match design/tokens.json")
else:
    print(source, end="")
