// Package bus lets agents talk to each other through tmux (§12): list
// panes, read a pane's screen, send it a message and wait until it is idle.
// Agents call these commands from their own shell tool; targets are always
// panes on imux's socket.
package bus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// Bus runs bus commands for one invocation.
type Bus struct {
	W *workspace.WS
	// Sleep pauses between screen samples; tests replace it.
	Sleep func(time.Duration)
}

// New returns a bus on w.
func New(w *workspace.WS) *Bus { return &Bus{W: w, Sleep: time.Sleep} }

// ErrAmbiguous means an agent name matched several panes.
var ErrAmbiguous = errors.New("ambiguous target")

// ErrNoTarget means nothing matched.
var ErrNoTarget = errors.New("no such pane")

// Resolve turns a target ("%N" or an agent name) into a pane. An agent name
// resolves to the unique live pane running it in the caller's session, then
// server-wide.
func (b *Bus) Resolve(ctx context.Context, target, caller string) (*workspace.Pane, error) {
	all, err := b.W.Panes(ctx)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(target, "%") {
		for _, p := range all {
			if p.ID == target {
				return p, nil
			}
		}
		return nil, fmt.Errorf("%w: %s", ErrNoTarget, target)
	}
	if !session.Known(target) {
		return nil, fmt.Errorf("%w: %s", ErrNoTarget, i18n.T("bus.bad_target", target))
	}
	var callerSession string
	for _, p := range all {
		if p.ID == caller {
			callerSession = p.Session
		}
	}
	var local, global []*workspace.Pane
	for _, p := range all {
		if string(p.Agent) != target || p.Dead {
			continue
		}
		global = append(global, p)
		if callerSession != "" && p.Session == callerSession {
			local = append(local, p)
		}
	}
	for _, set := range [][]*workspace.Pane{local, global} {
		switch len(set) {
		case 0:
			continue
		case 1:
			return set[0], nil
		default:
			var c []string
			for _, p := range set {
				c = append(c, p.ID+" ("+p.Session+")")
			}
			return nil, fmt.Errorf("%w: %s", ErrAmbiguous, i18n.T("bus.ambiguous", target, strings.Join(c, ", ")))
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrNoTarget, i18n.T("bus.no_agent_pane", target))
}

// Info is one pane as `imux panes` shows it.
type Info struct {
	Pane    string          `json:"pane"`
	Agent   session.AgentID `json:"agent,omitempty"`
	Label   string          `json:"label,omitempty"`
	CWD     string          `json:"cwd"`
	Session string          `json:"session"`
	State   string          `json:"state"` // idle | busy | exited
	// IdleSeconds is how long the screen has not changed (a lower bound
	// when measured by sampling).
	IdleSeconds int `json:"idle_seconds"`
}

// Activity is what the watcher publishes about screen changes.
type Activity struct {
	At      int64            `json:"at"`      // unix ms of the tick that wrote it
	Changed map[string]int64 `json:"changed"` // pane → unix ms of the last change seen
}

// ReadActivity returns the watcher's activity file when it is fresh
// (written within maxAge).
func ReadActivity(now time.Time, maxAge time.Duration) *Activity {
	b, err := os.ReadFile(paths.Activity())
	if err != nil {
		return nil
	}
	var a Activity
	if json.Unmarshal(b, &a) != nil || now.Sub(time.UnixMilli(a.At)) > maxAge {
		return nil
	}
	return &a
}

// WriteActivity stores the watcher's view of screen changes.
func WriteActivity(a *Activity) error {
	b, err := json.Marshal(a)
	if err != nil {
		return err
	}
	return paths.WriteFile(paths.Activity(), b)
}

// Panes lists every pane with its idle/busy state: from the watcher's
// activity when it is fresh, else from two screen samples 1 s apart.
func (b *Bus) Panes(ctx context.Context) ([]Info, error) {
	all, err := b.W.Panes(ctx)
	if err != nil {
		return nil, err
	}
	now := b.W.Now()
	idleAfter := b.W.Cfg.Bus.IdleAfter.Duration
	act := ReadActivity(now, 2*b.W.Cfg.Limits.PollInterval.Duration+time.Second)
	var sampled map[string]bool
	if act == nil {
		ids := make([]string, 0, len(all))
		for _, p := range all {
			if !p.Dead {
				ids = append(ids, p.ID)
			}
		}
		h1, err := b.hashes(ctx, ids)
		if err != nil {
			return nil, err
		}
		b.Sleep(time.Second)
		h2, err := b.hashes(ctx, ids)
		if err != nil {
			return nil, err
		}
		sampled = map[string]bool{}
		for _, id := range ids {
			sampled[id] = h1[id] != h2[id]
		}
	}
	var out []Info
	for _, p := range all {
		in := Info{Pane: p.ID, Agent: p.Agent, Label: p.Label, CWD: p.CWD, Session: p.Session}
		switch {
		case p.Dead:
			in.State = "exited"
		case act != nil:
			changed, ok := act.Changed[p.ID]
			idle := now.Sub(time.UnixMilli(changed))
			if !ok {
				idle = now.Sub(p.Start)
			}
			in.IdleSeconds = int(idle / time.Second)
			in.State = "busy"
			if idle >= idleAfter {
				in.State = "idle"
			}
		case sampled[p.ID]:
			in.State = "busy"
		default:
			in.State, in.IdleSeconds = "idle", 1
		}
		out = append(out, in)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Session < out[j].Session })
	return out, nil
}

// hashes captures the visible screens of panes in one exec.
func (b *Bus) hashes(ctx context.Context, ids []string) (map[string]uint64, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var cmds []tmux.Cmd
	for _, id := range ids {
		cmds = append(cmds, tmux.DelimCmd(id), tmux.Cmd{"capture-pane", "-p", "-J", "-t", id})
	}
	out, err := b.W.T.Run(ctx, cmds...)
	if err != nil {
		return nil, err
	}
	res := map[string]uint64{}
	for id, screen := range tmux.Sections(out) {
		h := fnv.New64a()
		_, _ = h.Write([]byte(screen))
		res[id] = h.Sum64()
	}
	return res, nil
}
