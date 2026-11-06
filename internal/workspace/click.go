package workspace

import (
	"context"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
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
	}
	if !session.Known(x) {
		return ClickNone, ""
	}
	id := session.AgentID(x)
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

// Click handles `ajan __click <range>` for pane p on client.
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
	case ClickInstall:
		a := agent.MustGet(id)
		w.Message(ctx, client, p.ID, i18n.T("agent.not_installed", a.Name, a.InstallHint))
		return nil
	case ClickConfirm:
		return w.Show(ctx, w.ConfirmMenu(id, p, client), client, p.ID)
	case ClickSwitch:
		_, err := w.Switch(ctx, f, SwitchReq{Pane: p.ID, To: id, Reason: "manual", Client: client})
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
