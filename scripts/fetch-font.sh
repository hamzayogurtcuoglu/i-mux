#!/bin/sh
# Puts the Nerd Font that ships with i-mux (D94) in DIR/fonts: the four
# JetBrainsMono Nerd Font styles a terminal uses and the font's licence
# (OFL.txt), from the Nerd Fonts release archive checked against its SHA-256.
# The release (and CI's snapshot) run it before GoReleaser and the Windows
# installers, which pack DIR/fonts beside imux.
#
#   ./scripts/fetch-font.sh [DIR]      (default: build)
#
# The version and checksum are internal/nerdfont's Release and Sum; each
# style's own checksum there is what imux checks the copy against.
set -eu

ROOT=$(cd "$(dirname "$0")/.." && pwd)
DIR="${1:-$ROOT/build}"
RELEASE="v3.5.1"
ARCHIVE="JetBrainsMono.tar.xz"
SUM="04d5e8f903693f9dd13e16f867e994834e681eb3c72c0d337a770dcda09010cf"
URL="${IMUX_NERD_FONT_URL:-https://github.com/ryanoasis/nerd-fonts/releases/download/$RELEASE/$ARCHIVE}"
STYLES="JetBrainsMonoNerdFont-Regular.ttf JetBrainsMonoNerdFont-Bold.ttf JetBrainsMonoNerdFont-Italic.ttf JetBrainsMonoNerdFont-BoldItalic.ttf"

die() {
	printf 'fetch-font: %s\n' "$*" >&2
	exit 1
}

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

# The release and checksum must be the ones imux checks.
grep -q "Release = \"$RELEASE\"" "$ROOT/internal/nerdfont/nerdfont.go" || die "internal/nerdfont's Release is not $RELEASE"
grep -q "Sum = \"$SUM\"" "$ROOT/internal/nerdfont/nerdfont.go" || die "internal/nerdfont's Sum is not this script's"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM
i=0
until curl -fsSL --retry 3 -o "$tmp/$ARCHIVE" "$URL"; do
	i=$((i + 1))
	[ "$i" -lt 3 ] || die "could not download $URL"
	sleep 10
done
[ "$(sha256_file "$tmp/$ARCHIVE")" = "$SUM" ] || die "$ARCHIVE does not match its checksum"
# shellcheck disable=SC2086 # STYLES is a list of file names
tar -xJf "$tmp/$ARCHIVE" -C "$tmp" $STYLES OFL.txt

mkdir -p "$DIR/fonts"
for f in $STYLES OFL.txt; do
	cp "$tmp/$f" "$DIR/fonts/$f"
	chmod 644 "$DIR/fonts/$f"
done
printf 'Put JetBrainsMono Nerd Font %s in %s\n' "$RELEASE" "$DIR/fonts"
