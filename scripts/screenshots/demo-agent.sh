#!/bin/sh
# Demo agent CLI for README screenshots (scripts/screenshots/shoot.sh). It is
# not a real agent: "claude" prints a short canned session, writes a matching
# session file (so imux builds a real handoff from it) and then reports a usage
# limit; the others print the start of the prompt imux gave them. shoot.sh
# copies it to <dir>/claude, <dir>/agy, ...; the agent is the file name.
set -u
agent="${DEMO_AGENT:-$(basename "$0")}"

case "${1:-}" in
--version) echo "$agent 1.0.0-demo" && exit 0 ;;
--help) echo "Usage: $agent [--session-id <id>] [-i <prompt>] [prompt]" && exit 0 ;;
esac

dim() { printf '\033[2m%s\033[0m\n' "$*"; }
bold() { printf '\033[1m%s\033[0m\n' "$*"; }
step() { printf '\033[36m●\033[0m %s\n' "$*"; }

opt() { # opt NAME ARGS... → value after NAME
	want="$1"
	shift
	while [ $# -gt 0 ]; do
		[ "$1" = "$want" ] && [ $# -gt 1 ] && printf '%s' "$2" && return 0
		shift
	done
	return 1
}

claude() {
	id=$(opt --session-id "$@") || id=00000000-0000-4000-8000-000000000001
	dir="$HOME/.claude/projects/$(printf %s "$PWD" | sed 's/[^A-Za-z0-9]/-/g')"
	mkdir -p "$dir"
	ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
	req="The login page ignores ?next= after sign-in and always lands on /home. Fix it and add a test."
	u() { printf '{"type":"user","sessionId":"%s","cwd":"%s","timestamp":"%s","message":{"role":"user","content":%s}}\n' "$id" "$PWD" "$ts" "$1"; }
	a() { printf '{"type":"assistant","sessionId":"%s","cwd":"%s","timestamp":"%s","message":{"id":"%s","role":"assistant","model":"demo","content":[%s]}}\n' "$id" "$PWD" "$ts" "$1" "$2"; }
	tool() { printf '{"type":"tool_use","id":"%s","name":"%s","input":%s}' "$1" "$2" "$3"; }
	ok() { u "[{\"type\":\"tool_result\",\"tool_use_id\":\"$1\",\"content\":\"$2\"}]"; }
	{
		u "\"$req\""
		a m1 "$(tool t1 TodoWrite '{"todos":[{"content":"Keep ?next= through sign-in","status":"completed"},{"content":"Add redirect tests","status":"completed"},{"content":"Keep ?next= across the OAuth callback","status":"in_progress"}]}')"
		a m2 "$(tool t2 Read '{"file_path":"src/auth/redirect.ts"}')"
		ok t2 "..."
		a m3 "$(tool t3 Edit '{"file_path":"src/auth/redirect.ts","old_string":"return \"/home\"","new_string":"return safeNext(next) ?? \"/home\""}')"
		ok t3 "ok"
		a m4 "$(tool t4 Write '{"file_path":"test/auth/redirect.test.ts","content":"..."}')"
		ok t4 "ok"
		a m5 "$(tool t5 Bash '{"command":"npm test -- auth"}')"
		ok t5 "14 passed, 1 failed: keeps next across the OAuth callback"
		a m6 '{"type":"text","text":"The redirect fix works for password sign-in. The OAuth callback still drops next: it is not stored in the state parameter. Fixing src/auth/oauth.ts next."}'
	} >"$dir/$id.jsonl"

	bold "> $req"
	echo
	step "Read src/auth/redirect.ts"
	step "Update src/auth/redirect.ts  (+14 −3)"
	step "Write test/auth/redirect.test.ts"
	step "Bash  npm test -- auth"
	dim "    14 passed, 1 failed: keeps next across the OAuth callback"
	echo
	echo "The redirect fix works for password sign-in. The OAuth callback"
	echo "still drops next: it is not stored in the state parameter."
	echo
	sleep "${DEMO_LIMIT_AFTER:-3}"
	echo "5-hour limit reached ∙ resets 3pm"
}

other() {
	last=""
	for x in "$@"; do last="$x"; done
	if [ -n "$last" ]; then
		dim "> $(printf '%s\n' "$last" | head -n 1)"
		printf '%s\n' "$last" | sed -n '2,14p' | fold -s -w 92 | head -n 14 | while IFS= read -r l; do dim "  $l"; done
		dim "  … ($(printf %s "$last" | wc -c | tr -d ' ') bytes)"
		echo
	fi
	printf '\033[35m✦\033[0m %s\n' "Picking up from Claude: the redirect fix is in and one OAuth test fails."
	step "Read src/auth/oauth.ts"
}

case "$agent" in
claude) claude "$@" ;;
*) other "$@" ;;
esac
while IFS= read -r _; do :; done
