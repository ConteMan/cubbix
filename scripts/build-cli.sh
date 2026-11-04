#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
export CGO_ENABLED=0
export GOTOOLCHAIN=go1.27.1
for arch in arm64 amd64; do
  target="resources/darwin-$arch/bin"
  mkdir -p "$target"
  GOOS=darwin GOARCH="$arch" go build -trimpath -o "$target/cubbix" ./cmd/cubbix
  # Go embeds an a.out ad-hoc identifier. Developer ID re-signing names the
  # code after the file, so pin the same name for ad-hoc builds.
  /usr/bin/codesign --force --sign - --identifier cubbix "$target/cubbix"
done
