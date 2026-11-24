#!/bin/sh
set -eu

PREFIX=${PREFIX:-/usr/local}
LIB=$PREFIX/lib/flc

die() {
	printf 'install: %s\n' "$*" >&2
	exit 1
}

has() { command -v "$1" >/dev/null 2>&1; }

has_setcap() { has setcap || [ -x /usr/sbin/setcap ] || [ -x /sbin/setcap ]; }

go_minor() {
	go env GOVERSION 2>/dev/null | sed -n 's/^go1\.\([0-9]*\).*/\1/p'
}

pkgs() {
	go=$1
	cap=$2
	if has pacman; then
		$SU pacman -S --needed $go${go:+ }$cap
	elif has apt-get; then
		$SU apt-get update
		$SU apt-get install ${go:+golang-go }${cap:+libcap2-bin}
	elif has dnf; then
		$SU dnf install ${go:+golang }${cap:+libcap}
	elif has zypper; then
		$SU zypper install ${go:+go }${cap:+libcap-progs}
	elif has xbps-install; then
		$SU xbps-install -S ${go:+go }${cap:+libcap-progs}
	elif has apk; then
		$SU apk add ${go:+go }${cap:+libcap-utils }shadow
	else
		die "не знаю твой пакетный менеджер, поставь вручную:${go:+ Go 1.21+}${cap:+ setcap (libcap)}"
	fi
}

deps() {
	go=""
	cap=""
	[ "$1" = build ] && ! has go && go=go
	has_setcap || cap=libcap
	[ -z "$go$cap" ] && return 0

	echo "==> ставлю зависимости: $go $cap"
	pkgs "$go" "$cap"
	[ -z "$go" ] || has go || die "go так и не появился в PATH"
	has_setcap || die "setcap так и не появился"
}

build() {
	v=$(go_minor)
	[ -n "$v" ] || die "не понял версию go: $(go version)"
	[ "$v" -ge 21 ] || die "go 1.$v слишком старый, поставь 1.21+ с https://go.dev/dl"
	if [ "$v" -lt 24 ]; then
		echo "==> системный go 1.$v, go скачает 1.24 сам"
		export GOTOOLCHAIN=auto
	fi

	VER=${FLC_VERSION:-}
	VER=${VER#v}
	LDV=""
	[ -n "$VER" ] && LDV="-X main.version=$VER"

	echo "==> go-модули"
	[ -f go.sum ] || go mod tidy
	go mod verify

	echo "==> сборка"
	export CGO_ENABLED=0
	MIHOMO=$(go list -m -f '{{.Version}}' github.com/metacubex/mihomo)
	mkdir -p build
	go build -trimpath -tags with_gvisor \
		-ldflags "-s -w -buildid= $LDV -X github.com/metacubex/mihomo/constant.Version=$MIHOMO" \
		-o build/flc ./src
}

[ "$(uname -s)" = Linux ] || die "FLClashX CLI работает только на Linux"
[ "$(id -u)" -ne 0 ] || die "запускай от обычного пользователя, права root попрошу сам"

cd "$(dirname "$0")"

if has sudo; then
	SU=sudo
elif has doas; then
	SU=doas
else
	die "нужен sudo или doas"
fi

if [ -f go.mod ] && [ -d src ]; then
	deps build
	build
	BIN=build/flc
	[ -n "${FLC_BUILD_ONLY:-}" ] && exit 0
elif [ -f flc ]; then
	deps bin
	BIN=./flc
	chmod 755 "$BIN"
else
	die "рядом нет ни исходников, ни готового flc"
fi

echo "==> установка в $PREFIX"
$SU install -d -m 755 -o root -g root "$PREFIX/bin" "$LIB"
$SU install -m 755 -o root -g root "$BIN" "$PREFIX/bin/flc"
$SU install -m 755 -o root -g root "$BIN" "$LIB/flc-core"
$SU setcap cap_net_admin,cap_net_bind_service=+ep "$LIB/flc-core"
$SU rm -f "$LIB/uninstall.sh"
$SU groupdel flc 2>/dev/null || true

echo
echo "готово: $PREFIX/bin/flc ($("$BIN" version | head -n 1))"
echo "дальше: flc add-profile --link <ссылка> && flc start"
