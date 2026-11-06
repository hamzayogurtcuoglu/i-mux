package workspace

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/clean"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/textfmt"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Picker limits (§9.3): rows per page and candidate files read.
const (
	PickerRows       = 9
	PickerCandidates = 50
	previewRunes     = 40
)

// pickerAgents are the installed agents in config order.
func (w *WS) pickerAgents() []session.AgentID {
	var ids []session.AgentID
	for _, a := range agent.Ordered(w.Cfg) {
		if w.Installed(a.ID) {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// PickerMenu builds page `page` (0-based) of the session picker for pane p:
// conversations of every installed agent for @ajan_cwd, newest first. Only
// the shown rows are read for their preview. It returns nil when there is
// nothing to show.
func (w *WS) PickerMenu(ctx context.Context, f *sources.Finder, p *Pane, client string, page int) *Menu {
	refs := f.Find(ctx, w.pickerAgents(), session.Query{CWD: p.CWD, Limit: PickerCandidates})
	if len(refs) == 0 {
		return nil
	}
	pages := (len(refs) + PickerRows - 1) / PickerRows
	if page < 0 || page >= pages {
		page = 0
	}
	title := i18n.T("picker.title", paths.Shorten(p.CWD))
	if pages > 1 {
		title += fmt.Sprintf(" (%d/%d)", page+1, pages)
	}
	m := &Menu{Title: title}
	tgt := uiTarget(p.ID, client)
	now := w.Now()
	rows := refs[page*PickerRows : min((page+1)*PickerRows, len(refs))]
	for i, r := range rows {
		preview := clean.OneLine(f.Preview(ctx, r), previewRunes)
		if strings.TrimSpace(preview) == "" {
			preview = i18n.T("picker.no_preview")
		}
		name := pad(session.ShortName(r.Agent), 8) + " " + pad(textfmt.Ago(now, r.UpdatedAt), 12) + " " + preview
		m.Items = append(m.Items, MenuItem{
			Name: name, Key: strconv.Itoa(i + 1),
			Cmd: w.ui(append([]string{"__menu", "--session", r.Key()}, tgt...)...),
		})
	}
	if pages > 1 {
		next := (page + 1) % pages
		m.Items = append(m.Items, Separator(), MenuItem{
			Name: i18n.T("picker.more"), Key: "m",
			Cmd: w.ui(append([]string{"__menu", "--sessions", "--page", strconv.Itoa(next)}, tgt...)...),
		})
	}
	return m
}

// SessionMenu is the second-level menu for a picked conversation: resume it
// in its own agent, continue it in another agent with context, preview, and
// the same in a new pane.
func (w *WS) SessionMenu(ref session.Ref, p *Pane, client string) *Menu {
	m := &Menu{Title: i18n.T("picker.session_title", session.ShortName(ref.Agent), textfmt.Ago(w.Now(), ref.UpdatedAt))}
	tgt := uiTarget(p.ID, client)
	key := ref.Key()
	own := w.Installed(ref.Agent) && w.Cfg.Agent(string(ref.Agent)).Resume && len(w.Cfg.Agent(string(ref.Agent)).ResumeArgs) > 0
	var others []agent.Info
	for _, a := range agent.Ordered(w.Cfg) {
		if a.ID != ref.Agent && w.Installed(a.ID) {
			others = append(others, a)
		}
	}
	if own {
		a := agent.MustGet(ref.Agent)
		m.Items = append(m.Items, MenuItem{
			Name: i18n.T("picker.resume", a.Short), Key: a.Key,
			Cmd: w.ui(append([]string{"switch", string(a.ID), "--from-session", key}, tgt...)...),
		})
	}
	for _, a := range others {
		m.Items = append(m.Items, MenuItem{
			Name: i18n.T("picker.continue", a.Short), Key: a.Key,
			Cmd: w.ui(append([]string{"switch", string(a.ID), "--from-session", key}, tgt...)...),
		})
	}
	m.Items = append(m.Items, Separator(),
		MenuItem{Name: i18n.T("menu.preview"), Key: "v", Cmd: w.previewCmd(p, client, key)},
		Separator())
	if own {
		a := agent.MustGet(ref.Agent)
		m.Items = append(m.Items, MenuItem{
			Name: i18n.T("picker.resume_new_pane", a.Short), Key: strings.ToUpper(a.Key),
			Cmd: w.ui(append([]string{"split", string(a.ID), "-h", "--from-session", key}, tgt...)...),
		})
	}
	for _, a := range others {
		m.Items = append(m.Items, MenuItem{
			Name: i18n.T("picker.continue_new_pane", a.Short), Key: strings.ToUpper(a.Key),
			Cmd: w.ui(append([]string{"split", string(a.ID), "-h", "--from-session", key}, tgt...)...),
		})
	}
	return m
}

// pad right-pads s with spaces to n runes (menus use a monospace font).
func pad(s string, n int) string {
	if k := utf8.RuneCountInString(s); k < n {
		return s + strings.Repeat(" ", n-k)
	}
	return s
}

// ClientFor returns the client to show a menu for pane on: the most recently
// active client attached to the pane's session, else any client.
func (w *WS) ClientFor(ctx context.Context, p *Pane) string {
	out, err := w.T.Run(ctx, tmux.Cmd{"list-clients", "-F", "#{client_name}\t#{client_session}\t#{client_activity}\t#{client_control_mode}"})
	if err != nil {
		return ""
	}
	best, bestAct, any := "", int64(-1), ""
	for _, r := range tmux.Rows(out, 4) {
		if r[3] == "1" {
			continue // control-mode clients cannot show menus
		}
		if any == "" {
			any = r[0]
		}
		act, _ := strconv.ParseInt(r[2], 10, 64)
		if r[1] == p.Session && act > bestAct {
			best, bestAct = r[0], act
		}
	}
	if best != "" {
		return best
	}
	return any
}
