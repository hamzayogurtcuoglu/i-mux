#!/bin/sh
# Fake agent CLI for ajan's end-to-end tests. It never touches the network
# and never runs a real agent. The wrappers fake-<agent> set FAKE_AGENT.
#
# Every run n (1, 2, ...) of agent A writes into $AJAN_FAKE_LOG:
#   A-n.argv   arguments, NUL-separated (byte-exact)
#   A-n.env    AJAN_PANE, AJAN_AGENT, TMUX_PANE, PATH and PWD
#   A-n.stdin  lines read from the terminal (paste and bus tests)
#   A-n.session  path of the session file it wrote (if any)
#
# Behavior controls (all optional):
#   $AJAN_FAKE_LOG/A.sh   sourced after start-up (print a limit line, sleep, exit 3, ...)
#   $AJAN_FAKE_LOG/A.nosession   if present, no session file is written
#   FAKE_SCRIPT           evaluated after the control file
set -u
agent="${FAKE_AGENT:?FAKE_AGENT not set}"
log="${AJAN_FAKE_LOG:-${TMPDIR:-/tmp}/ajan-fake}"
mkdir -p "$log"

# Version probes (ajan's bin cache) are not runs.
if [ "$#" -eq 1 ] && [ "$1" = "--version" ]; then
	echo "$agent 0.0.0-fake"
	echo "$agent --version" >>"$log/version-probes"
	exit 0
fi

# Claim the next run number atomically (noclobber create).
n=1
while ! (set -C; : >"$log/$agent-$n.argv") 2>/dev/null; do
	n=$((n + 1))
done
printf '%s\0' "$@" >"$log/$agent-$n.argv"
{
	echo "AJAN_PANE=${AJAN_PANE:-}"
	echo "AJAN_AGENT=${AJAN_AGENT:-}"
	echo "TMUX_PANE=${TMUX_PANE:-}"
	echo "PATH=$PATH"
	echo "PWD=$PWD"
} >"$log/$agent-$n.env"

now() { date -u +%Y-%m-%dT%H:%M:%SZ; }
uuid() {
	if [ -r /proc/sys/kernel/random/uuid ]; then
		cat /proc/sys/kernel/random/uuid
	elif command -v uuidgen >/dev/null 2>&1; then
		uuidgen | tr 'A-Z' 'a-z'
	else
		od -An -N16 -tx1 /dev/urandom | tr -d ' \n' |
			sed 's/^\(........\)\(....\)\(....\)\(....\)\(............\).*/\1-\2-\3-\4-\5/'
	fi
}
sha256() { { sha256sum 2>/dev/null || shasum -a 256; } | cut -c1-64; }

# Option values from argv.
opt() { # opt NAME ARGS... → value after NAME
	want="$1"
	shift
	while [ $# -gt 0 ]; do
		if [ "$1" = "$want" ] && [ $# -gt 1 ]; then
			printf '%s' "$2"
			return 0
		fi
		shift
	done
	return 1
}

pane="${TMUX_PANE:-none}"
req="fake $agent request $n from pane $pane"
reply="fake $agent reply $n"
ts=$(now)
session=""

write_session() {
	[ -e "$log/$agent.nosession" ] && return 0
	case "$agent" in
	claude)
		id=$(opt --session-id "$@") || id=$(opt --resume "$@") || id=$(uuid)
		dir="${CLAUDE_CONFIG_DIR:-$HOME/.claude}/projects/$(printf %s "$PWD" | sed 's/[^A-Za-z0-9]/-/g')"
		mkdir -p "$dir"
		session="$dir/$id.jsonl"
		cat >>"$session" <<EOF
{"type":"user","sessionId":"$id","cwd":"$PWD","timestamp":"$ts","uuid":"u$n","message":{"role":"user","content":"$req"}}
{"type":"assistant","sessionId":"$id","cwd":"$PWD","timestamp":"$ts","uuid":"a$n","message":{"id":"msg-$n-$id","role":"assistant","model":"fake-model","content":[{"type":"text","text":"$reply"}]}}
EOF
		;;
	codex)
		if [ "${1:-}" = "resume" ] && [ $# -gt 1 ]; then
			id="$2"
			session=$(find "${CODEX_HOME:-$HOME/.codex}/sessions" -name "rollout-*-$id.jsonl" 2>/dev/null | head -n 1)
		else
			id=$(uuid)
		fi
		if [ -z "$session" ]; then
			day=$(date -u +%Y/%m/%d)
			dir="${CODEX_HOME:-$HOME/.codex}/sessions/$day"
			mkdir -p "$dir"
			session="$dir/rollout-$(date -u +%Y-%m-%dT%H-%M-%S)-$id.jsonl"
			echo "{\"timestamp\":\"$ts\",\"type\":\"session_meta\",\"payload\":{\"id\":\"$id\",\"cwd\":\"$PWD\"}}" >>"$session"
		fi
		cat >>"$session" <<EOF
{"timestamp":"$ts","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"$req"}]}}
{"timestamp":"$ts","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"$reply"}]}}
EOF
		;;
	gemini)
		id=$(opt --resume "$@") || id=$(uuid)
		hash=$(printf %s "$PWD" | sha256)
		dir="$HOME/.gemini/tmp/$hash/chats"
		mkdir -p "$dir"
		session=$(grep -l "\"sessionId\":\"$id\"" "$dir"/session-*.json 2>/dev/null | head -n 1)
		[ -n "$session" ] || session="$dir/session-$(date -u +%Y-%m-%dT%H-%M)-$(printf %s "$id" | cut -c1-8).json"
		# Earlier messages of a resumed chat are kept in a side file.
		msgs="$log/gemini-$id.msgs"
		[ -s "$msgs" ] && printf ',' >>"$msgs"
		printf '{"id":"u%s","timestamp":"%s","type":"user","content":"%s"},{"id":"g%s","timestamp":"%s","type":"gemini","content":"%s"}' \
			"$n" "$ts" "$req" "$n" "$ts" "$reply" >>"$msgs"
		printf '{"sessionId":"%s","projectHash":"%s","startTime":"%s","lastUpdated":"%s","messages":[%s]}\n' \
			"$id" "$hash" "$ts" "$ts" "$(cat "$msgs")" >"$session.tmp"
		mv "$session.tmp" "$session"
		;;
	copilot)
		id=$(opt --resume "$@") || id=$(uuid)
		dir="${COPILOT_HOME:-$HOME/.copilot}/session-state/$id"
		mkdir -p "$dir"
		session="$dir/events.jsonl"
		[ -e "$dir/workspace.yaml" ] || printf 'id: %s\ncwd: %s\ncreated_at: %s\n' "$id" "$PWD" "$ts" >"$dir/workspace.yaml"
		cat >>"$session" <<EOF
{"type":"user.message","timestamp":"$ts","data":{"content":"$req"}}
{"type":"assistant.message","timestamp":"$ts","data":{"content":"$reply"}}
EOF
		;;
	esac
	[ -n "$session" ] && printf '%s\n' "$session" >"$log/$agent-$n.session"
	return 0
}
write_session "$@"

echo "fake $agent ready (run $n, pane $pane)"
last=""
for a in "$@"; do last="$a"; done
echo "argc $# · last argument $(printf %s "$last" | wc -c | tr -d ' ') bytes"

# shellcheck disable=SC1090
[ -f "$log/$agent.sh" ] && . "$log/$agent.sh"
[ -n "${FAKE_SCRIPT:-}" ] && eval "$FAKE_SCRIPT"

while IFS= read -r line; do
	printf '%s\n' "$line" >>"$log/$agent-$n.stdin"
done
exit "${FAKE_EXIT_STATUS:-0}"
