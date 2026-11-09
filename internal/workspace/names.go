package workspace

import (
	"context"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Conversation names (§9.3 "Named conversations"): an agent's name in the
// sidebar (@imux_name) is also its conversation's, so History shows it; a
// name given in History is the agent's again when the conversation comes
// back. Names live in state.json by conversation key ("agent:id").

// RememberNames gives each named pane's conversations its name: every run
// of the pane's trail, as the name stays when the chat moves to another
// CLI. only, when not nil, limits it to some panes. A conversation that has
// no file yet is named the next time (History calls it when it opens).
func (w *WS) RememberNames(ctx context.Context, f *sources.Finder, only func(*Pane) bool) {
	panes, err := w.Panes(ctx)
	if err != nil {
		return
	}
	names := map[string]string{}
	for _, p := range panes {
		if p.Name == "" || p.IsSidebar() || p.IsEmpty() || only != nil && !only(p) {
			continue
		}
		for _, e := range p.Trail {
			if r, err := w.RefFor(ctx, f, e, p.CWD); err == nil && r.ID != "" {
				names[r.Key()] = p.Name
			}
		}
	}
	if len(names) == 0 {
		return
	}
	cur := w.State().Names
	changed := false
	for k, n := range names {
		if cur[k] != n {
			changed = true
		}
	}
	if !changed {
		return
	}
	_ = state.Update(func(s *state.State) error {
		for k, n := range names {
			s.SetName(k, n)
		}
		return nil
	})
	w.st = nil // read again
}

// NameOf is conversation key's name ("" when it has none).
func (w *WS) NameOf(key string) string { return w.State().Names[key] }

// NameConversation names conversation key ("" removes the name) and gives
// the name to the open panes whose current run is that conversation.
func (w *WS) NameConversation(ctx context.Context, f *sources.Finder, key, name string) error {
	if err := state.Update(func(s *state.State) error { s.SetName(key, name); return nil }); err != nil {
		return err
	}
	w.st = nil
	panes, err := w.Panes(ctx)
	if err != nil {
		return nil
	}
	var cmds []tmux.Cmd
	for _, p := range panes {
		last := p.Trail.Last()
		if last == nil || p.IsSidebar() || p.IsEmpty() {
			continue
		}
		if r, err := w.RefFor(ctx, f, *last, p.CWD); err != nil || r.Key() != key {
			continue
		}
		if name == "" {
			cmds = append(cmds, tmux.Cmd{"set-option", "-p", "-u", "-t", p.ID, sidebar.OptName})
		} else {
			cmds = append(cmds, tmux.Cmd{"set-option", "-p", "-t", p.ID, sidebar.OptName, name})
		}
	}
	if len(cmds) > 0 {
		_, _ = w.T.Run(ctx, cmds...)
		w.RefreshSidebars(ctx)
	}
	return nil
}

// nameFromConversation gives pane the name of the conversation it now
// continues (key), when that has one.
func (w *WS) nameFromConversation(ctx context.Context, pane, key string) {
	if n := w.NameOf(key); n != "" {
		_, _ = w.T.Run(ctx, tmux.Cmd{"set-option", "-p", "-t", pane, sidebar.OptName, n})
	}
}
