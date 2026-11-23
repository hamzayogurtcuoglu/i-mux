#!/bin/sh
# Signs Windows programs with Authenticode through jsign (D100, docs/SIGNING.md).
#
#   ./scripts/sign-windows.sh FILE...        sign each .exe (other files are left alone)
#   ./scripts/sign-windows.sh --any FILE...  sign them whatever their names (NSIS's
#                                            uninstaller is a temporary file)
#   ./scripts/sign-windows.sh --fetch        only fetch jsign (before parallel signing)
#
# Nothing is signed unless IMUX_SIGN_STORETYPE is set (jsign's --storetype:
# TRUSTEDSIGNING, SIGNPATH, ESIGNER, PKCS12 ...), with IMUX_SIGN_KEYSTORE,
# IMUX_SIGN_ALIAS and IMUX_SIGN_STOREPASS (passed to jsign through the
# environment, never on its command line). IMUX_SIGN_TSA is the timestamp
# server (RFC 3161), "none" for no timestamp. Needs java and curl.
set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
JSIGN_VERSION="7.6"
JSIGN_SUM="6cf58aa080e7ff82656f320cff8d4a3bff30408df8766806dcba3740dd839fa5"
JSIGN_URL="https://github.com/ebourg/jsign/releases/download/$JSIGN_VERSION/jsign-$JSIGN_VERSION.jar"
JAR="${IMUX_JSIGN_JAR:-$ROOT/build/jsign-$JSIGN_VERSION.jar}"

die() {
	printf 'sign-windows: %s\n' "$*" >&2
	exit 1
}

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

# fetch puts jsign's jar at JAR, checked against its checksum; a jar already
# there is checked too.
fetch() {
	if [ -f "$JAR" ] && [ "$(sha256_file "$JAR")" = "$JSIGN_SUM" ]; then
		return 0
	fi
	mkdir -p "$(dirname "$JAR")"
	part="$JAR.$$.part"
	curl -fsSL --retry 3 -o "$part" "$JSIGN_URL" || die "could not download $JSIGN_URL"
	if [ "$(sha256_file "$part")" != "$JSIGN_SUM" ]; then
		rm -f "$part"
		die "jsign-$JSIGN_VERSION.jar does not match its checksum"
	fi
	mv -f "$part" "$JAR"
}

if [ -z "${IMUX_SIGN_STORETYPE:-}" ]; then
	exit 0 # signing is off (no certificate configured)
fi
[ -n "${IMUX_SIGN_STOREPASS:-}" ] || [ "$IMUX_SIGN_STORETYPE" = "TRUSTEDSIGNING" ] ||
	die "IMUX_SIGN_STOREPASS is not set"
command -v java >/dev/null 2>&1 || die "java not found (jsign needs it)"
fetch
[ "${1:-}" = "--fetch" ] && exit 0

any=0
if [ "${1:-}" = "--any" ]; then
	any=1
	shift
fi
tsa="${IMUX_SIGN_TSA:-http://timestamp.digicert.com}"
# The loop's list is the files given; set -- inside builds each jsign call.
for f in "$@"; do
	case "$f" in
	*.exe | *.EXE) ;;
	*) [ "$any" -eq 1 ] || continue ;;
	esac
	[ -f "$f" ] || die "no such file: $f"
	set -- --storetype "$IMUX_SIGN_STORETYPE" --alg SHA-256 --replace \
		--name "i-mux" --url "https://github.com/hamzayogurtcuoglu/i-mux"
	[ -n "${IMUX_SIGN_KEYSTORE:-}" ] && set -- "$@" --keystore "$IMUX_SIGN_KEYSTORE"
	[ -n "${IMUX_SIGN_ALIAS:-}" ] && set -- "$@" --alias "$IMUX_SIGN_ALIAS"
	[ -n "${IMUX_SIGN_STOREPASS:-}" ] && set -- "$@" --storepass "env:IMUX_SIGN_STOREPASS"
	[ "$tsa" != "none" ] && set -- "$@" --tsaurl "$tsa" --tsmode RFC3161
	java -jar "$JAR" "$@" "$f" >/dev/null || die "could not sign $f"
	printf 'Signed %s\n' "$f"
done
