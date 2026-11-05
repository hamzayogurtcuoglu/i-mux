#!/bin/sh
# Runs every fuzz target in the module for $FUZZTIME (default 60s).
set -eu
FUZZTIME="${FUZZTIME:-60s}"
cd "$(dirname "$0")/.."
targets=$(grep -rn --include='*_test.go' '^func Fuzz' internal 2>/dev/null |
	sed -n 's/^\([^:]*\):[0-9]*:func \(Fuzz[A-Za-z0-9_]*\)(.*/\1 \2/p' || true)
if [ -z "$targets" ]; then
	echo "no fuzz targets found"
	exit 0
fi
echo "$targets" | while read -r file fn; do
	dir="./$(dirname "$file")"
	echo "==> $dir $fn ($FUZZTIME)"
	go test -run '^$' -fuzz "^${fn}\$" -fuzztime "$FUZZTIME" "$dir"
done
