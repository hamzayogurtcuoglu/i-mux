// Command ajan is a lightweight tmux workspace that moves a conversation
// between AI coding CLIs (Claude Code, Gemini CLI, Copilot CLI, Codex CLI).
package main

import (
	"os"

	"github.com/hamzayogurtcuoglu/i-mux/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args))
}
