package cli

import (
	"context"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:    "__pane",
		Args:    "swap <left|down|up|right> | resize  --pane %N [--client C]",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runPane,
	})
}

// runPane does the pane keys' work (D67): swap swaps the pane with the one
// on a side (prefix H/J/K/L), resize puts the client in the resize mode
// (the help's Resize panes; the key itself is a tmux binding).
func runPane(e *Env, args []string) error {
	fs := newFlags(e, "__pane")
	paneF := fs.String("pane", "", "the pane the key was pressed in")
	clientF := fs.String("client", "", "the client")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	usage := &UsageError{Msg: i18n.T("cli.usage", "__pane swap <left|down|up|right> | resize")}
	if len(pos) == 0 {
		return usage
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	switch {
	case pos[0] == "swap" && len(pos) == 2 && workspace.ValidDir(pos[1]):
		pane, err := targetPane(*paneF)
		if err != nil {
			return err
		}
		_, err = w.SwapPane(ctx, pane, pos[1])
		return err
	case pos[0] == "resize" && len(pos) == 1:
		return w.ResizeMode(ctx, *clientF)
	}
	return usage
}
