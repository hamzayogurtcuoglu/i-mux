package cli

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/popmenu"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func init() {
	register(&command{
		Name:   "__menu-popup",
		Args:   "<id>",
		Hidden: true,
		Run:    runMenuPopup,
	})
}

// runMenuPopup is a menu in a popup (D54): it shows ~/.imux/menus/<id>.json
// and writes the chosen command to <id>.tmux, which tmux sources once the
// popup has closed.
func runMenuPopup(e *Env, args []string) error {
	if len(args) != 1 || !popmenu.ValidID(args[0]) {
		return &UsageError{Msg: "__menu-popup <id>"}
	}
	menuPath, choicePath := popmenu.Files(args[0])
	b, err := os.ReadFile(menuPath)
	_ = os.Remove(menuPath)
	if err != nil {
		return err
	}
	var m popmenu.Menu
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	i, err := popmenu.Run(&m, os.Stdin, os.Stdout)
	if err != nil || i < 0 || m.Items[i].Cmd == "" {
		return err
	}
	cmd := expandMenuCmd(context.Background(), tmux.New(paths.TmuxConf()), &m, m.Items[i].Cmd)
	return os.WriteFile(choicePath, []byte(cmd+"\n"), 0o600)
}

// expandMenuCmd expands a menu command's formats as display-menu does when it
// creates the menu, for the menu's pane and client (tmux 3.2 picks the
// client). display-message also runs strftime, so "%" is doubled first.
func expandMenuCmd(ctx context.Context, t *tmux.Client, m *popmenu.Menu, cmd string) string {
	c := tmux.Cmd{"display-message", "-p"}
	if m.Client != "" && t.MessageTakesClient(ctx) {
		c = append(c, "-c", m.Client)
	}
	if m.Pane != "" {
		c = append(c, "-t", m.Pane)
	}
	out, err := t.Run(ctx, append(c, strings.ReplaceAll(cmd, "%", "%%")))
	if err != nil {
		return cmd
	}
	return strings.TrimSuffix(out, "\n")
}
