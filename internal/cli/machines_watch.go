package cli

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/machine"
	"github.com/hamzayogurtcuoglu/i-mux/internal/notify"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/watch"
)

// How often the watcher asks a machine (D73): connected, and not.
var (
	MachineEvery   = 3 * time.Second
	MachineRetry   = 10 * time.Second
	machinesReload = 5 * time.Second
)

// keepMachines asks each added machine for its report, each on its own
// (a slow one stalls no other), and follows the list as machines are added
// and removed.
func keepMachines(t *tmux.Client, cfg config.Notify, logger *log.Logger) {
	type run struct {
		stop   context.CancelFunc
		target string
	}
	running := map[string]run{}
	for {
		ms, err := machine.Load()
		if err != nil {
			logger.Printf("machines: %v", err)
		}
		want := map[string]machine.Machine{}
		for _, m := range ms {
			want[m.Name] = m
		}
		// Removed, or given another destination: that one stops.
		for name, r := range running {
			if m, ok := want[name]; !ok || m.Target != r.target {
				r.stop()
				delete(running, name)
			}
		}
		for _, m := range ms {
			if _, ok := running[m.Name]; !ok {
				ctx, stop := context.WithCancel(context.Background())
				running[m.Name] = run{stop, m.Target}
				go watchMachine(ctx, m, t, cfg, logger)
			}
		}
		time.Sleep(machinesReload)
	}
}

// watchMachine asks m until ctx ends: its state written for the sidebars
// when it changed, its agents' questions and finished work notified.
func watchMachine(ctx context.Context, m machine.Machine, t *tmux.Client, cfg config.Notify, logger *log.Logger) {
	prev := machine.ReadState(m.Name)
	seen := false // the first report is no news
	nt := &remoteNotices{cfg: cfg, t: t, working: map[string]time.Time{}}
	for ctx.Err() == nil {
		_, st := machine.Fetch(ctx, m)
		if ctx.Err() != nil {
			return
		}
		st.At = time.Now()
		if st.Status != machine.StatusOK {
			st.Projects = prev.Projects // shown dimmed as they were last
			if prev.Status == machine.StatusOK {
				logger.Printf("machine %s: %s %s", m.Name, st.Status, st.Error)
			}
		}
		if changed(prev, st) {
			if err := machine.WriteState(m.Name, st); err != nil {
				logger.Printf("machine %s: %v", m.Name, err)
			}
			sidebar.RefreshAll(ctx, t)
		}
		if st.Status == machine.StatusOK {
			for _, n := range nt.compare(m.Name, prev, st, seen, time.Now()) {
				nt.send(ctx, n.title, m.Name, n.project)
			}
			seen = true
		}
		prev = st
		wait := MachineEvery
		if st.Status != machine.StatusOK {
			wait = MachineRetry
		}
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
	}
}

// changed reports a state the sidebars should show again (its time apart).
func changed(a, b machine.State) bool {
	a.At, b.At = time.Time{}, time.Time{}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) != string(jb)
}

// remoteNotices notifies a machine's agents as local ones are: one that
// turns to Needs you, or stops after working at least watch.MinWork.
type remoteNotices struct {
	cfg     config.Notify
	t       *tmux.Client
	mu      sync.Mutex
	working map[string]time.Time // machine/session/pane → since when it works
}

// remoteNotice is one notification compare found.
type remoteNotice struct{ title, project string }

func (nt *remoteNotices) compare(name string, prev, cur machine.State, seen bool, now time.Time) []remoteNotice {
	var out []remoteNotice
	was := map[string]string{}
	for _, p := range prev.Projects {
		for _, x := range p.Panes {
			was[p.Session+"/"+x.ID] = x.State
		}
	}
	nt.mu.Lock()
	defer nt.mu.Unlock()
	for _, p := range cur.Projects {
		for _, x := range p.Panes {
			key := name + "/" + p.Session + "/" + x.ID
			before := was[p.Session+"/"+x.ID]
			if x.State == sidebar.StateWorking {
				if _, ok := nt.working[key]; !ok {
					nt.working[key] = now
				}
				continue
			}
			start, worked := nt.working[key]
			delete(nt.working, key)
			if !seen || before == x.State {
				continue
			}
			who := x.Name
			if who == "" {
				who = session.ShortName(session.AgentID(x.Agent))
			}
			switch {
			case x.State == sidebar.StateBlocked:
				out = append(out, remoteNotice{i18n.T("notify.needs", who), p.Name})
			case worked && now.Sub(start) >= watch.MinWork:
				out = append(out, remoteNotice{i18n.T("notify.finished", who), p.Name})
			}
		}
	}
	return out
}

func (nt *remoteNotices) send(ctx context.Context, title, machineName, project string) {
	if !nt.cfg.Enabled {
		return
	}
	where := machineName + " · " + project
	m := notify.Message{Title: title + " · " + where, Body: i18n.T("machine.notify_body", where), Sound: nt.cfg.Sound}
	n := &notify.Notifier{Command: strings.Fields(nt.cfg.Command)}
	_, _ = n.System(ctx, m)
	if out, err := nt.t.Run(ctx, tmux.Cmd{"list-clients", "-F", "#{client_name}"}); err == nil {
		for _, c := range tmux.Rows(out, 1) {
			_, _ = nt.t.Run(ctx, tmux.Cmd{"display-message", "-c", c[0], tmux.EscapeMessage("🔔 " + m.Title)})
		}
	}
}
