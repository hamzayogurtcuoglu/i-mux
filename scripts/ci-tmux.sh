#!/bin/sh
# Builds the tmux the e2e suite runs against in CI, pinned to one version
# (TASKS.md T91.2): the distribution's tmux changes under us — Ubuntu 26.04's
# 3.6 closes menus by itself (docs/TROUBLESHOOTING.md) — so the suite must not
# depend on it. tmux is built from the release tarball, checked against its
# SHA-256, into ~/.cache/tmux-<version> (kept warm by the Actions cache) and
# put on PATH for the steps that follow.
#
#   scripts/ci-tmux.sh    (Linux: needs gcc, make, libevent-dev,
#                          libncurses-dev, bison, pkg-config)
set -eu

version=3.8
sha256=e79c699c7e949dccd0a4a125e17b8d1261e311b16979dbc8eb34542f3966d82e
url="https://github.com/tmux/tmux/releases/download/$version/tmux-$version.tar.gz"
prefix="$HOME/.cache/tmux-$version"

if [ -x "$prefix/bin/tmux" ] && "$prefix/bin/tmux" -V | grep -qx "tmux $version"; then
	echo "tmux $version (cached)"
else
	rm -rf "$prefix"
	mkdir -p "$prefix/src"
	curl -fsSLo "$prefix/src/tmux.tar.gz" "$url"
	echo "$sha256  $prefix/src/tmux.tar.gz" | sha256sum -c -
	tar -xzf "$prefix/src/tmux.tar.gz" -C "$prefix/src"
	(
		cd "$prefix/src/tmux-$version"
		./configure --prefix="$prefix" >/dev/null
		make -j"$(nproc)" >/dev/null
		make install >/dev/null
	)
	rm -rf "$prefix/src"
fi

if [ -n "${GITHUB_PATH:-}" ]; then
	echo "$prefix/bin" >>"$GITHUB_PATH"
fi
