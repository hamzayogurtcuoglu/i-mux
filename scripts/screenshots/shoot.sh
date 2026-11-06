#!/bin/sh
# Regenerates the README screenshots in docs/images from a real imux session
# with demo agents (demo-agent.sh). Needs Linux or macOS, tmux >= 3.4 and
# python3. Nothing outside a temp dir is touched; no real agent is started.
#
#   scripts/screenshots/shoot.sh [out-dir]
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
out="${1:-$root/docs/images}"
bin="${IMUX_BIN:-$root/bin/imux}"
[ -x "$bin" ] || (cd "$root" && make build >/dev/null)
COLS=104 ROWS=30
# shellcheck disable=SC2088 # a literal ~: it is the window title
title="~/code/acme-api — imux"

tmp=$(mktemp -d "${TMPDIR:-/tmp}/imux-shot.XXXXXX")
outer="imux-shot-outer-$$"
export HOME="$tmp/home" IMUX_HOME="$tmp/home/.imux" IMUX_TMUX_SOCKET="imux-shot-$$"
export TZ=UTC LANG=C.UTF-8 LC_ALL=C.UTF-8 TERM=xterm-256color IMUX_LANG=en
cleanup() {
	tmux -L "$outer" kill-server 2>/dev/null || true
	tmux -L "$IMUX_TMUX_SOCKET" kill-server 2>/dev/null || true
	"$bin" __watch stop >/dev/null 2>&1 || true
	rm -rf "$tmp"
}
trap cleanup EXIT INT TERM

# Demo agents and config.
mkdir -p "$IMUX_HOME" "$tmp/bin" "$out"
for a in claude agy codex; do
	cp "$here/demo-agent.sh" "$tmp/bin/$a" # its name is the agent, and the process name
	chmod +x "$tmp/bin/$a"
done
cat >"$IMUX_HOME/config.toml" <<EOF
[limits]
poll_interval = "0.5s"
grace = "1s"
[agents.claude]
bin = "$tmp/bin/claude"
[agents.antigravity]
bin = "$tmp/bin/agy"
[agents.codex]
bin = "$tmp/bin/codex"
[agents.copilot]
bin = "/nonexistent/copilot"
EOF

# A small project with uncommitted work, for the handoff's repository state.
proj="$HOME/code/acme-api"
mkdir -p "$proj/src/auth" "$proj/test/auth"
(
	cd "$proj"
	git init -q -b main
	printf 'export function redirectTarget(next?: string) {\n  return "/home"\n}\n' >src/auth/redirect.ts
	printf 'export const oauth = {}\n' >src/auth/oauth.ts
	git add -A && git -c user.name=demo -c user.email=demo@example.com commit -q -m "Initial commit"
	printf 'export function redirectTarget(next?: string) {\n  return safeNext(next) ?? "/home"\n}\n' >src/auth/redirect.ts
	printf 'test("keeps next", () => {})\n' >test/auth/redirect.test.ts
)

cap() { tmux -L "$outer" capture-pane -p -e -N -t shot; }
waitfor() { # waitfor TEXT
	i=0
	until cap | grep -qF -- "$1"; do
		i=$((i + 1))
		[ "$i" -lt 100 ] || { echo "timed out waiting for: $1" >&2 && cap >&2 && exit 1; }
		sleep 0.2
	done
	sleep 0.6 # let the screen settle
}
shot() { # shot NAME TITLE
	cap >"$tmp/$1.ans"
	[ -z "${SHOT_RAW:-}" ] || cp "$tmp/$1.ans" "$out/$1.ans"
	python3 "$here/ansi2svg.py" "$COLS" "$ROWS" "$2" <"$tmp/$1.ans" >"$out/$1.svg"
	echo "wrote $out/$1.svg"
}

# The terminal: an outer tmux without a status line, running imux.
tmux -L "$outer" -f /dev/null new-session -d -s shot -x "$COLS" -y "$ROWS" -c "$proj" \
	"env -u TMUX -u TMUX_PANE DEMO_LIMIT_AFTER=4 '$bin' claude"
tmux -L "$outer" set -g status off
tmux -L "$outer" set -g default-terminal tmux-256color 2>/dev/null || true

# 1. Claude hits its limit; the watcher asks where to continue.
waitfor "hit its limit"
shot limit "$title"

# 2. Press a: the pane continues in Antigravity, which gets the handoff.
tmux -L "$outer" send-keys -t shot a
waitfor "Picking up from Claude"
shot switched "$title"

# 3. The pane menu (Ctrl+\).
tmux -L "$outer" send-keys -t shot "C-\\"
waitfor "Preview handoff"
shot menu "$title"
tmux -L "$outer" send-keys -t shot Escape
