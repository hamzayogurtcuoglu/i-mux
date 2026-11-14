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
	Width   int
	Height  int
	Session string
	// Unreaped: dead, but tmux has no exit status or signal for it yet,
	// so the pane-died hook has not run (see reapNudge).
	Unreaped bool
	// Hook is the state the agent's hook last reported (D71), and HookAt
	// when; "" without one.
	Hook   string
	HookAt time.Time
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

	// OnTick runs after the pane list is read (status bar upkeep). The
	// commands it returns join this tick's second exec.
	OnTick func(ctx context.Context, panes []PaneRow, clients []Client) []tmux.Cmd
	// OnTickFailed runs when the exec carrying OnTick's commands failed.
	OnTickFailed func()
	// OnActivity receives, after each attached tick, when each agent pane's
	// screen last changed (the bus uses it for idle/busy) and which screens
	// ask the user something (Asking).
	OnActivity func(at time.Time, changed map[string]time.Time, asking map[string]bool)
	// WatchDetached keeps reading the agents' screens with no client
	// attached (at the idle interval), for notifications: a closed terminal
	// is when they matter most. Limit lines are scanned only when attached.
	WatchDetached bool
	// OnEvent runs for each new limit or warn line outside grace and
	// cooldown. It reports whether it prompted or acted, which starts the
	// pane's cooldown.
	OnEvent func(ctx context.Context, ev Event, clients []Client) bool
	// Seed is when each pane's agent last worked, as the watcher before
	// this one saw it (its activity file): a pane met for the first time
	// is not at work until its screen changes.
	Seed map[string]time.Time

	hashes   map[string]uint64          // pane → hash of the last capture
	screens  map[string]string          // pane → its last capture
	sizes    map[string]string          // pane → its size ("WxH") at the last capture
	settle   map[string]bool            // pane → resized while quiet at the last tick
	changed  map[string]time.Time       // pane → when its agent last worked (its screen changed)
	busy     map[string]bool            // pane → its last screen shows its agent's busy line (D55)
	asking   map[string]bool            // pane → its last screen asks a question
	seen     map[string]map[string]bool // pane instance → matched lines
	cooldown map[string]time.Time       // pane → no prompts until
	nudges   map[string]int             // dead, unreaped pane → reap nudges sent
	spent    map[string]time.Time       // pane → its hook (by time) the busy line has outdated
}

const (
	listFormat   = "#{pane_id}\t#{@imux_agent}\t#{@imux_start}\t#{pane_dead}\t#{pane_height}\t#{session_name}\t#{pane_dead_status}#{pane_dead_signal}\t#{pane_width}\t#{@imux_hook}\t#{@imux_hook_at}"
	clientFormat = "#{client_name}\t#{client_control_mode}\t#{client_session}\t#{client_activity}"
)

// reapNudge makes the tmux server reap its exited children. tmux builds that
// use libutempter (Debian, Ubuntu) can lose the SIGCHLD of a pane's process:
// when the pane's pty closes, libutempter sets SIGCHLD to SIG_DFL while its
// helper runs, and a child that exits in that window stays a zombie. The pane
// is then dead with no exit status, and the pane-died hook (imux __pane-died:
// shell, close or fallback) never runs. Any child exit makes tmux reap them
// all, so a run-shell job that exits at once is enough.
var reapNudge = tmux.Cmd{"run-shell", "-b", "true"}

// maxNudges bounds the nudges sent for one dead pane.
const maxNudges = 3

// Tick implements Ticker: one exec lists clients and panes; when a client
// is attached, one more exec captures every agent pane.
func (d *Detector) Tick(ctx context.Context) (bool, error) {
	if d.hashes == nil {
		d.hashes, d.seen, d.cooldown = map[string]uint64{}, map[string]map[string]bool{}, map[string]time.Time{}
		d.changed, d.busy = map[string]time.Time{}, map[string]bool{}
		d.screens, d.sizes, d.settle = map[string]string{}, map[string]string{}, map[string]bool{}
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
	nudge := false
	nudges := map[string]int{}
	for _, r := range tmux.Rows(sec["panes"], 10) {
		p := PaneRow{ID: r[0], Agent: session.AgentID(r[1]), Dead: r[3] == "1", Session: r[5], Hook: r[8]}
		if ms, err := strconv.ParseInt(r[9], 10, 64); err == nil && ms > 0 {
			p.HookAt = time.UnixMilli(ms)
		}
		p.Unreaped = p.Dead && r[6] == ""
		p.Height, _ = strconv.Atoi(r[4])
		p.Width, _ = strconv.Atoi(r[7])
		if ms, err := strconv.ParseInt(r[2], 10, 64); err == nil && ms > 0 {
			p.Start = time.UnixMilli(ms)
		}
		panes = append(panes, p)
		// A few nudges per dead pane: tmux 3.2 has no pane_dead_signal, so
		// a pane killed by a signal there never shows a status.
		if p.Unreaped {
			nudges[p.ID] = d.nudges[p.ID] + 1
			nudge = nudge || nudges[p.ID] <= maxNudges
		}
		if p.Agent != "" && !p.Dead && session.Known(string(p.Agent)) {
			agents = append(agents, p)
		}
	}
	d.nudges = nudges
	d.prune(panes)
	var extra []tmux.Cmd
	if d.OnTick != nil {
		extra = d.OnTick(ctx, panes, clients)
	}
	if nudge {
		extra = append(extra, reapNudge)
	}
	detached := len(clients) == 0
	if len(agents) == 0 || detached && !d.WatchDetached {
		if len(extra) > 0 {
			if _, err := d.T.Run(ctx, extra...); err != nil {
				d.tickFailed()
				return len(clients) > 0, err
			}
		}
		return len(clients) > 0, nil
	}
	cmds := extra
	// The visible screen is captured and its last scan_lines non-blank
	// lines are scanned: inline CLIs write from the top, so on a short
	// session the message is not on the bottom rows of the screen.
	for _, p := range agents {
		cmds = append(cmds, tmux.DelimCmd(p.ID), tmux.Cmd{"capture-pane", "-p", "-J", "-t", p.ID})
	}
	out, err = d.T.Run(ctx, cmds...)
	if err != nil {
		if len(extra) > 0 {
			d.tickFailed()
		}
		return !detached, err
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
		size := strconv.Itoa(p.Width) + "x" + strconv.Itoa(p.Height)
		last, known := d.hashes[p.ID]
		resized := known && d.sizes[p.ID] != size
		d.sizes[p.ID] = size
		if known && last == sum {
			d.settle[p.ID] = false
			if d.busy[p.ID] {
				d.changed[p.ID] = d.Now() // it still says it works (a long tool call)
			}
			continue
		}
		old := d.screens[p.ID]
		d.hashes[p.ID], d.screens[p.ID] = sum, screen
		busy, says := Busy(p.Agent, screen)
		d.busy[p.ID] = busy
		if d.work(p.ID, known, resized, old, screen, busy, says) {
			d.changed[p.ID] = d.Now()
		}
		if d.asking == nil {
			d.asking = map[string]bool{}
		}
		d.asking[p.ID] = Asking(screen)
		if !detached {
			d.scan(ctx, p, screen, clients)
		}
	}
	for _, p := range agents {
		d.hooked(p)
	}
	if d.OnActivity != nil {
		d.OnActivity(d.Now(), d.changed, d.asking)
	}
	return !detached, nil
}

// HookFresh is how long a hook's "working" counts without the agent's busy
// line: a turn the user interrupted runs no hook (Claude), so it lapses.
const HookFresh = WorkingFor

// hooked takes what the agent's hook said over the screen's reading (D71):
// blocked is a question at once, idle ends the work at once, working
// counts while the busy line shows or HookFresh after the hook. The busy
// line wins over blocked and idle (a permission granted; a new turn Codex
// does not report). A hook from before this run is no news.
func (d *Detector) hooked(p PaneRow) {
	if p.Hook == "" || p.HookAt.Before(p.Start) {
		return
	}
	if d.asking == nil {
		d.asking = map[string]bool{}
	}
	if d.spent == nil {
		d.spent = map[string]time.Time{}
	}
	// Once the busy line showed after a blocked or idle hook, the agent
	// moved on (a permission granted, a turn Codex does not report): the
	// screen decides until the next hook, so a pause between two tools is
	// no new question.
	if d.spent[p.ID].Equal(p.HookAt) {
		return
	}
	if p.Hook != "working" && d.busy[p.ID] && d.changed[p.ID].After(p.HookAt) {
		d.spent[p.ID] = p.HookAt
		return
	}
	changed, asking := hookOverride(p.Hook, p.HookAt, d.Now(), d.changed[p.ID], d.asking[p.ID], d.busy[p.ID])
	d.changed[p.ID], d.asking[p.ID] = changed, asking
}

// hookOverride is the pane's last-work time and question, given the hook's
// state at hookAt, now, the screen's reading (changed, asking) and whether
// the screen shows the agent's busy line.
func hookOverride(state string, hookAt, now, changed time.Time, asking, busy bool) (time.Time, bool) {
	quiet := now.Add(-WorkingFor) // a time that reads as no longer working
	switch state {
	case "working":
		if busy || now.Sub(hookAt) < HookFresh {
			return now, false
		}
	case "blocked":
		if busy {
			return now, false
		}
		if changed.After(quiet) {
			changed = quiet
		}
		return changed, true
	case "idle":
		if busy && changed.After(hookAt) {
			return now, false
		}
		if changed.After(quiet) {
			changed = quiet
		}
		return changed, false
	}
	return changed, asking
}

// work reports whether a pane's changed screen is its agent at work. It is
// not when the watcher meets the pane (it may be one a watcher before this
// one knew: then it worked when that one saw it last), when a quiet pane was
// resized (the panel opened or closed) or redraws right after it, when an
// agent that says when it works (says) does not show its busy line (D55:
// the user moved through one of its lists), or when the user types in the
// agent's input (Typing).
func (d *Detector) work(id string, known, resized bool, old, cur string, busy, says bool) bool {
	now := d.Now()
	settle := d.settle[id]
	d.settle[id] = false
	if !known {
		if t, ok := d.Seed[id]; ok {
			d.changed[id] = t
		} else {
			d.changed[id] = now.Add(-WorkingFor)
		}
		return false
	}
	if quiet := now.Sub(d.changed[id]) >= WorkingFor; quiet && (resized || settle) {
		d.settle[id] = resized
		return false
	}
	if says {
		return busy
	}
	return !Typing(old, cur)
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
	for _, raw := range lastLines(screen, d.Cfg.ScanLines) {
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

func (d *Detector) tickFailed() {
	if d.OnTickFailed != nil {
		d.OnTickFailed()
	}
}

// lastLines returns the last n lines of screen after trailing blank lines.
func lastLines(screen string, n int) []string {
	lines := strings.Split(strings.TrimRight(screen, " \t\r\n"), "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
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
			delete(d.screens, id)
			delete(d.sizes, id)
			delete(d.settle, id)
		}
	}
	for id := range d.asking {
		if !live[id] {
			delete(d.asking, id)
		}
	}
	for id := range d.busy {
		if !live[id] {
			delete(d.busy, id)
		}
	}
	for id := range d.cooldown {
		if !live[id] {
			delete(d.cooldown, id)
		}
	}
	for id := range d.spent {
		if !live[id] {
			delete(d.spent, id)
		}
	}
	for id := range d.changed {
		if !live[id] {
			delete(d.changed, id)
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
