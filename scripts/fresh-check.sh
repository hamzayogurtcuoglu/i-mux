#!/bin/sh
# Stale checkout guard (D95, TASKS.md T100.2).
#
# make install-local runs this first: it fetches the branch's upstream and
# fails when HEAD is behind it, so ~/.local/bin/imux is never built from a
# checkout that misses releases made from another machine. Offline, it says
# so and goes on. STALE_OK=1 installs anyway.
set -eu
cd "$(dirname "$0")/.."

if [ "${STALE_OK:-}" = 1 ]; then
	exit 0
fi
if ! git rev-parse --git-dir >/dev/null 2>&1; then
	exit 0
fi
up=$(git rev-parse --abbrev-ref --symbolic-full-name '@{u}' 2>/dev/null || true)
if [ -z "$up" ]; then
	echo "fresh-check: $(git rev-parse --abbrev-ref HEAD) has no upstream, not checked"
	exit 0
fi
remote=${up%%/*}
if ! GIT_TERMINAL_PROMPT=0 git fetch --quiet --tags "$remote" 2>/dev/null; then
	echo "fresh-check: could not fetch $remote (offline?), $up as last fetched is used"
fi
behind=$(git rev-list --count "HEAD..$up")
if [ "$behind" -gt 0 ]; then
	echo "fresh-check: HEAD is $behind commit(s) behind $up; this build would be older than what is released." >&2
	echo "fresh-check: run 'git pull --ff-only' first, or STALE_OK=1 make install-local to install it anyway." >&2
	exit 1
fi
