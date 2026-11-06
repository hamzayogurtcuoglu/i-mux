#!/bin/sh
# Measures the limit watcher's footprint (ARCHITECTURE §17, TASKS T3.9).
#
#   scripts/measure-watch.sh [seconds per phase]   (default 600)
#
# Runs an isolated workspace (own HOME, AJAN_HOME and tmux socket) with four
# fake-agent panes, two of which redraw a spinner constantly, like real
# agents. Phase 1 has a client attached, phase 2 has none. Every INTERVAL
# seconds (default 5) it samples the watcher's RSS; CPU comes from the
# process's CPU time over the phase. It also counts the watcher's tmux execs
# from the command recorder. Nothing touches your real ~/.ajan or agents.
set -eu
cd "$(dirname "$0")/.."
DUR="${1:-600}"
INT="${INTERVAL:-5}"
BIN="${AJAN_BIN:-$PWD/bin/ajan}"
[ -x "$BIN" ] || make build >/dev/null
command -v tmux >/dev/null || { echo "tmux is not installed" >&2; exit 1; }

tmp=$(mktemp -d "${TMPDIR:-/tmp}/ajan-perf.XXXXXX")
SOCK="ajan-perf-$$"
cleanup() {
	tmux -L "$SOCK" kill-server 2>/dev/null || true
	[ -n "${client:-}" ] && kill "$client" 2>/dev/null || true
	"$BIN" __watch stop >/dev/null 2>&1 || true
	rm -rf "$tmp"
}
trap cleanup EXIT INT TERM

export HOME="$tmp/home" AJAN_HOME="$tmp/home/.ajan" AJAN_TMUX_SOCKET="$SOCK"
export CLAUDE_CONFIG_DIR="$HOME/.claude" CODEX_HOME="$HOME/.codex" COPILOT_HOME="$HOME/.copilot"
export AJAN_FAKE_LOG="$tmp/log" AJAN_TMUX_RECORD="$tmp/record.jsonl" TMUX= TMUX_PANE=
mkdir -p "$HOME/proj" "$AJAN_HOME" "$AJAN_FAKE_LOG" "$tmp/fake"
cp e2e/fakeagents/* "$tmp/fake/"
chmod +x "$tmp/fake/"*
{
	for a in claude gemini copilot codex; do
		printf '[agents.%s]\nbin = "%s/fake/fake-%s"\n\n' "$a" "$tmp" "$a"
	done
} >"$AJAN_HOME/config.toml"
# Claude and Gemini redraw a spinner line five times a second.
spin='while :; do for c in a b c d; do printf "\r%s working %s" "$c" "$(date +%S)"; sleep 0.2; done; done'
echo "$spin" >"$AJAN_FAKE_LOG/claude.sh"
echo "$spin" >"$AJAN_FAKE_LOG/gemini.sh"

cd "$HOME/proj"
"$BIN" claude -d >/dev/null
p0=$(tmux -L "$SOCK" list-panes -a -F '#{pane_id}' | head -n 1)
"$BIN" split gemini -h --pane "$p0" >/dev/null
"$BIN" split codex -v --pane "$p0" >/dev/null
"$BIN" split copilot -v --pane "$p0" >/dev/null

# An attached client on a pseudo-terminal (util-linux or BSD script).
if script --version >/dev/null 2>&1; then
	(sleep $((DUR + 120)) | script -qfc "tmux -u -L $SOCK attach" /dev/null >/dev/null 2>&1) &
else
	(sleep $((DUR + 120)) | script -q /dev/null tmux -u -L "$SOCK" attach >/dev/null 2>&1) &
fi
client=$!
i=0
until tmux -L "$SOCK" list-clients -F x 2>/dev/null | grep -q x; do
	i=$((i + 1))
	[ "$i" -lt 100 ] || { echo "client did not attach" >&2; exit 1; }
	sleep 0.1
done
"$BIN" __watch ensure
sleep 1
pid=$(cat "$AJAN_HOME/watch.pid")
[ -n "$pid" ] && kill -0 "$pid" || { echo "watcher not running" >&2; exit 1; }

hz=$(getconf CLK_TCK 2>/dev/null || echo 100)
cputicks() { # utime+stime in clock ticks (Linux), or ps cputime in seconds*hz
	if [ -r "/proc/$1/stat" ]; then
		sed 's/^.*) //' "/proc/$1/stat" | awk '{print $12 + $13}'
	else
		ps -o time= -p "$1" 2>/dev/null | awk -v hz="$hz" '{n=split($1,a,":"); s=0; for(i=1;i<=n;i++) s=s*60+a[i]; print s*hz}'
	fi
}
execs() { grep -c "\"pid\":$pid," "$AJAN_TMUX_RECORD" 2>/dev/null || echo 0; }

phase() { # phase NAME
	maxrss=0
	c0=$(cputicks "$pid")
	e0=$(execs)
	t0=$(date +%s)
	end=$((t0 + DUR))
	while [ "$(date +%s)" -lt "$end" ]; do
		rss=$(ps -o rss= -p "$pid" 2>/dev/null | tr -d ' ')
		[ "${rss:-0}" -gt "$maxrss" ] && maxrss=$rss
		sleep "$INT"
	done
	c1=$(cputicks "$pid")
	e1=$(execs)
	t1=$(date +%s)
	awk -v n="$1" -v r="$maxrss" -v c="$((c1 - c0))" -v hz="$hz" -v w="$((t1 - t0))" -v e="$((e1 - e0))" 'BEGIN {
		printf "%-9s max RSS %.1f MB  mean CPU %.3f%%  tmux execs %d (%.2f/s)  over %ds\n", n, r/1024, 100*c/hz/w, e, e/w, w }'
}

echo "ajan watcher: $DUR s per phase, sample every $INT s, $(tmux -V), $(uname -sm)"
phase attached
for c in $(tmux -L "$SOCK" list-clients -F '#{client_name}'); do
	tmux -L "$SOCK" detach-client -t "$c"
done
kill "$client" 2>/dev/null || true
client=
sleep 2
phase detached
