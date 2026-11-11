// Command imux is a lightweight tmux workspace that moves a conversation
// between AI coding CLIs (Claude Code, Antigravity CLI, Copilot CLI, Codex CLI, OpenCode).
package main

import (
	"os"

	"github.com/hamzayogurtcuoglu/i-mux/internal/cli"
	"github.com/hamzayogurtcuoglu/i-mux/internal/pwshshim"
)

func main() {
	if code, ok := pwshshim.Run(os.Args); ok {
		os.Exit(code)
	}
	os.Exit(cli.Main(os.Args))
}
