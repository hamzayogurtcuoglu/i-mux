// Command imux is a lightweight tmux workspace that moves a conversation
// between AI coding CLIs (Claude Code, Antigravity CLI, Copilot CLI, Codex CLI, OpenCode).
//
// It carries its engine, i-mux-core (D88): run under the name imux-core (a
// link or copy of it), it is the engine, so `go install …/cmd/imux` alone
// gives a working i-mux.
package main

import (
	"os"

	"github.com/hamzayogurtcuoglu/i-mux-core/engine"
	"github.com/hamzayogurtcuoglu/i-mux/internal/cli"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func main() {
	if tmux.RunsAsEngine(os.Args[0]) {
		os.Exit(engine.Main(os.Args[1:], os.Stdout, os.Stderr))
	}
	tmux.CarriesEngine = true
	os.Exit(cli.Main(os.Args))
}
