#!/bin/sh
# Install ajan: https://github.com/hamzayogurtcuoglu/i-mux
#
#   curl -fsSL https://raw.githubusercontent.com/hamzayogurtcuoglu/i-mux/main/install.sh | sh
#   sh install.sh [--prefix DIR] [--version vX.Y.Z] [--yes]
#
# Downloads the release archive for this OS and architecture, verifies its
# sha256 against checksums.txt, and installs the binary to ~/.local/bin (or
# DIR). If tmux is missing it offers to install it with the system package
# manager; it asks first (--yes skips the question) and says so before it
# uses sudo.
set -eu

REPO="hamzayogurtcuoglu/i-mux"
PREFIX="${HOME}/.local/bin"
VERSION=""
YES=0
BASE_URL="${AJAN_INSTALL_BASE_URL:-https://github.com/${REPO}/releases/download}"

say() { printf '%s\n' "$*"; }
die() {
	printf 'ajan install: %s\n' "$*" >&2
	exit 1
}

while [ $# -gt 0 ]; do
	case "$1" in
	--prefix)
		[ $# -ge 2 ] || die "--prefix needs a directory"
		PREFIX="$2"
		shift 2
		;;
	--prefix=*)
		PREFIX="${1#--prefix=}"
		shift
		;;
	--version)
		[ $# -ge 2 ] || die "--version needs a tag like v0.1.0"
		VERSION="$2"
		shift 2
		;;
	--version=*)
		VERSION="${1#--version=}"
		shift
		;;
	-y | --yes)
		YES=1
		shift
		;;
	-h | --help)
		sed -n '2,12p' "$0" 2>/dev/null | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*) die "unknown option: $1" ;;
	esac
done

have() { command -v "$1" >/dev/null 2>&1; }

fetch() { # fetch URL FILE
	if have curl; then
		curl -fsSL -o "$2" "$1"
	elif have wget; then
		wget -q -O "$2" "$1"
	else
		die "neither curl nor wget is installed"
	fi
}

sha256() {
	if have sha256sum; then
		sha256sum "$1" | cut -d' ' -f1
	elif have shasum; then
		shasum -a 256 "$1" | cut -d' ' -f1
	else
		die "neither sha256sum nor shasum is installed"
	fi
}

# confirm QUESTION: yes with --yes; no when there is no terminal to ask on.
confirm() {
	[ "$YES" -eq 1 ] && return 0
	[ -r /dev/tty ] || return 1
	printf '%s [y/N] ' "$1"
	read -r answer </dev/tty || return 1
	case "$answer" in
	y | Y | yes | YES) return 0 ;;
	esac
	return 1
}

os=$(uname -s)
case "$os" in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) die "unsupported OS: $os (ajan runs on macOS and Linux; on Windows use WSL)" ;;
esac
arch=$(uname -m)
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) die "unsupported architecture: $arch" ;;
esac

if [ -z "$VERSION" ]; then
	tmpv=$(mktemp)
	fetch "https://api.github.com/repos/${REPO}/releases/latest" "$tmpv" || die "could not look up the latest release"
	VERSION=$(sed -n 's/.*"tag_name"[^"]*"\([^"]*\)".*/\1/p' "$tmpv" | head -n 1)
	rm -f "$tmpv"
	[ -n "$VERSION" ] || die "could not find the latest release"
fi
num="${VERSION#v}"
archive="ajan_${num}_${os}_${arch}.tar.gz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM
say "Downloading ajan ${VERSION} for ${os}/${arch}..."
fetch "${BASE_URL}/${VERSION}/${archive}" "$tmp/$archive" || die "download failed: ${BASE_URL}/${VERSION}/${archive}"
fetch "${BASE_URL}/${VERSION}/checksums.txt" "$tmp/checksums.txt" || die "download failed: checksums.txt"
want=$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || die "$archive is not listed in checksums.txt"
got=$(sha256 "$tmp/$archive")
[ "$want" = "$got" ] || die "checksum mismatch for $archive (expected $want, got $got)"
say "Checksum verified."

tar -xzf "$tmp/$archive" -C "$tmp" ajan || die "could not unpack $archive"
mkdir -p "$PREFIX" || die "cannot create $PREFIX"
if ! { cp "$tmp/ajan" "$PREFIX/ajan.tmp.$$" && chmod 755 "$PREFIX/ajan.tmp.$$" && mv -f "$PREFIX/ajan.tmp.$$" "$PREFIX/ajan"; }; then
	die "cannot install to $PREFIX (use --prefix)"
fi
say "Installed $PREFIX/ajan"

case ":${PATH}:" in
*":${PREFIX}:"*) ;;
*)
	say ""
	say "$PREFIX is not on your PATH. Add this to your shell profile:"
	say "  export PATH=\"$PREFIX:\$PATH\""
	;;
esac

# tmux is ajan's only runtime dependency.
if have tmux; then
	say "tmux found: $(tmux -V)"
else
	say ""
	say "ajan needs tmux, which is not installed."
	pm=""
	if have brew; then
		pm="brew install tmux"
	elif have apt-get; then
		pm="apt-get install -y tmux"
	elif have dnf; then
		pm="dnf install -y tmux"
	elif have pacman; then
		pm="pacman -S --noconfirm tmux"
	elif have apk; then
		pm="apk add tmux"
	fi
	if [ -z "$pm" ]; then
		say "Install tmux with your package manager, then run: ajan doctor"
	else
		sudo=""
		if [ "${pm%% *}" != "brew" ] && [ "$(id -u)" -ne 0 ]; then
			sudo="sudo "
		fi
		if [ -n "$sudo" ]; then
			say "Installing it runs, with sudo (administrator rights): ${sudo}${pm}"
		else
			say "Installing it runs: ${pm}"
		fi
		if confirm "Install tmux now?"; then
			# shellcheck disable=SC2086 # word splitting of the command is intended
			if [ "${pm%% *}" = "apt-get" ]; then ${sudo}apt-get update -q; fi
			# shellcheck disable=SC2086 # word splitting of the command is intended
			${sudo}${pm} || die "tmux installation failed; install it yourself, then run: ajan doctor"
		else
			say "Skipped. Install tmux yourself, then run: ajan doctor"
		fi
	fi
fi

say ""
say "Next: run 'ajan doctor', then 'ajan' in a project directory."
