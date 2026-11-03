#!/bin/sh
set -eu
[ "$#" -eq 1 ] || exit 2
helper="$1/Contents/Resources/bin/cubbix-helper"
[ -x "$helper" ] || { printf 'Bundled helper missing\n' >&2; exit 1; }
/usr/bin/codesign -v "$helper"
identifier=$(/usr/bin/codesign -dv --verbose=4 "$helper" 2>&1 | sed -n 's/^Identifier=//p')
[ "$identifier" = cubbix-helper ] || { printf 'Unexpected helper signature identifier: %s\n' "$identifier" >&2; exit 1; }
