#!/bin/sh
# Cross-build the per-user Windows NSIS installer on Linux (D85, T90.8).
#
#   ./scripts/build-installer.sh [--version X.Y.Z] [--arch amd64|arm64|all] [--out-dir DIR]
#
# Builds imux.exe and imux-core.exe for Windows (when --bin-dir is not given),
# packages them with makensis into imux_<version>_windows_<arch>_setup.exe,
# and writes a rendered WinGet manifest beside them when both amd64 and arm64
# installers are present.
set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
VERSION="${IMUX_VERSION:-}"
ARCH="all"
OUT_DIR="$ROOT/bin"
BIN_DIR=""

die() {
	printf 'build-installer: %s\n' "$*" >&2
	exit 1
}

while [ $# -gt 0 ]; do
	case "$1" in
	--version)
		[ $# -ge 2 ] || die "--version needs a value"
		VERSION="$2"
		shift 2
		;;
	--arch)
		[ $# -ge 2 ] || die "--arch needs amd64, arm64 or all"
		ARCH="$2"
		shift 2
		;;
	--out-dir)
		[ $# -ge 2 ] || die "--out-dir needs a directory"
		OUT_DIR="$2"
		shift 2
		;;
	--bin-dir)
		[ $# -ge 2 ] || die "--bin-dir needs a directory"
		BIN_DIR="$2"
		shift 2
		;;
	*)
		die "unknown argument: $1"
		;;
	esac
done

command -v makensis >/dev/null 2>&1 || die "makensis not found (install nsis)"

if [ -z "$VERSION" ]; then
	VERSION=$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")
fi
VERSION="${VERSION#v}"
COMMIT=$(git -C "$ROOT" rev-parse --short=12 HEAD 2>/dev/null || echo "unknown")
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
PKG="github.com/hamzayogurtcuoglu/i-mux"

mkdir -p "$OUT_DIR"
OUT_DIR=$(cd "$OUT_DIR" && pwd)

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

build_one() {
	a="$1"
	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT INT TERM
	src="$BIN_DIR"
	if [ -z "$src" ]; then
		src="$tmp"
		LDFLAGS="-s -w -X $PKG/internal/version.Version=$VERSION -X $PKG/internal/version.Commit=$COMMIT -X $PKG/internal/version.Date=$DATE"
		(cd "$ROOT" && CGO_ENABLED=0 GOOS=windows GOARCH="$a" go build -trimpath -ldflags "$LDFLAGS" -o "$src/imux.exe" ./cmd/imux)
		(cd "$ROOT" && CGO_ENABLED=0 GOOS=windows GOARCH="$a" go build -trimpath -ldflags "-s -w" -o "$src/imux-core.exe" ./cmd/imux-core)
	fi
	out="$OUT_DIR/imux_${VERSION}_windows_${a}_setup.exe"
	makensis -V2 \
		"-DVERSION=$VERSION" \
		"-DARCH=$a" \
		"-DBIN_DIR=$src" \
		"-DROOT_DIR=$ROOT" \
		"-DOUT_FILE=$out" \
		"$ROOT/scripts/installer.nsi"
	rm -rf "$tmp"
	trap - EXIT INT TERM
	printf 'Built %s\n' "$out"
}

case "$ARCH" in
amd64 | arm64)
	build_one "$ARCH"
	;;
all)
	build_one amd64
	build_one arm64
	;;
*)
	die "unsupported architecture: $ARCH (want amd64, arm64 or all)"
	;;
esac

amd64_exe="$OUT_DIR/imux_${VERSION}_windows_amd64_setup.exe"
arm64_exe="$OUT_DIR/imux_${VERSION}_windows_arm64_setup.exe"
manifest_src="$ROOT/packaging/winget/hamzayogurtcuoglu.i-mux.yaml"
if [ -f "$amd64_exe" ] && [ -f "$arm64_exe" ] && [ -f "$manifest_src" ]; then
	amd64_sha=$(sha256_file "$amd64_exe" | tr '[:lower:]' '[:upper:]')
	arm64_sha=$(sha256_file "$arm64_exe" | tr '[:lower:]' '[:upper:]')
	sed \
		-e "s|0\.65\.0|$VERSION|g" \
		-e "s|0000000000000000000000000000000000000000000000000000000000000001|$amd64_sha|g" \
		-e "s|0000000000000000000000000000000000000000000000000000000000000002|$arm64_sha|g" \
		"$manifest_src" >"$OUT_DIR/hamzayogurtcuoglu.i-mux.yaml"
fi
