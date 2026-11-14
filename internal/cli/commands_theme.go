package cli

import (
	"context"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

func init() {
	register(&command{
		Name:    "__theme",
		Args:    "appearance light|dark",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runTheme,
	})
}

// runTheme notes the terminal's light or dark (tmux's client-light-theme
// and client-dark-theme hooks, a sidebar's OSC 11 query, D68): a theme
// that follows it changes at once.
func runTheme(e *Env, args []string) error {
	fs := newFlags(e, "__theme")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 || pos[0] != "appearance" || pos[1] != "light" && pos[1] != "dark" {
		return &UsageError{Msg: i18n.T("cli.usage", "__theme appearance light|dark")}
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	_, err = w.SetAppearance(context.Background(), pos[1])
	return err
}
