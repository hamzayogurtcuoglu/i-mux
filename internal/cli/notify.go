package cli

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/notify"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/watch"
)

// NotifyEvery is the least time between two notifications of one pane.
const NotifyEvery = 30 * time.Second

// notifier turns the watcher's pane events into notifications (§9.3
// "Notifications"): not for a pane the user is looking at, at most one of
// each kind per pane per NotifyEvery.
type notifier struct {
	cfg  config.Notify
	t    *tmux.Client
	n    *notify.Notifier
	now  func() time.Time
	mu   sync.Mutex
	last map[string]time.Time
}

func newNotifier(cfg config.Notify, t *tmux.Client) *notifier {
	return &notifier{
		cfg: cfg, t: t, now: time.Now, last: map[string]time.Time{},
		n: &notify.Notifier{Command: strings.Fields(cfg.Command)},
	}
}

// Send notifies about events.
func (nf *notifier) Send(ctx context.Context, events []watch.PaneEvent) {
	if !nf.cfg.Enabled || len(events) == 0 {
		return
	}
	out, err := nf.t.Run(ctx,
		tmux.DelimCmd("clients"), tmux.Cmd{"list-clients", "-F", "#{client_tty}\t#{client_name}\t#{window_id}\t#{client_flags}"},
		tmux.DelimCmd("term"), tmux.Cmd{"show-options", "-gqv", "@imux_terminal"},
		tmux.DelimCmd("panes"), tmux.Cmd{"list-panes", "-a", "-F", "#{pane_id}\t#{@imux_agent}\t#{@imux_name}\t#{window_id}\t#{window_index}\t#{@imux_cwd}\t#{@imux_tab}\t#{@imux_tab_auto}"})
	if err != nil {
		return
	}
	sec := tmux.Sections(out)
	program := strings.TrimSpace(sec["term"])
	var clients []notify.Client
	looking := map[string]bool{} // windows a focused terminal shows
	for _, r := range tmux.Rows(sec["clients"], 4) {
		clients = append(clients, notify.Client{TTY: r[0], Program: program})
		if strings.Contains(r[3], "focused") {
			looking[r[2]] = true
		}
	}
	panes := map[string][]string{}
	for _, r := range tmux.Rows(sec["panes"], 8) {
		panes[r[0]] = r
	}
	now := nf.now()
	for _, ev := range events {
		p, ok := panes[ev.Pane]
		if !ok || p[1] == "" || looking[p[3]] {
			continue // gone, not an agent, or in front of the user
		}
		key := ev.Pane + " " + ev.Kind // a question is news even right after "finished"
		nf.mu.Lock()
		recent := now.Sub(nf.last[key]) < NotifyEvery
		if !recent {
			nf.last[key] = now
		}
		nf.mu.Unlock()
		if recent {
			continue
		}
		name := p[2]
		if name == "" {
			name = session.ShortName(session.AgentID(p[1]))
		}
		title := i18n.T("notify.finished", name)
		if ev.Kind == watch.EventNeedsYou {
			title = i18n.T("notify.needs", name)
		}
		project := filepath.Base(p[5])
		tab := p[4]
		if t := firstNonEmpty(p[6], p[7]); t != "" {
			tab += " " + t
		}
		m := notify.Message{Title: title + " · " + project, Body: i18n.T("notify.body", project, tab), Sound: nf.cfg.Sound}
		if _, err := nf.n.System(ctx, m); err != nil {
			nf.n.Terminals(clients, m)
		}
		for _, c := range tmux.Rows(sec["clients"], 4) {
			_, _ = nf.t.Run(ctx, tmux.Cmd{"display-message", "-c", c[1], tmux.EscapeMessage("🔔 " + m.Title)})
		}
	}
}
