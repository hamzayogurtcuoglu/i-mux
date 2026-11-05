// Command gen-large-fixture writes a synthetic Claude Code session for
// benchmarks (default 64 MB). The output is generated at test time and is
// never committed.
//
//	go run ./scripts/gen-large-fixture -o /tmp/large.jsonl -size 64
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func main() {
	out := flag.String("o", "large.jsonl", "output file")
	sizeMB := flag.Int64("size", 64, "approximate size in MB")
	cwd := flag.String("cwd", "/home/test/proj", "cwd recorded in the session")
	id := flag.String("id", "99999999-0000-4000-8000-000000000064", "session id")
	flag.Parse()
	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := testutil.GenClaudeSession(f, *sizeMB<<20, *cwd, *id); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := f.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
