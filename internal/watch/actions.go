package watch

import (
	"context"
	"log"
	"os/exec"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/limits"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
	"github.com/hamzayogurtcuoglu/i-mux/internal/textfmt"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// Actor records limits and acts on them according to on_limit (§10).
type Actor struct {
	W   *workspace.WS
	Log *log.Logger
	// Spawn starts `imux args...` detached (auto mode); tests replace it.
	Spawn func(self string, args ...string) error
}

// resolve runs after the caches are dropped; tests pin the binaries.
var resolve = func(*workspace.WS) {}

// SpawnDetached starts a command in its own session without waiting.
func SpawnDetached(self string, args ...string) error {
	return sysx.StartDetached(func() *exec.Cmd {
		cmd := exec.Command(self, args...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
		return cmd
	})
}

// Record writes the limit to state.json (§15) and returns it.
func (a *Actor) Record(ev Event) (*state.Limit, error) {
	cfg := a.W.Cfg
	l := &state.Limit{
		At: ev.At, Text: ev.Text, Source: "screen", Pane: ev.Pane, Severity: string(ev.Severity),
		ResetAt: limits.ParseReset(ev.Text, ev.At, time.Local),
	}
	l.ExpiresAt = l.ResetAt
	if l.ExpiresAt.IsZero() || !l.ExpiresAt.After(ev.At) {
		l.ExpiresAt = ev.At.Add(cfg.Limits.DefaultExpiry.Duration)
	}
	err := state.Update(func(s *state.State) error {
		// A warning never replaces an active limit.
		if ev.Severity == limits.SevWarn {
			if old := s.ActiveLimit(string(ev.Agent), ev.At); old != nil {
				return nil
			}
		}
		s.SetLimit(string(ev.Agent), l)
		return nil
	})
	return l, err
}

// OnEvent implements Detector.OnEvent: record, then ask / auto / off. It
// reports whether it prompted or acted (which starts the pane's cooldown).
func (a *Actor) OnEvent(ctx context.Context, ev Event, clients []Client) bool {
	w := a.W
	w.Refresh() // the watcher lives long: read state and binaries afresh
	resolve(w)
	l, err := a.Record(ev)
	if err != nil {
		a.logf("record limit for %s: %v", ev.Agent, err)
	}
	if ev.Severity != limits.SevLimit {
		return false
	}
	name := session.ShortName(ev.Agent)
	until := ""
	if l != nil && !l.ResetAt.IsZero() {
		until = textfmt.Clock(w.Now(), l.ResetAt)
	}
	targets := a.targets(ev.Agent)
	client := pickClient(clients, ev.Session)
	if len(targets) == 0 {
		w.Message(ctx, client, ev.Pane, i18n.T("watch.all_limited", name))
		a.logf("pane %s %s: every other agent is limited; notified", ev.Pane, ev.Agent)
		return true
	}
	switch w.Cfg.OnLimit {
	case "auto":
		to := targets[0]
		spawn := a.Spawn
		if spawn == nil {
			spawn = SpawnDetached
		}
		args := []string{"switch", string(to), "--pane", ev.Pane, "--reason", "limit"}
		if client != "" {
			args = append(args, "--client", client)
		}
		if err := spawn(w.Self, args...); err != nil {
			a.logf("auto switch: %v", err)
			return true
		}
		w.Message(ctx, client, ev.Pane, i18n.T("watch.auto", name, resetSuffix(until), session.ShortName(to)))
		a.logf("pane %s %s: auto switch to %s", ev.Pane, ev.Agent, to)
	case "off":
		w.Message(ctx, client, ev.Pane, i18n.T("watch.off", name, resetSuffix(until)))
		a.logf("pane %s %s: limit noted (on_limit = off)", ev.Pane, ev.Agent)
	default: // ask
		if err := w.Show(ctx, a.AskMenu(ev, until, targets, client), client, ev.Pane); err != nil {
			a.logf("ask menu: %v", err)
		}
		a.logf("pane %s %s: asked on %s", ev.Pane, ev.Agent, client)
	}
	return true
}

// targets are the installed, non-limited agents other than from, in order.
func (a *Actor) targets(from session.AgentID) []session.AgentID {
	var out []session.AgentID
	for _, x := range agent.Ordered(a.W.Cfg) {
		if x.ID != from && a.W.Installed(x.ID) && a.W.LimitedUntil(x.ID) == "" {
			out = append(out, x.ID)
		}
	}
	return out
}

// AskMenu is the "continue in …" prompt of ask mode.
func (a *Actor) AskMenu(ev Event, until string, targets []session.AgentID, client string) *workspace.Menu {
	m := &workspace.Menu{Title: i18n.T("watch.ask_title", session.ShortName(ev.Agent), resetSuffix(until))}
	for _, to := range targets {
		info := agent.MustGet(to)
		args := []string{"switch", string(to), "--pane", ev.Pane, "--reason", "limit"}
		if client != "" {
			args = append(args, "--client", client)
		}
		m.Items = append(m.Items, workspace.MenuItem{
			Name: i18n.T("watch.ask_item", info.Short), Key: info.Key, Cmd: a.W.UICommand(args...),
		})
	}
	m.Items = append(m.Items, workspace.Separator(), workspace.MenuItem{Name: i18n.T("watch.dismiss"), Key: "d", Cmd: ""})
	return m
}

// pickClient prefers the most recently active client showing session, else
// the most recently active client.
func pickClient(clients []Client, session string) string {
	best, bestAct := "", int64(-1)
	any, anyAct := "", int64(-1)
	for _, c := range clients {
		if c.Activity > anyAct {
			any, anyAct = c.Name, c.Activity
		}
		if c.Session == session && c.Activity > bestAct {
			best, bestAct = c.Name, c.Activity
		}
	}
	if best != "" {
		return best
	}
	return any
}

func resetSuffix(until string) string {
	if until == "" {
		return ""
	}
	return " " + i18n.T("watch.resets", until)
}

func (a *Actor) logf(format string, args ...any) {
	if a.Log != nil {
		a.Log.Printf(format, args...)
	}
}
