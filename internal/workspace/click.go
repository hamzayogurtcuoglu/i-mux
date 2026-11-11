package workspace

import (
	"context"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
)

// ClickAction says what a click on an agent-bar region does.
type ClickAction int

const (
	ClickNone     ClickAction = iota
	ClickMenu                 // open the pane menu
	ClickSessions             // open the session picker
	ClickSwitch               // switch the pane to the agent
	ClickConfirm              // the agent is limited: ask first
	ClickInstall              // the agent is not installed: show the hint
	ClickNewTab               // the tab bar's +: ask which agent a new tab starts with
	ClickCloseTab             // the × of the current tab: ask, then close it
	ClickQuit                 // the tab bar's ⏻ Exit: back to the terminal, or quit (D48)
)

// Classify decides what a click on range rng does for pane p (§9.3, §14.7).
func (w *WS) Classify(rng string, p *Pane) (ClickAction, session.AgentID) {
	x, ok := strings.CutPrefix(rng, RangePrefix)
	if !ok {
		return ClickNone, ""
	}
	switch x {
	case "cur", "menu":
		return ClickMenu, ""
	case "sessions":
		return ClickSessions, ""
	case "tab":
		return ClickNewTab, ""
	case "close":
		return ClickCloseTab, ""
	case "quit":
		return ClickQuit, ""
	}
	id, ok := AgentOfRange(x)
	if !ok {
		return ClickNone, ""
	}
	cur := p.Agent
	if p.Dead {
		cur = ""
	}
	switch {
	case id == cur:
		return ClickMenu, id
	case !w.Installed(id):
		return ClickInstall, id
	case w.LimitedUntil(id) != "":
		return ClickConfirm, id
	}
	return ClickSwitch, id
}

// Click handles `imux __click <range>` for pane p on client.
func (w *WS) Click(ctx context.Context, f *sources.Finder, rng string, p *Pane, client string) error {
	action, id := w.Classify(rng, p)
	switch action {
	case ClickMenu:
		return w.Show(ctx, w.PaneMenu(p, client), client, p.ID)
	case ClickSessions:
		m := w.PickerMenu(ctx, f, p, client, 0)
		if m == nil {
			w.Message(ctx, client, p.ID, i18n.T("picker.none", paths.Shorten(p.CWD)))
			return nil
		}
		return w.Show(ctx, m, client, p.ID)
	case ClickNewTab:
		return w.Show(ctx, w.TabMenu(p, client), client, p.ID)
	case ClickCloseTab:
		m, err := w.CloseTabMenu(ctx, p, client)
		if err != nil {
			return err
		}
		return w.Show(ctx, m, client, p.ID)
	case ClickQuit:
		return w.Show(ctx, w.QuitMenu(p, client), client, p.ID)
	case ClickInstall:
		return w.ShowInstall(id, p.ID, client)
	case ClickConfirm:
		return w.Show(ctx, w.ConfirmMenu(id, p, client), client, p.ID)
	case ClickSwitch:
		// No forced reason: a source that just failed with a limit is
		// reported as such (T3.7).
		_, err := w.Switch(ctx, f, SwitchReq{Pane: p.ID, To: id, Client: client})
		if err != nil {
			w.Message(ctx, client, p.ID, SwitchError(id, err))
		}
		return err
	}
	return nil
}

// ConfirmMenu asks before switching to a limited agent.
func (w *WS) ConfirmMenu(id session.AgentID, p *Pane, client string) *Menu {
	name := session.ShortName(id)
	return &Menu{
		Title: i18n.T("click.limited_title", name, w.LimitedUntil(id)),
		Items: []MenuItem{
			{Name: i18n.T("click.switch_anyway", name), Key: "y", Cmd: w.ui(append([]string{"switch", string(id)}, uiTarget(p.ID, client)...)...)},
			{Name: i18n.T("click.cancel"), Key: "n", Cmd: ""},
		},
	}
}
