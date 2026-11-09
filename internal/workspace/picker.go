package workspace

import (
	"context"
	"fmt"
	"regexp"
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
	namedRunes       = 24 // a conversation's name in the list
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

// agentCol is the width of the picker's agent column: the longest short
// name, so the columns line up.
var agentCol = func() int {
	n := 0
	for _, a := range session.AllAgents {
		n = max(n, utf8.RuneCountInString(session.ShortName(a)))
	}
	return n
}()

// pastedRe matches the tags some agents wrap pasted text in.
var pastedRe = regexp.MustCompile(`</?\\?/?pasted_content[^>]*>`)

// PickerMenu builds page `page` (0-based) of the session picker for pane p:
// conversations of every installed agent for @imux_cwd, the pinned ones
// first (§9.3 "Pinned conversations"), then newest first. Only the shown
// rows are read for their preview. It returns nil when there is nothing to
// show.
func (w *WS) PickerMenu(ctx context.Context, f *sources.Finder, p *Pane, client string, page int) *Menu {
	return w.picker(ctx, f, p, client, page, false)
}

// PinMenu is the picker for pinning: choosing a conversation pins or unpins
// it, and the picker opens again.
func (w *WS) PinMenu(ctx context.Context, f *sources.Finder, p *Pane, client string, page int) *Menu {
	return w.picker(ctx, f, p, client, page, true)
}

func (w *WS) picker(ctx context.Context, f *sources.Finder, p *Pane, client string, page int, pinMode bool) *Menu {
	// Agents named in the sidebar name their conversations (names.go).
	w.RememberNames(ctx, f, nil)
	names := w.State().Names
	pinned := w.pinnedRefs(ctx, f, p.CWD)
	isPinned := map[string]bool{}
	for _, r := range pinned {
		isPinned[r.Key()] = true
	}
	refs := pinned
	for _, r := range f.Find(ctx, w.pickerAgents(), session.Query{CWD: p.CWD, Limit: PickerCandidates}) {
		if !isPinned[r.Key()] {
			refs = append(refs, r)
		}
	}
	if len(refs) == 0 {
		return nil
	}
	pages := (len(refs) + PickerRows - 1) / PickerRows
	if page < 0 || page >= pages {
		page = 0
	}
	title := i18n.T("picker.title", paths.Shorten(p.CWD))
	if pinMode {
		title = i18n.T("picker.pin_title", paths.Shorten(p.CWD))
	}
	if pages > 1 {
		title += fmt.Sprintf(" (%d/%d)", page+1, pages)
	}
	m := &Menu{Title: title}
	tgt := uiTarget(p.ID, client)
	now := w.Now()
	rows := refs[page*PickerRows : min((page+1)*PickerRows, len(refs))]
	for i, r := range rows {
		preview := clean.OneLine(pastedRe.ReplaceAllString(f.Preview(ctx, r), " "), previewRunes)
		if strings.TrimSpace(preview) == "" {
			preview = i18n.T("picker.no_preview")
		}
		if n := names[r.Key()]; n != "" {
			// The name first; the preview shortened to keep the width.
			n = clean.OneLine(n, namedRunes)
			preview = n + " · " + clean.OneLine(preview, max(previewRunes-utf8.RuneCountInString(n)+namedRunes/2, 12))
		}
		name := pad(session.ShortName(r.Agent), agentCol) + " " + pad(textfmt.Ago(now, r.UpdatedAt), 12) + " " + preview
		if len(pinned) > 0 || pinMode {
			mark := "   " // the width of "📌 ", so the columns line up
			if isPinned[r.Key()] {
				mark = "📌 "
			}
			name = mark + name
		}
		cmd := w.ui(append([]string{"__menu", "--session", r.Key()}, tgt...)...)
		if pinMode {
			args := []string{"__pin", r.Key()}
			if isPinned[r.Key()] {
				args = append(args, "--unpin")
			}
			cmd = w.ui(append(args, tgt...)...)
		}
		m.Items = append(m.Items, MenuItem{Name: name, Key: strconv.Itoa(i + 1), Cmd: cmd})
	}
	var tail []MenuItem
	if pages > 1 {
		next := (page + 1) % pages
		args := []string{"__menu", "--sessions", "--page", strconv.Itoa(next)}
		if pinMode {
			args = append(args, "--pin")
		}
		tail = append(tail, MenuItem{Name: i18n.T("picker.more"), Key: "m", Cmd: w.ui(append(args, tgt...)...)})
	}
	if !pinMode {
		tail = append(tail, MenuItem{
			Name: i18n.T("picker.pin_mode"), Key: "p",
			Cmd: w.ui(append([]string{"__menu", "--sessions", "--pin", "--page", strconv.Itoa(page)}, tgt...)...),
		})
	}
	if len(tail) > 0 {
		m.Items = append(append(m.Items, Separator()), tail...)
	}
	return m
}

// pinnedRefs looks up the pinned conversations of folder cwd by their ids,
// the newest pin first; ones that are gone or of agents not installed are
// left out.
func (w *WS) pinnedRefs(ctx context.Context, f *sources.Finder, cwd string) []session.Ref {
	var out []session.Ref
	for _, k := range w.State().Pinned {
		a, id, ok := strings.Cut(k, ":")
		if !ok || !session.Known(a) || !w.Installed(session.AgentID(a)) {
			continue
		}
		for _, r := range f.Find(ctx, []session.AgentID{session.AgentID(a)}, session.Query{CWD: cwd, ID: id, Limit: 1}) {
			if r.ID == id {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// SessionMenu is the second-level menu for a picked conversation: resume it
// in its own agent, continue it in another agent with context, preview, and
// the same in a new pane.
func (w *WS) SessionMenu(ref session.Ref, p *Pane, client string) *Menu {
	m := &Menu{Title: i18n.T("picker.session_title", session.ShortName(ref.Agent), textfmt.Ago(w.Now(), ref.UpdatedAt))}
	tgt := uiTarget(p.ID, client)
	key := ref.Key()
	name := w.NameOf(key)
	if name != "" {
		m.Title = name + " · " + m.Title
	}
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
		MenuItem{Name: i18n.T("menu.preview"), Key: "v", Cmd: w.previewCmd(p, key)},
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
	pin, args := i18n.T("picker.pin"), []string{"__pin", key}
	if w.State().IsPinned(key) {
		pin, args = i18n.T("picker.unpin"), append(args, "--unpin")
	}
	m.Items = append(m.Items, Separator(),
		MenuItem{Name: i18n.T("picker.rename"), Key: "r", Cmd: w.NamePrompt(key, name, p.ID, client)},
		MenuItem{Name: pin, Key: "t", Cmd: w.ui(append(args, tgt...)...)})
	return m
}

// tmux options the conversation-name prompt leaves for imux __name-session:
// the conversation, the pane and client to reopen History for, the answer.
const (
	OptNameKey    = "@imux_name_key"
	OptNamePane   = "@imux_name_pane"
	OptNameClient = "@imux_name_client"
	OptNameValue  = "@imux_name_value"
)

// NamePrompt asks on client for conversation key's name (cur: the current
// one). Nothing but the answer goes through the prompt's template, which
// would turn a pane id's %1 into the answer.
func (w *WS) NamePrompt(key, cur, pane, client string) string {
	c := "set-option -g " + OptNameKey + " " + tmux.Quote(key) +
		" ; set-option -g " + OptNamePane + " " + tmux.Quote(pane) +
		" ; set-option -g " + OptNameClient + " " + tmux.Quote(client) + " ; command-prompt"
	if client != "" {
		c += " -t " + tmux.Quote(client)
	}
	tmpl := "set-option -g " + OptNameValue + " \"%%%\" ; " + tmux.RunShell(w.Self, "__name-session", "--from-option")
	return c + " -I " + tmux.Quote(cur) + " -p " + tmux.Quote(i18n.T("prompt.name_session")) + " " + tmux.Quote(tmpl)
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
