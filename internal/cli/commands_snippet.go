package cli

import (
	"fmt"

	"github.com/hamzayogurtcuoglu/i-mux/internal/bus"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

func init() {
	register(&command{
		Name:    "snippet",
		Args:    "[claude|antigravity|agents]",
		Summary: "cmd.snippet",
		Run:     runSnippet,
	})
}

// runSnippet prints the agent-bus block for the user to paste into
// CLAUDE.md, GEMINI.md or AGENTS.md. It never edits those files.
func runSnippet(e *Env, args []string) error {
	target := "agents"
	if len(args) > 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "snippet [claude|antigravity|agents]")}
	}
	if len(args) == 1 {
		target = args[0]
	}
	s, err := bus.Snippet(target)
	if err != nil {
		return &UsageError{Msg: err.Error()}
	}
	fmt.Fprintln(e.Stderr, i18n.T("snippet.hint", bus.SnippetTargets[target]))
	_, err = fmt.Fprint(e.Stdout, s)
	return err
}
