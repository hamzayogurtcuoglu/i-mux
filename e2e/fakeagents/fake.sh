#!/bin/sh
# Fake agent CLI for imux's end-to-end tests. It never touches the network
# and never runs a real agent. The wrappers fake-<agent> set FAKE_AGENT.
#
# Every run n (1, 2, ...) of agent A writes into $IMUX_FAKE_LOG:
#   A-n.argv   arguments, NUL-separated (byte-exact)
#   A-n.env    IMUX_PANE, IMUX_AGENT, TMUX_PANE, PATH and PWD
#   A-n.stdin  lines read from the terminal (paste and bus tests)
#   A-n.session  path of the session file it wrote (if any)
#
# Behavior controls (all optional):
#   $IMUX_FAKE_LOG/A.sh   sourced after start-up (print a limit line, sleep, exit 3, ...)
#   $IMUX_FAKE_LOG/A.nosession   if present, no session file is written
#   FAKE_SCRIPT           evaluated after the control file
set -u
agent="${FAKE_AGENT:?FAKE_AGENT not set}"
log="${IMUX_FAKE_LOG:-${TMPDIR:-/tmp}/imux-fake}"
mkdir -p "$log"

# Version probes (imux's bin cache) and help (imux doctor) are not runs.
if [ "$#" -eq 1 ] && [ "$1" = "--version" ]; then
	echo "$agent ${FAKE_VERSION:-9.9.9}-fake"
	echo "$agent --version" >>"$log/version-probes"
	exit 0
fi
if [ "$#" -eq 1 ] && [ "$1" = "--help" ]; then
	echo "Usage: $agent [options] [prompt]"
	echo "  --session-id <uuid>  -i, --prompt-interactive <prompt>  -r, --resume <id>  resume <id>"
	echo "$agent --help" >>"$log/version-probes"
	exit 0
fi

# Claim the next run number atomically (noclobber create).
n=1
while ! (set -C; : >"$log/$agent-$n.argv") 2>/dev/null; do
	n=$((n + 1))
done
[ $# -eq 0 ] || printf '%s\0' "$@" >"$log/$agent-$n.argv"
{
	echo "IMUX_PANE=${IMUX_PANE:-}"
	echo "IMUX_AGENT=${IMUX_AGENT:-}"
	echo "TMUX_PANE=${TMUX_PANE:-}"
	echo "PATH=$PATH"
	echo "PWD=$PWD"
} >"$log/$agent-$n.env"

now() { date -u +%Y-%m-%dT%H:%M:%SZ; }
uuid() {
	if [ -r /proc/sys/kernel/random/uuid ]; then
		cat /proc/sys/kernel/random/uuid
	elif command -v uuidgen >/dev/null 2>&1; then
		uuidgen | tr '[:upper:]' '[:lower:]'
	else
		od -An -N16 -tx1 /dev/urandom | tr -d ' \n' |
			sed 's/^\(........\)\(....\)\(....\)\(....\)\(............\).*/\1-\2-\3-\4-\5/'
	fi
}

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
# $IMUX_FAKE_LOG/A.handoff-as-request: an i-mux handoff in the last argument
# becomes the first user message, as with a real agent (chained handoffs).
last=""
for a in "$@"; do last="$a"; done
if [ -e "$log/$agent.handoff-as-request" ]; then
	case "$last" in
	"[i-mux handoff"*)
		req=$(printf '%s' "$last" | awk 'BEGIN { ORS = "" } { gsub(/\\/, "\\\\"); gsub(/"/, "\\\""); if (NR > 1) printf "\\n"; print }')
		;;
	esac
fi
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
	antigravity)
		# Like agy: brain/<id>/.system_generated/logs/transcript_full.jsonl,
		# a history.jsonl line naming the directory, and the plan in task.md.
		id=$(opt --conversation "$@") || id=$(uuid)
		root="$HOME/.gemini/antigravity-cli"
		dir="$root/brain/$id"
		mkdir -p "$dir/.system_generated/logs"
		session="$dir/.system_generated/logs/transcript_full.jsonl"
		step=0
		[ -f "$session" ] && step=$(wc -l <"$session" | tr -d ' ')
		printf '{"step_index":%d,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"%s","content":"<USER_REQUEST>\\n%s\\n</USER_REQUEST>\\n<ADDITIONAL_METADATA>\\nThe current local time is: %s.\\n</ADDITIONAL_METADATA>"}\n' "$step" "$ts" "$req" "$ts" >>"$session"
		printf '{"step_index":%d,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"%s","content":"%s","tool_calls":[{"name":"run_command","args":{"CommandLine":"fake test %s","WaitMsBeforeAsync":0}}],"input_tokens":100,"output_tokens":10}\n' "$((step + 1))" "$ts" "$reply" "$n" >>"$session"
		printf '{"step_index":%d,"source":"MODEL","type":"GENERIC","status":"DONE","created_at":"%s","content":"ok"}\n' "$((step + 2))" "$ts" >>"$session"
		printf '{"display":"%s","timestamp":%s000,"workspace":"%s"}\n' "$req" "$(date -u +%s)" "$PWD" >>"$root/history.jsonl"
		printf '# Task\n\n- [x] fake antigravity step done %s\n- [/] fake antigravity step in progress %s\n' "$n" "$n" >"$dir/task.md"
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
echo "argc $# · last argument $(printf %s "$last" | wc -c | tr -d ' ') bytes"

# shellcheck disable=SC1090
[ -f "$log/$agent.sh" ] && . "$log/$agent.sh"
[ -n "${FAKE_SCRIPT:-}" ] && eval "$FAKE_SCRIPT"

while IFS= read -r line; do
	case "$line" in
	"/exit "*) exit "${line#/exit }" ;; # tests end a run with a given status
	esac
	printf '%s\n' "$line" >>"$log/$agent-$n.stdin"
done
exit "${FAKE_EXIT_STATUS:-0}"
