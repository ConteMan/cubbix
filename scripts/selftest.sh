#!/bin/sh
set -eu

cd "$(dirname "$0")/.."
export CGO_ENABLED=0
export GOTOOLCHAIN=go1.27.1
tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/cubbix-selftest.XXXXXX")
trap 'rm -rf "$tmp_dir"' EXIT HUP INT TERM
export GOCACHE="$tmp_dir/cache"
unset MYGO_UPDATER_PRIVATE_KEY

unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
  printf 'gofmt required:\n%s\n' "$unformatted" >&2
  exit 1
fi
python3 scripts/tokens.py --check
tiffutil -cathidpicheck brand/dist/menubar-template.png brand/dist/menubar-template@2x.png -out "$tmp_dir/menubar-template.tiff"
cmp assets/menubar-template.tiff "$tmp_dir/menubar-template.tiff"
go vet ./...
go test ./...
release_dir="$HOME/Projects/temp/cubbix/release"
mkdir -p "$release_dir"
go run github.com/egoist/mygo/cmd/mygo@v0.6.2 build -platform darwin/arm64 -sign - -skip-dmg -skip-notarize -o "$tmp_dir/build"
rm -rf "$release_dir/Cubbix.app"
ditto "$tmp_dir/build/darwin-arm64/Cubbix.app" "$release_dir/Cubbix.app"
