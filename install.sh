#!/bin/sh
# Install imux: https://github.com/hamzayogurtcuoglu/i-mux
#
#   curl -fsSL https://raw.githubusercontent.com/hamzayogurtcuoglu/i-mux/main/install.sh | sh
#
# Works on macOS, Linux and Windows through WSL. Options (after `sh -s --`
# when piping, e.g. `curl ... | sh -s -- --yes`):
#   --prefix DIR       install directory (default ~/.local/bin; env IMUX_INSTALL_DIR)
#   --version vX.Y.Z   install this release instead of the latest (env IMUX_VERSION)
#   --yes              install tmux without asking if it is missing
#   --no-modify-path   do not add the install directory to your shell profile
#
# Downloads the release archive for this OS and architecture, verifies its
# sha256 against checksums.txt, installs the binary, and puts the install
# directory on PATH in your shell profile. If tmux is missing it offers to
# install it with the system package manager; it asks first and says so
# before it uses sudo.
set -eu

REPO="hamzayogurtcuoglu/i-mux"
PREFIX="${IMUX_INSTALL_DIR:-${HOME}/.local/bin}"
VERSION="${IMUX_VERSION:-}"
YES=0
MODIFY_PATH=1
BASE_URL="${IMUX_INSTALL_BASE_URL:-https://github.com/${REPO}/releases/download}"

say() { printf '%s\n' "$*"; }
die() {
	printf 'imux install: %s\n' "$*" >&2
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
	--no-modify-path)
		MODIFY_PATH=0
		shift
		;;
	-h | --help)
		sed -n '2,17p' "$0" 2>/dev/null | sed 's/^# \{0,1\}//'
		exit 0
		;;
	*) die "unknown option: $1" ;;
	esac
done

have() { command -v "$1" >/dev/null 2>&1; }

fetch() { # fetch URL FILE
	if have curl; then
		curl -fsSL --retry 3 -o "$2" "$1"
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

# latest_tag prints the newest release tag. It follows the redirect of
# /releases/latest, which has no rate limit, and falls back to the API.
latest_tag() {
	url="https://github.com/${REPO}/releases/latest"
	final=""
	if have curl; then
		final=$(curl -fsSLI --retry 3 -o /dev/null -w '%{url_effective}' "$url" 2>/dev/null) || final=""
	elif have wget; then
		final=$(wget -q -S --spider "$url" 2>&1 | awk 'tolower($1) == "location:" { loc = $2 } END { print loc }' | tr -d '\r') || final=""
	fi
	case "$final" in
	*/releases/tag/*)
		printf '%s\n' "${final##*/}"
		return 0
		;;
	esac
	tmpv=$(mktemp)
	if fetch "https://api.github.com/repos/${REPO}/releases/latest" "$tmpv" 2>/dev/null; then
		sed -n 's/.*"tag_name"[^"]*"\([^"]*\)".*/\1/p' "$tmpv" | head -n 1
	fi
	rm -f "$tmpv"
}

# confirm QUESTION: yes with --yes; no when there is no terminal to ask on.
confirm() {
	[ "$YES" -eq 1 ] && return 0
	{ [ -r /dev/tty ] && [ -w /dev/tty ]; } || return 1
	printf '%s [y/N] ' "$1" >/dev/tty
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
MINGW* | MSYS* | CYGWIN*) die "Windows is supported through WSL: open a WSL terminal and run this again" ;;
*) die "unsupported OS: $os (imux runs on macOS and Linux; on Windows use WSL)" ;;
esac
arch=$(uname -m)
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) die "unsupported architecture: $arch" ;;
esac
platform="${os}/${arch}"
if [ "$os" = linux ] && grep -qi microsoft /proc/version 2>/dev/null; then
	platform="${platform} (WSL)"
fi

if [ -z "$VERSION" ]; then
	VERSION=$(latest_tag)
	[ -n "$VERSION" ] || die "could not find the latest release of ${REPO}"
fi
case "$VERSION" in
v*) ;;
*) VERSION="v${VERSION}" ;;
esac
num="${VERSION#v}"
archive="imux_${num}_${os}_${arch}.tar.gz"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM
say "Downloading imux ${VERSION} for ${platform}..."
fetch "${BASE_URL}/${VERSION}/${archive}" "$tmp/$archive" || die "download failed: ${BASE_URL}/${VERSION}/${archive}"
fetch "${BASE_URL}/${VERSION}/checksums.txt" "$tmp/checksums.txt" || die "download failed: checksums.txt"
want=$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || die "$archive is not listed in checksums.txt"
got=$(sha256 "$tmp/$archive")
[ "$want" = "$got" ] || die "checksum mismatch for $archive (expected $want, got $got)"
say "Checksum verified."

tar -xzf "$tmp/$archive" -C "$tmp" imux || die "could not unpack $archive"
mkdir -p "$PREFIX" || die "cannot create $PREFIX"
if ! { cp "$tmp/imux" "$PREFIX/imux.tmp.$$" && chmod 755 "$PREFIX/imux.tmp.$$" && mv -f "$PREFIX/imux.tmp.$$" "$PREFIX/imux"; }; then
	die "cannot install to $PREFIX (use --prefix)"
fi
say "Installed $PREFIX/imux"

# Put PREFIX on PATH for new terminals by adding one line to the profile of
# the user's login shell. A piped script cannot change the PATH of the shell
# that started it, so the closing message says how to use imux right away.
on_path=0
case ":${PATH}:" in
*":${PREFIX}:"*) on_path=1 ;;
esac
path_note=""
if [ "$on_path" -eq 0 ]; then
	shell_name=$(basename "${SHELL:-sh}")
	case "$shell_name" in
	zsh) profile="${ZDOTDIR:-$HOME}/.zshrc" ;;
	bash)
		if [ "$os" = darwin ]; then
			profile="$HOME/.bash_profile"
		else
			profile="$HOME/.bashrc"
		fi
		;;
	fish) profile="${XDG_CONFIG_HOME:-$HOME/.config}/fish/conf.d/imux.fish" ;;
	*) profile="$HOME/.profile" ;;
	esac
	if [ "$shell_name" = fish ]; then
		line="fish_add_path \"$PREFIX\""
	else
		line="export PATH=\"$PREFIX:\$PATH\""
	fi
	if [ "$MODIFY_PATH" -eq 1 ]; then
		if [ -f "$profile" ] && grep -F "$line" "$profile" >/dev/null 2>&1; then
			say "$PREFIX is already on PATH in $profile"
		elif mkdir -p "$(dirname "$profile")" && printf '\n# Added by the imux installer\n%s\n' "$line" >>"$profile"; then
			say "Added $PREFIX to PATH in $profile"
		else
			say "Could not write $profile. Add this line to your shell profile:"
			say "  $line"
		fi
		path_note="$PREFIX/imux"
	else
		say ""
		say "$PREFIX is not on your PATH. Add this to your shell profile:"
		say "  $line"
	fi
fi

# tmux is imux's only runtime dependency.
if have tmux; then
	say "tmux found: $(tmux -V)"
else
	say ""
	say "imux needs tmux, which is not installed."
	pm=""
	if have brew; then
		pm="brew install tmux"
	elif have apt-get; then
		pm="apt-get install -y tmux"
	elif have dnf; then
		pm="dnf install -y tmux"
	elif have pacman; then
		# A fresh Arch has no package database yet, and -Sy alone risks a
		# partial upgrade; -Syu is how Arch installs a package safely.
		pm="pacman -Syu --needed --noconfirm tmux"
	elif have zypper; then
		pm="zypper --non-interactive install tmux"
	elif have apk; then
		pm="apk add tmux"
	fi
	if [ -z "$pm" ]; then
		say "Install tmux with your package manager, then run: imux doctor"
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
			${sudo}${pm} || die "tmux installation failed; install it yourself, then run: imux doctor"
		else
			say "Skipped. Install tmux yourself, then run: imux doctor"
		fi
	fi
fi

say ""
if [ -n "$path_note" ]; then
	say "Done. Open a new terminal, then run 'imux' in a project directory."
	say "(In this terminal you can run it now as: $path_note)"
else
	say "Done. Run 'imux' in a project directory ('imux doctor' checks the setup)."
fi
