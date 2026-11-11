// Package workspace runs agents in imux's tmux workspace: starting and
// attaching sessions, launching agents into panes through launch specs, the
// per-pane options and trail (§9.4), switching, menus and pane lifecycle.
//
// Only this package, watch and bus talk to tmux.
package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/launch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Pane options (§9.4).
const (
	OptAgent    = "@imux_agent"
	OptStart    = "@imux_start"
	OptCWD      = "@imux_cwd"
	OptLabel    = "@imux_label"
	OptFlag     = "@imux_flag"
	OptTrail    = "@imux_trail"
	OptFallback = "@imux_fallback"
	OptLock     = "@imux_lock"
)

// MaxTrail is the number of runs kept in a pane's trail.
const MaxTrail = 10

// TrailEntry is one run of an agent in a pane.
type TrailEntry struct {
	Agent     session.AgentID `json:"a"`
	Start     int64           `json:"s"`           // unix ms
	End       int64           `json:"e,omitempty"` // unix ms; 0 = current
	SessionID string          `json:"id,omitempty"`
	File      string          `json:"f,omitempty"`
	Handoff   string          `json:"h,omitempty"` // handoff file that seeded this run
	Resumed   bool            `json:"r,omitempty"`
}

// StartTime returns Start as a time.
func (e TrailEntry) StartTime() time.Time { return time.UnixMilli(e.Start) }

// EndTime returns End as a time (zero while the run is current).
func (e TrailEntry) EndTime() time.Time {
	if e.End == 0 {
		return time.Time{}
	}
	return time.UnixMilli(e.End)
}

// Trail is a pane's run history, oldest first.
type Trail []TrailEntry

// ParseTrail decodes @imux_trail; garbage yields an empty trail.
func ParseTrail(s string) Trail {
	var t Trail
	if s == "" || json.Unmarshal([]byte(s), &t) != nil {
		return nil
	}
	return t
}

// String encodes the trail for the pane option.
func (t Trail) String() string {
	if len(t) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(t)
	return string(b)
}

// Last returns the newest entry, or nil.
func (t Trail) Last() *TrailEntry {
	if len(t) == 0 {
		return nil
	}
	return &t[len(t)-1]
}

// Push closes the current run at end and appends e, keeping MaxTrail entries.
func (t Trail) Push(e TrailEntry, end time.Time) Trail {
	out := append(Trail{}, t...)
	if l := out.Last(); l != nil && l.End == 0 {
		l.End = end.UnixMilli()
	}
	out = append(out, e)
	if len(out) > MaxTrail {
		out = out[len(out)-MaxTrail:]
	}
	return out
}

// Close ends the current run at end.
func (t Trail) Close(end time.Time) Trail {
	out := append(Trail{}, t...)
	if l := out.Last(); l != nil && l.End == 0 {
		l.End = end.UnixMilli()
	}
	return out
}

// WS is the workspace for one invocation.
type WS struct {
	Cfg  *config.Config
	T    *tmux.Client
	Self string // absolute path of the imux binary (symlinks resolved)
	Now  func() time.Time

	st   *state.State
	bins map[session.AgentID]agent.Resolved
}

// New returns a workspace for cfg on imux's socket.
func New(cfg *config.Config) (*WS, error) {
	self, err := Self()
	if err != nil {
		return nil, err
	}
	return &WS{Cfg: cfg, T: tmux.New(paths.TmuxConf()), Self: self, Now: time.Now}, nil
}

// Self returns the absolute, symlink-resolved path of the running binary.
func Self() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p, nil
}

// State returns state.json (read once per invocation).
func (w *WS) State() *state.State {
	if w.st == nil {
		st, err := state.Read()
		if err != nil || st == nil {
			st = &state.State{Version: state.SchemaVersion}
		}
		w.st = st
	}
	return w.st
}

// Refresh drops the per-invocation caches (state and binaries); the
// long-running watcher calls it before each use.
func (w *WS) Refresh() { w.st, w.bins = nil, nil }

// SetBins replaces the resolved agent binaries (after a cache refresh).
func (w *WS) SetBins(b map[session.AgentID]agent.Resolved) { w.bins = b }

// Bins resolves every agent (cached per invocation). Menus and hooks run
// with the tmux server's environment and rely on state.json's bin cache.
func (w *WS) Bins() map[session.AgentID]agent.Resolved {
	if w.bins == nil {
		w.bins = agent.ResolveAll(w.Cfg, w.State())
	}
	return w.bins
}

// Bin returns one agent's resolution.
func (w *WS) Bin(id session.AgentID) agent.Resolved { return w.Bins()[id] }

// Installed reports whether agent id can be launched.
func (w *WS) Installed(id session.AgentID) bool { return w.Bin(id).Installed }

// NotInstalledError is returned when launching an agent that is not installed.
type NotInstalledError struct{ Agent session.AgentID }

func (e *NotInstalledError) Error() string {
	a := agent.MustGet(e.Agent)
	return i18n.T("agent.not_installed", a.Name, a.InstallHint)
}

// DefaultAgent picks the agent for a plain `imux`: config default_agent,
// else the first installed agent in order.
func (w *WS) DefaultAgent() (session.AgentID, error) {
	if d := w.Cfg.DefaultAgent; d != "" {
		return session.AgentID(d), nil
	}
	for _, a := range agent.Ordered(w.Cfg) {
		if w.Installed(a.ID) {
			return a.ID, nil
		}
	}
	var hints []string
	for _, a := range agent.Ordered(w.Cfg) {
		hints = append(hints, "  "+a.InstallHint)
	}
	return "", errors.New(i18n.T("start.no_agent", strings.Join(hints, "\n")))
}

// SessionName is the tmux session for a project directory:
// <sanitized basename>-<first 4 hex of sha256(cwd)> (tmux forbids . and :).
func SessionName(cwd string) string {
	base := filepath.Base(filepath.Clean(cwd))
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" {
		name = "root"
	}
	if len(name) > 32 {
		name = name[:32]
	}
	sum := sha256.Sum256([]byte(cwd))
	return name + "-" + hex.EncodeToString(sum[:])[:4]
}

// Label is the pane border label: "Claude", or "Claude · opus" when the
// configured args pick a model.
func (w *WS) Label(id session.AgentID) string {
	name := session.ShortName(id)
	args := w.Cfg.Agent(string(id)).Args
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--model" || args[i] == "-m" {
			return name + " · " + args[i+1]
		}
	}
	return name
}

// Launch describes one agent run in a pane.
type Launch struct {
	Agent    session.AgentID
	CWD      string
	Prompt   string // initial prompt (a handoff); "" = none
	ResumeID string // resume this native session instead of starting a new one
	Handoff  string // handoff file that seeded the run (recorded in the trail)
	Pane     string // pane id when known (respawn); "" for a new pane
	// Before starts a new pane's trail (a conversation picked in History
	// that this run continues).
	Before []TrailEntry
}

// Spec builds the launch spec and the trail entry of a run.
func (w *WS) Spec(l Launch) (*launch.Spec, TrailEntry, error) {
	r := w.Bin(l.Agent)
	if !r.Installed {
		return nil, TrailEntry{}, &NotInstalledError{Agent: l.Agent}
	}
	ac := w.Cfg.Agent(string(l.Agent))
	tpl := ac.NewArgs
	vars := agent.Vars{Prompt: l.Prompt, UUID: agent.NewUUID()}
	if w.pasteMode(l) {
		vars.Prompt = "" // pasted after start (paste.go)
	}
	if l.ResumeID != "" {
		tpl, vars.ID = ac.ResumeArgs, l.ResumeID
	}
	var argv []string
	if l.Handoff != "" && w.Cfg.Handoff.Deliver != "inline" {
		argv = agent.ReadDirArgs(l.Agent, filepath.Dir(l.Handoff)) // the agent reads the file itself
	}
	if l.Agent == session.Claude && w.Cfg.Limits.ClaudeStatusline {
		if p, err := w.claudeSettings(); err == nil {
			argv = append(argv, "--settings="+p)
		}
	}
	argv = append(append(argv, ac.Args...), agent.Expand(tpl, vars)...)
	te := TrailEntry{Agent: l.Agent, Start: w.Now().UnixMilli(), Handoff: l.Handoff, Resumed: l.ResumeID != ""}
	switch {
	case l.ResumeID != "":
		te.SessionID = l.ResumeID
	case hasPlaceholder(tpl, "{uuid}"):
		// Pre-assigned (claude --session-id, opencode --session ses_imux…):
		// the argument that holds {uuid}, expanded.
		for _, a := range tpl {
			if strings.Contains(a, "{uuid}") {
				te.SessionID = strings.ReplaceAll(a, "{uuid}", vars.UUID)
				break
			}
		}
	}
	pathEnv := r.PathEnv
	if pathEnv == "" {
		pathEnv = os.Getenv("PATH")
	}
	spec := &launch.Spec{
		Agent: l.Agent, Bin: r.Path, Args: argv, Dir: l.CWD,
		Env: map[string]string{"PATH": pathEnv}, Pane: l.Pane, MaxArgBytes: w.Cfg.Handoff.MaxArgBytes,
	}
	if err := spec.Validate(); err != nil {
		return nil, TrailEntry{}, err
	}
	return spec, te, nil
}

func hasPlaceholder(tpl []string, p string) bool {
	for _, a := range tpl {
		if strings.Contains(a, p) {
			return true
		}
	}
	return false
}

// execCmd is the pane command that runs a spec.
func (w *WS) execCmd(specPath string) []string { return []string{w.Self, "__exec", specPath} }

// optCmds sets the pane options of a run. target "" means the pane created
// by the preceding command in the same batch.
func optCmds(target string, te TrailEntry, trail Trail, cwd, label string) []tmux.Cmd {
	set := func(k, v string) tmux.Cmd {
		c := tmux.Cmd{"set-option", "-p"}
		if target != "" {
			c = append(c, "-t", target)
		}
		return append(c, k, v)
	}
	unset := func(k string) tmux.Cmd {
		c := tmux.Cmd{"set-option", "-p", "-u"}
		if target != "" {
			c = append(c, "-t", target)
		}
		return append(c, k)
	}
	return []tmux.Cmd{
		set("remain-on-exit", "on"),
		set(OptAgent, string(te.Agent)),
		set(OptStart, strconv.FormatInt(te.Start, 10)),
		set(OptCWD, cwd),
		set(OptLabel, label),
		set(OptTrail, trail.String()),
		// The previous run's state does not carry over to the new one.
		unset(sidebar.OptState), unset(sidebar.OptStateAt), unset(sidebar.OptDoneAt),
	}
}

// Pane is the imux state of one pane.
type Pane struct {
	ID       string
	Agent    session.AgentID // "" in a plain shell
	Start    time.Time
	CWD      string // @imux_cwd, else the pane's current path
	Label    string
	Trail    Trail
	Fallback string
	Lock     string
	Dead     bool
	Session  string
	Window   string // window id
	Command  string // pane_current_command
	Role     string // @imux_role: "sidebar" for sidebar panes
	Name     string // @imux_name: the user's name for it
}

// paneFormat reads every pane field in one display-message.
const paneFormat = "#{pane_id}\t#{@imux_agent}\t#{@imux_start}\t#{@imux_cwd}\t#{pane_current_path}\t#{@imux_label}\t" +
	"#{pane_dead}\t#{session_name}\t#{window_id}\t#{pane_current_command}\t#{@imux_fallback}\t#{@imux_lock}\t#{@imux_trail}\t#{@imux_role}\t#{@imux_name}"

const paneFields = 15

func parsePane(f []string) *Pane {
	p := &Pane{
		ID: f[0], Agent: session.AgentID(f[1]), CWD: f[3], Label: f[5], Dead: f[6] == "1",
		Session: f[7], Window: f[8], Command: f[9], Fallback: f[10], Lock: f[11], Trail: ParseTrail(f[12]),
		Role: f[13], Name: f[14],
	}
	if ms, err := strconv.ParseInt(f[2], 10, 64); err == nil && ms > 0 {
		p.Start = time.UnixMilli(ms)
	}
	if p.CWD == "" {
		p.CWD = f[4]
	}
	return p
}

// Pane reads the state of pane id.
func (w *WS) Pane(ctx context.Context, id string) (*Pane, error) {
	out, err := w.T.Run(ctx, tmux.Cmd{"display-message", "-p", "-t", id, paneFormat})
	if err != nil {
		return nil, err
	}
	rows := tmux.Rows(out, paneFields)
	if len(rows) != 1 || rows[0][0] != id {
		return nil, fmt.Errorf("%w: %s", tmux.ErrNoPane, id)
	}
	return parsePane(rows[0]), nil
}

// Panes lists every pane on the server.
func (w *WS) Panes(ctx context.Context) ([]*Pane, error) {
	out, err := w.T.Run(ctx, tmux.Cmd{"list-panes", "-a", "-F", paneFormat})
	if err != nil {
		return nil, err
	}
	var ps []*Pane
	for _, r := range tmux.Rows(out, paneFields) {
		ps = append(ps, parsePane(r))
	}
	return ps, nil
}

// writeSpec stores the spec for __exec.
func writeSpec(s *launch.Spec) (string, error) { return launch.Write(s) }

// --- tmux.conf ---------------------------------------------------------------

// agentBar renders status-format[1] (T2.8a); "" disables the bar.
var agentBar = func(w *WS, v tmux.Version) string { return "" }

// ErrTmuxMissing means tmux is not installed.
var ErrTmuxMissing = errors.New("tmux missing")

// TmuxVersion checks that tmux is installed and recent enough.
func (w *WS) TmuxVersion(ctx context.Context) (tmux.Version, error) {
	v, err := w.T.BinVersion(ctx)
	if errors.Is(err, tmux.ErrNotFound) {
		return v, fmt.Errorf("%w: %s", ErrTmuxMissing, i18n.T("tmux.missing"))
	}
	if err != nil {
		return v, err
	}
	if !v.AtLeast(tmux.MinVersion[0], tmux.MinVersion[1]) {
		return v, errors.New(i18n.T("tmux.too_old", v.String(), fmt.Sprintf("%d.%d", tmux.MinVersion[0], tmux.MinVersion[1])))
	}
	return v, nil
}

// EnsureConf (re)writes ~/.imux/tmux.conf when its content changed and makes
// a running server reload it.
func (w *WS) EnsureConf(ctx context.Context) error {
	v, err := w.TmuxVersion(ctx)
	if err != nil {
		return err
	}
	if err := paths.Ensure(); err != nil {
		return err
	}
	in := &tmux.ConfInput{
		Self: w.Self, Cfg: w.Cfg, Version: v, Terminal: tmux.DefaultTerminal(),
		AgentBar: agentBar(w, v), LocalConf: paths.TmuxLocal(),
	}
	changed, err := tmux.WriteConf(paths.TmuxConf(), tmux.GenerateConf(in), paths.WriteFile)
	if err != nil {
		return err
	}
	if changed {
		return w.T.Reload(ctx, paths.TmuxConf())
	}
	return nil
}

// EnsureWatcher starts the limit watcher or wakes it (M3).
var EnsureWatcher = func(w *WS) {}

// claudeSettings writes the settings file Claude is started with: only a
// status line that runs imux (D25). It is rewritten when imux moved.
func (w *WS) claudeSettings() (string, error) {
	p := paths.ClaudeSettings()
	cmd := statuslineCmd(w.Self)
	b, err := json.Marshal(map[string]any{"statusLine": map[string]any{"type": "command", "command": cmd, "padding": 0}})
	if err != nil {
		return "", err
	}
	if old, err := os.ReadFile(p); err == nil && string(old) == string(b) {
		return p, nil
	}
	if err := paths.EnsureDir(paths.Home()); err != nil {
		return "", err
	}
	return p, paths.WriteFile(p, b)
}
