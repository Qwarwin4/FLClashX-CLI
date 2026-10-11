#!/bin/sh
set -eu

TAG=${1:-}
case "$TAG" in
v[0-9]*.[0-9]*.[0-9]*) ;;
*)
	echo "usage: ./release.sh v0.1.0" >&2
	exit 2
	;;
esac

cd "$(dirname "$0")"
[ -f go.sum ] || go mod tidy
go mod verify

MIHOMO=$(go list -m -f '{{.Version}}' github.com/metacubex/mihomo)
export CGO_ENABLED=0 GOOS=linux
rm -rf dist
mkdir -p dist

for arch in ${ARCHES:-amd64 arm64}; do
	name=flc-$TAG-linux-$arch
	out=dist/$name
	echo "==> $name"
	mkdir -p "$out"
	chmod 755 "$out"
	GOARCH=$arch go build -trimpath -tags with_gvisor \
		-ldflags "-s -w -buildid= -X main.version=${TAG#v} -X github.com/metacubex/mihomo/constant.Version=$MIHOMO" \
		-o "$out/flc" ./src
	cp README.md install.sh "$out/"
	chmod 755 "$out/flc" "$out/install.sh"
	chmod 644 "$out/README.md"
	tar --owner=0 --group=0 --numeric-owner --sort=name -C dist -czf "dist/$name.tar.gz" "$name"
	rm -rf "$out"
done

(cd dist && sha256sum ./*.tar.gz | sed 's#\./##' >SHA256SUMS)

echo
ls -1 dist
echo
echo "upload everything in dist/ to the $TAG release, e.g.:"
echo "  gh release create $TAG dist/* --title $TAG"
