#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
export CGO_ENABLED=0
export GOTOOLCHAIN=go1.27.1
for arch in arm64 amd64; do
  target="resources/darwin-$arch/bin"
  mkdir -p "$target"
  GOOS=darwin GOARCH="$arch" go build -trimpath -o "$target/cubbix" ./cmd/cubbix
  # Go embeds an a.out ad-hoc identifier; MyGo preserves an existing identifier.
  # Pin it on the generated resource before MyGo applies the final signature.
  /usr/bin/codesign --force --sign - --identifier cubbix-cli "$target/cubbix"
done
