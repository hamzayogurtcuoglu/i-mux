#!/bin/sh
# Fixture privacy guard (TASKS.md T0.9).
#
# Fails if any testdata/ directory contains:
#   - a macOS home path (/Users/...) or a /home/<user> path other than /home/test;
#   - a real-looking secret (same families as internal/handoff/redact);
#   - an email address outside the reserved example domains.
# Fixtures must be synthetic: never copy, trim or "anonymize" real sessions.
set -eu
cd "$(dirname "$0")/.."

dirs=$(find . -type d -name testdata -not -path './.git/*' -not -path './dist/*' 2>/dev/null || true)
if [ -z "$dirs" ]; then
	echo "check-fixtures: no testdata directories"
	exit 0
fi

fail=0
report() {
	echo "check-fixtures: $1" >&2
	fail=1
}

# shellcheck disable=SC2086 # word splitting of $dirs is intended
scan() {
	grep -rIEn -- "$1" $dirs 2>/dev/null || true
}

hits=$(scan '/Users/')
[ -z "$hits" ] || report "home directory paths (/Users/):
$hits"

# shellcheck disable=SC2086
hits=$(grep -rIEon -- '/home/[A-Za-z0-9_-]+' $dirs 2>/dev/null | grep -Ev ':/home/test$' || true)
[ -z "$hits" ] || report "/home paths other than /home/test:
$hits"

secret_re='-----BEGIN [A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9]{30,}|github_pat_[A-Za-z0-9_]{20,}|sk-ant-[A-Za-z0-9_-]{20,}|sk-(proj-)?[A-Za-z0-9]{20,}|AIza[0-9A-Za-z_-]{35}|xox[abpr]-[A-Za-z0-9-]{10,}|[Bb]earer [A-Za-z0-9._~+/=-]{20,}'
hits=$(scan "$secret_re")
[ -z "$hits" ] || report "real-looking secrets:
$hits"

hits=$(scan '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}' |
	grep -Ev '@(example\.(com|org|net)|test\.invalid)([^A-Za-z0-9.-]|$)' || true)
[ -z "$hits" ] || report "email addresses (use @example.com):
$hits"

if [ "$fail" -ne 0 ]; then
	exit 1
fi
echo "check-fixtures: ok"
