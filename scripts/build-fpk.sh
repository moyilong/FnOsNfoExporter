#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
arch="${1:-amd64}"
version="${2:-0.1.0}"
case "$arch" in amd64) platform=x86;; arm64) platform=arm;; *) echo "unsupported architecture"; exit 1;; esac
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]] || { echo "invalid version"; exit 1; }
mkdir -p build dist
stage=$(mktemp -d "$PWD/build/fpk.XXXXXX")
trap 'rm -rf "$stage"' EXIT
cp -R packaging/. "$stage/"
sed -i "s/^platform=.*/platform=$platform/;s/^version=.*/version=$version/" "$stage/manifest"
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -ldflags="-s -w" -o "$stage/app/fnos-nfo-exporter" ./cmd/fnos-nfo-exporter
go run ./tools/icon "$stage"
chmod +x "$stage"/cmd/* "$stage/app/fnos-nfo-exporter"
(cd "$stage" && fnpack build)
package=$(find "$stage" -maxdepth 1 -name '*.fpk' -print -quit)
[ -n "$package" ] || { echo "fnpack did not produce a package"; exit 1; }
cp "$package" "dist/fnos-nfo-exporter-$version-$arch.fpk"
(cd dist && sha256sum "fnos-nfo-exporter-$version-$arch.fpk" > "fnos-nfo-exporter-$version-$arch.fpk.sha256")
