package watch

import (
	"context"
	"hash/fnv"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/limits"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// PaneRow is one pane as seen by a tick.
type PaneRow struct {
	ID      string
	Agent   session.AgentID
	Start   time.Time
	Dead    bool
	Height  int
	Session string
}

// Client is one attached client.
type Client struct {
	Name     string
	Session  string
	Activity int64
}

// Event is a limit or warn line seen on an agent's screen.
type Event struct {
	Pane      string
	Agent     session.AgentID
	Severity  limits.Severity
	Text      string // the matched line (normalized); never logged
	PatternID string
	Session   string
	At        time.Time
}

// Detector looks at the bottom of agent panes once per tick (§10).
type Detector struct {
	T        *tmux.Client
	Patterns limits.Set
	Cfg      config.Limits
	Now      func() time.Time
	Log      *log.Logger

	// OnTick runs after the pane list is read (status bar upkeep).
	OnTick func(ctx context.Context, panes []PaneRow, clients []Client)
	// OnEvent runs for each new limit or warn line outside grace and
	// cooldown. It reports whether it prompted or acted, which starts the
	// pane's cooldown.
	OnEvent func(ctx context.Context, ev Event, clients []Client) bool

	hashes   map[string]uint64          // pane → hash of the last capture
	seen     map[string]map[string]bool // pane instance → matched lines
	cooldown map[string]time.Time       // pane → no prompts until
}

const (
	listFormat   = "#{pane_id}\t#{@ajan_agent}\t#{@ajan_start}\t#{pane_dead}\t#{pane_height}\t#{session_name}"
	clientFormat = "#{client_name}\t#{client_control_mode}\t#{client_session}\t#{client_activity}"
)

// Tick implements Ticker: one exec lists clients and panes; when a client
// is attached, one more exec captures every agent pane.
func (d *Detector) Tick(ctx context.Context) (bool, error) {
	if d.hashes == nil {
		d.hashes, d.seen, d.cooldown = map[string]uint64{}, map[string]map[string]bool{}, map[string]time.Time{}
	}
	out, err := d.T.Run(ctx,
		tmux.DelimCmd("clients"), tmux.Cmd{"list-clients", "-F", clientFormat},
		tmux.DelimCmd("panes"), tmux.Cmd{"list-panes", "-a", "-F", listFormat})
	if err != nil {
		return false, err
	}
	sec := tmux.Sections(out)
	var clients []Client
	for _, r := range tmux.Rows(sec["clients"], 4) {
		if r[1] == "1" {
			continue // control mode cannot show prompts
		}
		act, _ := strconv.ParseInt(r[3], 10, 64)
		clients = append(clients, Client{Name: r[0], Session: r[2], Activity: act})
	}
	var panes, agents []PaneRow
	for _, r := range tmux.Rows(sec["panes"], 6) {
		p := PaneRow{ID: r[0], Agent: session.AgentID(r[1]), Dead: r[3] == "1", Session: r[5]}
		p.Height, _ = strconv.Atoi(r[4])
		if ms, err := strconv.ParseInt(r[2], 10, 64); err == nil && ms > 0 {
			p.Start = time.UnixMilli(ms)
		}
		panes = append(panes, p)
		if p.Agent != "" && !p.Dead && session.Known(string(p.Agent)) {
			agents = append(agents, p)
		}
	}
	d.prune(panes)
	if d.OnTick != nil {
		d.OnTick(ctx, panes, clients)
	}
	if len(clients) == 0 || len(agents) == 0 {
		return len(clients) > 0, nil
	}
	var cmds []tmux.Cmd
	for _, p := range agents {
		start := p.Height - d.Cfg.ScanLines
		if start < 0 {
			start = 0
		}
		cmds = append(cmds, tmux.DelimCmd(p.ID), tmux.Cmd{"capture-pane", "-p", "-J", "-t", p.ID, "-S", strconv.Itoa(start)})
	}
	out, err = d.T.Run(ctx, cmds...)
	if err != nil {
		return true, err
	}
	sec = tmux.Sections(out)
	for _, p := range agents {
		screen, ok := sec[p.ID]
		if !ok {
			continue
		}
		h := fnv.New64a()
		_, _ = h.Write([]byte(screen))
		sum := h.Sum64()
		if d.hashes[p.ID] == sum {
			continue
		}
		d.hashes[p.ID] = sum
		d.scan(ctx, p, screen, clients)
	}
	return true, nil
}

// scan matches the lines of one changed screen.
func (d *Detector) scan(ctx context.Context, p PaneRow, screen string, clients []Client) {
	now := d.Now()
	inst := p.ID + "@" + strconv.FormatInt(p.Start.UnixMilli(), 10)
	seen := d.seen[inst]
	if seen == nil {
		seen = map[string]bool{}
		d.seen[inst] = seen
	}
	for _, raw := range strings.Split(screen, "\n") {
		line := Normalize(raw)
		if line == "" {
			continue
		}
		pat, ok := d.Patterns.Match(p.Agent, line)
		if !ok || pat.Severity == limits.SevIgnore || seen[line] {
			continue
		}
		seen[line] = true
		if !p.Start.IsZero() && now.Sub(p.Start) < d.Cfg.Grace.Duration {
			d.logf("pane %s %s: %s within grace", p.ID, p.Agent, pat.ID)
			continue
		}
		if pat.Severity == limits.SevLimit && now.Before(d.cooldown[p.ID]) {
			d.logf("pane %s %s: %s during cooldown", p.ID, p.Agent, pat.ID)
			continue
		}
		d.logf("pane %s %s: %s", p.ID, p.Agent, pat.ID)
		ev := Event{Pane: p.ID, Agent: p.Agent, Severity: pat.Severity, Text: line, PatternID: pat.ID, Session: p.Session, At: now}
		if d.OnEvent != nil && d.OnEvent(ctx, ev, clients) {
			d.cooldown[p.ID] = now.Add(d.Cfg.Cooldown.Duration)
		}
	}
}

// prune forgets panes that are gone and instances that ended.
func (d *Detector) prune(panes []PaneRow) {
	live := map[string]bool{}
	inst := map[string]bool{}
	for _, p := range panes {
		live[p.ID] = true
		inst[p.ID+"@"+strconv.FormatInt(p.Start.UnixMilli(), 10)] = true
	}
	for id := range d.hashes {
		if !live[id] {
			delete(d.hashes, id)
		}
	}
	for id := range d.cooldown {
		if !live[id] {
			delete(d.cooldown, id)
		}
	}
	for k := range d.seen {
		if !inst[k] {
			delete(d.seen, k)
		}
	}
}

func (d *Detector) logf(format string, args ...any) {
	if d.Log != nil {
		d.Log.Printf(format, args...)
	}
}

var (
	ansiRe  = regexp.MustCompile("\x1b(?:\\[[0-9;?]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\)|[()][0-9A-Za-z]|[=>78DEHMNOc])")
	spaceRe = regexp.MustCompile(`\s+`)
)

// frame characters agents draw around messages.
const frame = "│┃║|╭╮╰╯┌┐└┘─━═⎿⏺●■✦✗✕×*>• \t"

// Normalize strips escapes and frame characters and collapses whitespace.
func Normalize(s string) string {
	s = ansiRe.ReplaceAllString(s, "")
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.Trim(s, frame)
}
