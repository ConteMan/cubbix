#!/bin/sh
set -eu
[ "$#" -eq 1 ] || exit 2
cli="$1/Contents/Resources/bin/cubbix"
[ -x "$cli" ] || { printf 'Bundled command line tool missing\n' >&2; exit 1; }
/usr/bin/codesign -v "$cli"
identifier=$(/usr/bin/codesign -dv --verbose=4 "$cli" 2>&1 | sed -n 's/^Identifier=//p')
[ "$identifier" = cubbix-cli ] || { printf 'Unexpected CLI signature identifier: %s\n' "$identifier" >&2; exit 1; }
