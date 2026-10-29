#!/bin/sh
set -eu
# Never allow shell tracing to print the updater private key.
set +x

cd "$(dirname "$0")/.."
upload=false
case "${1:-}" in
  '') [ "$#" -eq 0 ] || exit 2 ;;
  --upload) [ "$#" -eq 1 ] || exit 2; upload=true ;;
  *) printf 'Usage: %s [--upload]\n' "$0" >&2; exit 2 ;;
esac

if [ -n "$(git status --porcelain --untracked-files=all)" ]; then
  printf 'Release requires a clean working tree. Commit changes first.\n' >&2
  exit 1
fi
python3 - <<'PY'
import json
import re
from pathlib import Path
version = json.loads(Path('mygo.json').read_text())['version']
if not re.fullmatch(r'(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)', version):
    raise SystemExit('Release requires a stable semantic version')
headings = re.findall(r'^## (.+)$', Path('CHANGELOG.md').read_text(), re.M)
if not headings or headings[0] != version:
    raise SystemExit('Version must match the first CHANGELOG.md section (## x.y.z)')
fallback = re.search(r'const appVersion = "([^"]+)"', Path('main.go').read_text())
if fallback is None or fallback[1] != version:
    raise SystemExit('appVersion must match mygo.json')
PY

export CGO_ENABLED=0
export GOTOOLCHAIN=go1.27.1
unset MYGO_UPDATER_PRIVATE_KEY
update_key_path="${CUBBIX_UPDATE_KEY_PATH:-$HOME/Library/Application Support/cubbix-release/update-keys/mygo-update.key}"
if [ ! -f "$update_key_path" ] || [ ! -r "$update_key_path" ]; then
  printf 'Updater signing key is not readable.\n' >&2
  exit 1
fi
if [ "$(stat -f '%Lp' "$update_key_path")" != 600 ]; then
  printf 'Updater signing key permissions must be 0600.\n' >&2
  exit 1
fi
./scripts/selftest.sh
if [ -n "$(git status --porcelain --untracked-files=all)" ]; then
  printf 'Selftest changed the working tree; refusing to release.\n' >&2
  exit 1
fi
# Only this build process receives the key; it is never copied to another file.
if [ "$upload" = true ]; then
  MYGO_UPDATER_PRIVATE_KEY="$(cat "$update_key_path")" \
    go run github.com/egoist/mygo/cmd/mygo@v0.6.2 build -upload
else
  MYGO_UPDATER_PRIVATE_KEY="$(cat "$update_key_path")" \
    go run github.com/egoist/mygo/cmd/mygo@v0.6.2 build
fi
