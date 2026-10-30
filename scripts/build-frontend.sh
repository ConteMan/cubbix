#!/bin/sh
set -eu
export npm_config_update_notifier=false
cd "$(dirname "$0")/../frontend"
# Installation is isolated: the repository retains source, lockfile and dist only.
task_tmp=$(mktemp -d "${TMPDIR:-/tmp}/cubbix-frontend.XXXXXX")
trap 'rm -rf "$task_tmp"' EXIT HUP INT TERM
cp package.json package-lock.json build.mjs "$task_tmp/"
cp -R src "$task_tmp/src"
cp -R test "$task_tmp/test"
# Source imports refer to the checked-in Lucide assets.
mkdir -p "$task_tmp/design/assets"
cp -R ../design/assets/lucide-0.468.0 "$task_tmp/design/assets/"
# Make source imports independent of the build checkout's absolute path.
(cd "$task_tmp" && npm ci --ignore-scripts --no-audit --no-fund --cache "$task_tmp/npm-cache" >/dev/null)
node "$task_tmp/build.mjs" "$task_tmp/dist"
if [ "${1:-}" = --check ]; then
  diff -rq dist "$task_tmp/dist"
  printf 'frontend dist matches locked rebuild\n'
  (cd "$task_tmp" && npm test)
else
  mkdir -p dist
  cp "$task_tmp/dist/"* dist/
fi
