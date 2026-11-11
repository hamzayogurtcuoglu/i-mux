package workspace

import (
	"context"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// QuitMenu is the tab bar's ⏻ Exit (D48): back to the terminal (this
// client detaches, the agents keep running), or quit i-mux after a
// confirm.
func (w *WS) QuitMenu(p *Pane, client string) *Menu {
	detach := "detach-client"
	if client != "" {
		detach += " -t " + tmux.Quote(client)
	}
	return &Menu{Title: i18n.T("quit.title"), Items: []MenuItem{
		{Name: i18n.T("quit.detach"), Key: "b", Cmd: detach},
		Separator(),
		{Name: i18n.T("quit.quit"), Key: "q", Cmd: w.ui(append([]string{"quit", "--menu"}, uiTarget(p.ID, client)...)...)},
	}}
}

// QuitConfirmMenu asks before quitting: every agent stops.
func (w *WS) QuitConfirmMenu(ctx context.Context, pane, client string) *Menu {
	return &Menu{Title: i18n.T("quit.confirm_title", w.AgentCount(ctx)), Items: []MenuItem{
		{Name: i18n.T("quit.confirm"), Key: "y", Cmd: w.ui(append([]string{"quit", "--yes"}, uiTarget(pane, client)...)...)},
		{Name: i18n.T("click.cancel"), Key: "n", Cmd: ""},
	}}
}

// AgentCount is how many agents run, in every project.
func (w *WS) AgentCount(ctx context.Context) int {
	out, err := w.T.Run(ctx, tmux.Cmd{"list-panes", "-a", "-F", "#{@imux_agent}\t#{pane_dead}"})
	if err != nil {
		return 0
	}
	n := 0
	for _, r := range tmux.Rows(out, 2) {
		if strings.TrimSpace(r[0]) != "" && r[1] != "1" {
			n++
		}
	}
	return n
}

// Quit records the open projects (the next imux offers them back, D49),
// runs before (stops what lives outside tmux), then ends the tmux server:
// every agent stops and every client returns to its shell.
func (w *WS) Quit(ctx context.Context, before func()) error {
	_ = w.SaveSnapshot(ctx)
	if before != nil {
		before()
	}
	_, err := w.T.Run(ctx, tmux.Cmd{"kill-server"})
	return err
}
