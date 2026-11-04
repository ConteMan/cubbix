#!/bin/sh
set -eu
# The one entry mygo.json's buildCommand calls: every helper binary that ships
# inside Cubbix.app/Contents/Resources/bin.
cd "$(dirname "$0")/.."
./scripts/build-helper.sh
./scripts/build-cli.sh
