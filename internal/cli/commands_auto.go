package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/auto"
	"github.com/hamzayogurtcuoglu/i-mux/internal/autodialog"
	"github.com/hamzayogurtcuoglu/i-mux/internal/autoui"
	"github.com/hamzayogurtcuoglu/i-mux/internal/autoview"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/notify"
	"github.com/hamzayogurtcuoglu/i-mux/internal/pager"
	"github.com/hamzayogurtcuoglu/i-mux/internal/popmenu"
	"github.com/hamzayogurtcuoglu/i-mux/internal/quickpick"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:   "__auto",
		Args:   "menu item|pick history|confirm remove <id> | run|dialog|log|show|stop|pause|resume|remove|open|continue|exec <id> [--cwd D] [--client C] [--pane P] [--sidebar S]",
		Hidden: true,
		Run:    runAuto,
	})
}

// autoCmd is one `imux __auto` call: the Scheduler's Run panel, its output
// view, menus and Quick Picks, or a run (§9.3 "Scheduler", D61).
type autoCmd struct {
	e      *Env
	w      *workspace.WS
	cfg    *config.Config
	ctx    context.Context
	c      autoui.Ctx
	log    string    // --log: a run's log
	after  time.Time // --after: the run that started then or later
	popup  string    // --popup: the Run panel's popup (what runs once it closed)
	atWhen bool      // --at-when: the Run panel opens at the time
	manual bool
	now    time.Time
}

func autoUsage() error {
	return &UsageError{Msg: i18n.T("cli.usage", "__auto <verb> [name] <id>")}
}

func runAuto(e *Env, args []string) error {
	fs := newFlags(e, "__auto")
	cwd := fs.String("cwd", "", "the project root (default: @imux_auto_cwd)")
	client := fs.String("client", "", "the client")
	pane := fs.String("pane", "", "the pane the sidebar follows")
	sidebarF := fs.String("sidebar", "", "the sidebar's pane")
	x := fs.Int("x", 0, "the menu's column in the sidebar")
	y := fs.Int("y", 0, "the menu's row in the sidebar")
	logF := fs.String("log", "", "log, show: a run's log (its file name)")
	afterF := fs.Int64("after", 0, "log, show: the run that started at this time (unix ms) or later")
	popupF := fs.String("popup", "", "dialog: the popup's id")
	atWhen := fs.Bool("at-when", false, "run, dialog: the panel opens at the time")
	manual := fs.Bool("manual", false, "exec: started by hand")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 || *logF != "" && !validLog(*logF) || *popupF != "" && !popmenu.ValidID(*popupF) {
		return autoUsage()
	}
	verb, rest := pos[0], pos[1:]
	name := ""
	switch verb {
	case "menu", "pick", "confirm", "skill":
		if len(rest) == 0 {
			return autoUsage()
		}
		name, rest = rest[0], rest[1:]
	}
	if len(rest) != 1 || !auto.ValidID(rest[0]) {
		return autoUsage()
	}
	id := rest[0]
	w, cfg, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	self, err := workspace.Self()
	if err != nil {
		return err
	}
	a := &autoCmd{e: e, w: w, cfg: cfg, ctx: context.Background(), log: *logF, popup: *popupF, atWhen: *atWhen, manual: *manual, now: time.Now()}
	if *afterF > 0 {
		a.after = time.UnixMilli(*afterF)
	}
	a.c = autoui.Ctx{Self: self, Client: *client, Pane: *pane, Sidebar: *sidebarF, X: *x, Y: *y}
	if verb == "exec" {
		return a.exec(id)
	}
	// The UI's commands: the project is --cwd, else the one the last
	// command was given.
	if *cwd != "" {
		a.c.Dir = *cwd
		a.setOpt(autoui.OptCwd, *cwd)
	} else {
		a.c.Dir = a.opt(autoui.OptCwd)
	}
	err = a.do(verb, name, id)
	if err != nil {
		a.say(i18n.T("file.failed", err.Error()))
	}
	return err
}

var logRE = regexp.MustCompile(`^[0-9]{8}-[0-9]{6}\.log$`)

// validLog reports a run's log name (auto.LogName).
func validLog(name string) bool { return logRE.MatchString(name) }

func (a *autoCmd) opt(name string) string {
	out, _ := a.w.T.Run(a.ctx, tmux.Cmd{"show-options", "-gqv", name})
	return strings.TrimSpace(out)
}

// setOpt sets a global option ("" unsets it).
func (a *autoCmd) setOpt(name, v string) {
	if v == "" {
		_, _ = a.w.T.Run(a.ctx, tmux.Cmd{"set-option", "-gu", name})
		return
	}
	_, _ = a.w.T.Run(a.ctx, tmux.Cmd{"set-option", "-g", name, v})
}

func (a *autoCmd) say(msg string) {
	if a.c.Client != "" {
		a.w.Message(a.ctx, a.c.Client, "", msg)
	} else {
		fmt.Fprintln(a.e.Stderr, msg)
	}
}

func (a *autoCmd) refresh() { sidebar.RefreshAll(a.ctx, a.w.T) }

// target is what an id names: its definition (while it is there) and its
// schedule (when it has one).
type target struct {
	id    string
	root  string
	def   auto.Def
	found bool
	sched auto.Schedule
	has   bool
}

func homeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

func (a *autoCmd) find(id string) target {
	t := target{id: id, root: a.c.Dir}
	if ss, err := auto.Load(); err == nil {
		t.sched, t.has = auto.Get(ss, id)
	}
	if t.has && t.sched.Project != "" {
		t.root = t.sched.Project
	}
	if t.root != "" {
		t.def, t.found = auto.Find(auto.List(t.root, homeDir()), id)
	}
	if a.c.Dir == "" {
		a.c.Dir = t.root
	}
	return t
}

func (t target) name() string {
	switch {
	case t.found:
		return t.def.Name
	case t.has:
		return t.sched.Name
	}
	return t.id
}

// scheduled reports a target with a time (not only by hand).
func (t target) scheduled() bool { return t.has && !t.sched.Parsed().Manual() }

// change changes t's schedule, made from its definition when it has none.
func (a *autoCmd) change(t target, fn func(*auto.Schedule)) (auto.Schedule, error) {
	var d *auto.Def
	if t.found {
		d = &t.def
	}
	return auto.Change(t.id, d, t.root, a.now, fn)
}

func (a *autoCmd) gone(t target) error {
	a.say(i18n.T("auto.gone", t.name()))
	return nil
}

// cliLabel is "Claude · haiku".
func cliLabel(cli session.AgentID, model string) string {
	s := session.ShortName(cli)
	if model != "" {
		s += " · " + model
	}
	return s
}

func (a *autoCmd) do(verb, name, id string) error {
	switch verb {
	case "menu":
		if name != "item" {
			return autoUsage()
		}
		return a.showMenu(autoui.ItemMenu(a.c, a.item(a.find(id))))
	case "pick":
		if name != "history" {
			return autoUsage()
		}
		return a.showPick(autoui.HistoryPick(a.c, id, a.find(id).name(), auto.Runs(id), a.now))
	case "confirm":
		if name != "remove" {
			return autoUsage()
		}
		t := a.find(id)
		return a.showMenu(autoui.ConfirmRemove(a.c, id, t.name(), t.scheduled()))
	case "run":
		return a.panel(id)
	case "dialog":
		return a.dialog(id)
	case "log":
		return a.logPopup(id)
	case "show":
		return a.show(id)
	case "open":
		return a.open(id)
	case "continue":
		return a.cont(id)
	case "skill":
		return a.skill(name, id)
	}
	t := a.find(id)
	switch verb {
	case "stop":
		if auto.Stop(id) {
			a.say(i18n.T("auto.stopped_msg", t.name()))
		}
	case "pause", "resume":
		if !t.has {
			return nil
		}
		if _, err := a.change(t, func(s *auto.Schedule) { s.Paused = verb == "pause" }); err != nil {
			return err
		}
		if verb == "pause" {
			a.say(i18n.T("auto.paused", t.name()))
		} else {
			a.say(i18n.T("auto.resumed", t.name()))
		}
	case "remove":
		auto.Stop(id)
		if err := auto.Remove(id); err != nil {
			return err
		}
		if t.scheduled() {
			a.say(i18n.T("auto.removed", t.name()))
		} else {
			a.say(i18n.T("auto.forgotten", t.name()))
		}
	default:
		return autoUsage()
	}
	a.refresh()
	return nil
}

// showMenu shows a menu: at the pointer (x, y in the sidebar) or centered.
func (a *autoCmd) showMenu(m *workspace.Menu) error {
	if a.c.X > 0 && a.c.Y > 0 && a.c.Sidebar != "" {
		c := m.Cmd(a.c.Client, a.c.Sidebar)
		px, py := sidebar.MenuAt(a.c.X, a.c.Y)
		for i := 0; i+1 < len(c) && c[i] != "--"; i++ {
			switch c[i] {
			case "-x":
				c[i+1] = px
			case "-y":
				c[i+1] = py
			}
		}
		return a.w.T.ShowMenu(a.ctx, append(a.w.T.Menu(a.ctx), c[1:]...))
	}
	return a.w.Show(a.ctx, m, a.c.Client, a.c.Pane)
}

// clientSize is the client's size (0, 0 when unknown).
func (a *autoCmd) clientSize() (int, int) {
	cw, ch := 0, 0
	if a.c.Client != "" {
		out, err := a.w.T.Run(a.ctx, tmux.Cmd{"display-message", "-p", "-c", a.c.Client, "#{client_width}\t#{client_height}"})
		if err == nil {
			_, _ = fmt.Sscanf(strings.TrimSpace(out), "%d\t%d", &cw, &ch)
		}
	}
	return cw, ch
}

func (a *autoCmd) showPick(p *quickpick.Pick) error {
	cw, ch := a.clientSize()
	return quickpick.Show(a.ctx, a.w.T, a.c.Self, p, cw, ch)
}

// item is what t's menu shows.
func (a *autoCmd) item(t target) autoui.Item {
	runs := auto.Runs(t.id)
	_, running := auto.Running(t.id)
	x := autoui.Item{ID: t.id, Name: t.name(), Record: t.has, Scheduled: t.scheduled(), Running: running, Runs: len(runs), Gone: !t.found}
	x.Paused = t.has && t.sched.Paused
	x.Skill = t.found && t.def.Kind == auto.KindSkill
	x.Shared = x.Skill && t.def.Shared
	x.Continue = len(runs) > 0 && canContinue(runs[0])
	return x
}

// skill runs a skill's action from its Jobs menu (D66): `imux __skill op`
// on the skill, the project and the pane, given as the sidebar's skill
// menus give them (the @imux_skill* options).
func (a *autoCmd) skill(op, id string) error {
	switch op {
	case "add", "improve", "share", "delete":
	default:
		return autoUsage()
	}
	t := a.find(id)
	if !t.found || t.def.Kind != auto.KindSkill {
		return a.gone(t)
	}
	if _, err := a.w.T.Run(a.ctx,
		tmux.Cmd{"set-option", "-g", sidebar.OptSkill, filepath.Dir(t.def.File)},
		tmux.Cmd{"set-option", "-g", sidebar.OptSkillRoot, t.root},
		tmux.Cmd{"set-option", "-g", sidebar.OptSkillPane, a.c.Pane}); err != nil {
		return err
	}
	args := []string{op}
	if a.c.Client != "" {
		args = append(args, "--client", a.c.Client)
	}
	_ = runSkill(a.e, args) // it says what failed
	return nil
}

// canContinue reports a run whose conversation an agent pane can go on
// with (`imux split --from-session`).
func canContinue(r auto.Run) bool {
	switch session.AgentID(r.CLI) {
	case session.Claude, session.Copilot, session.Antigravity:
		return r.Session != ""
	}
	return false
}

// panel opens the Run panel in a popup sized to it (one that runs shows its
// output instead). Once the panel closed, what it chose to show next runs
// (`imux __quickpick --run`).
func (a *autoCmd) panel(id string) error {
	t := a.find(id)
	if !t.found {
		return a.gone(t)
	}
	if _, running := auto.Running(id); running && !a.atWhen {
		return a.logPopup(id)
	}
	pid, err := popmenu.NewID()
	if err != nil {
		return err
	}
	cw, ch := a.clientSize()
	if cw <= 0 || ch <= 0 {
		cw, ch = 100, 30
	}
	w := min(max(cw-6, 40), 92)
	h := min(autodialog.Height(a.panelOptions(t)), max(ch-2, 12))
	args := []string{a.c.Self, "__auto", "dialog", id, "--popup", pid}
	if a.c.Client != "" {
		args = append(args, "--client", a.c.Client)
	}
	if a.c.Pane != "" {
		args = append(args, "--pane", a.c.Pane)
	}
	if a.atWhen {
		args = append(args, "--at-when")
	}
	after := tmux.Cmd{"run-shell", "-b", tmux.EscapeFormat(tmux.ShJoin(a.c.Self, "__quickpick", "--run", pid))}
	return popmenu.Popup(a.ctx, a.w.T, a.c.Client, a.c.Pane, w, h, "C", "C", tmux.ShJoin(args...), after)
}

// panelOptions fill the Run panel for t: the installed CLIs (its own
// first), and what it ran with last.
func (a *autoCmd) panelOptions(t target) autodialog.Options {
	var clis []autodialog.CLI
	for _, ag := range agent.Ordered(a.cfg) {
		if a.w.Installed(ag.ID) {
			clis = append(clis, autodialog.CLI{ID: ag.ID, Name: ag.Short, Key: ag.Key, Limited: a.w.LimitedUntil(ag.ID)})
		}
	}
	sort.SliceStable(clis, func(i, j int) bool { return clis[i].ID == t.def.Owner && clis[j].ID != t.def.Owner })
	o := autodialog.Options{
		Name: t.name(), Description: t.def.Description, Project: filepath.Base(t.root), Skill: t.def.Kind == auto.KindSkill,
		CLIs: clis, AgentModel: t.def.Model, Owner: t.def.Owner, AtWhen: a.atWhen,
		Used: map[session.AgentID][]string{}, LastStart: auto.LastStart(t.id), Now: a.now,
	}
	var order []session.AgentID
	for _, c := range clis {
		order = append(order, c.ID)
	}
	o.CLI = auto.DefaultCLI(t.def, order, a.w.Installed)
	if t.has {
		if t.sched.CLI != "" {
			o.CLI = session.AgentID(t.sched.CLI)
		}
		o.Model, o.ReadOnly, o.Created = t.sched.Model, t.sched.ReadOnly(), t.sched.Created
		if t.scheduled() {
			o.When = t.sched.Parsed().String()
		}
	}
	if ss, err := auto.Load(); err == nil {
		for _, s := range ss {
			id := session.AgentID(s.CLI)
			if s.Model != "" && !containsStr(o.Used[id], s.Model) {
				o.Used[id] = append(o.Used[id], s.Model)
			}
		}
	}
	return o
}

func containsStr(l []string, s string) bool {
	for _, v := range l {
		if v == s {
			return true
		}
	}
	return false
}

// models lists a CLI's models for the Run panel (cached a day).
func (a *autoCmd) models(id session.AgentID) []string {
	bin := a.w.Bin(id)
	var out []string
	for _, m := range auto.Models(a.ctx, id, binPath(bin, id), bin.PathEnv, time.Now()) {
		out = append(out, m.ID)
	}
	return out
}

func binPath(r agent.Resolved, id session.AgentID) string {
	if r.Path != "" {
		return r.Path
	}
	return agent.MustGet(id).Bin
}

// dialog is the Run panel on this terminal (the popup). What it chose is
// kept; a run it starts (now, or a schedule that is due) opens its output
// once the popup closed.
func (a *autoCmd) dialog(id string) error {
	t := a.find(id)
	if !t.found {
		return a.gone(t)
	}
	m := autodialog.New(a.panelOptions(t))
	res, err := autodialog.Run(m, a.models, os.Stdin, os.Stdout)
	if err != nil || !res.Run {
		return err
	}
	access := auto.AccessFull
	if res.ReadOnly {
		access = auto.AccessRead
	}
	s, err := a.change(t, func(s *auto.Schedule) {
		s.CLI, s.Model, s.Access = string(res.CLI), res.Model, access
		if res.When != "" {
			s.When, s.Paused = res.When, false
		}
	})
	if err != nil {
		return err
	}
	at := time.Now()
	who := cliLabel(res.CLI, res.Model)
	started := false
	if res.When == "" {
		if err := auto.Launch(a.c.Self, id, true); err != nil {
			return err
		}
		started = true
		a.say(i18n.T("auto.started", t.name(), who))
	} else {
		started = a.startIfDue(s)
		a.say(i18n.T("auto.scheduled", t.name(), s.Parsed().Label(), who))
	}
	a.refresh()
	if !started || a.popup == "" {
		return nil
	}
	argv := []string{"__auto", "log", id, "--after", strconv.FormatInt(at.UnixMilli(), 10)}
	if a.c.Client != "" {
		argv = append(argv, "--client", a.c.Client)
	}
	if a.c.Pane != "" {
		argv = append(argv, "--pane", a.c.Pane)
	}
	return quickpick.Later(a.popup, argv)
}

// startIfDue starts s when it is due now (an interval schedule at once);
// it reports whether it did.
func (a *autoCmd) startIfDue(s auto.Schedule) bool {
	w := s.Parsed()
	if s.Paused || w.Manual() {
		return false
	}
	if due, _ := w.Due(s.Created, auto.LastStart(s.ID), a.now); !due {
		return false
	}
	if _, running := auto.Running(s.ID); running {
		return false
	}
	return auto.Launch(a.c.Self, s.ID, false) == nil
}

// logPopup shows a run's output (the one going on, the last one, --log or
// the one started --after) in a popup.
func (a *autoCmd) logPopup(id string) error {
	t := a.find(id)
	args := []string{a.c.Self, "__auto", "show", id}
	if a.log != "" {
		args = append(args, "--log", a.log)
	}
	if !a.after.IsZero() {
		args = append(args, "--after", strconv.FormatInt(a.after.UnixMilli(), 10))
	}
	c := tmux.Cmd{"display-popup"}
	if a.c.Client != "" {
		c = append(c, "-c", a.c.Client)
	}
	c = append(c, "-E", "-w", "90%", "-h", "85%")
	if t.root != "" {
		c = append(c, "-d", tmux.EscapeFormat(t.root))
	}
	return a.w.T.Spawn(append(c, tmux.ShJoin(args...)))
}

// startWait is how long the output view waits for a run it was opened for
// to start.
var startWait = 20 * time.Second

// outputView is the output view of one run: it reads its log as it grows.
type outputView struct {
	id, name string
	log      string    // the run's log ("" until the run is found)
	after    time.Time // the run started then or later (zero: the last one)
	opened   time.Time
	run      *auto.Run // how it ended (nil while it runs)
	cur      auto.Current
	out      *auto.Output
	off      int64
	tick     int
	gaveUp   bool
	p        *pager.Pager
}

func (a *autoCmd) newRunView(id string) *outputView {
	t := a.find(id)
	v := &outputView{id: id, name: t.name(), log: a.log, after: a.after, opened: time.Now(), out: &auto.Output{Dir: t.root}}
	if v.log != "" {
		for _, r := range auto.Runs(id) {
			if r.Log == v.log {
				r := r
				v.run = &r
			}
		}
		if c, ok := auto.Running(id); ok && c.Log == v.log {
			v.cur = c
		}
	}
	return v
}

// find finds the run to show: the one going on, else the last one; with
// --after, only one that started then or later.
func (v *outputView) find() {
	recent := func(t time.Time) bool { return v.after.IsZero() || !t.Before(v.after.Add(-2*time.Second)) }
	if c, ok := auto.Running(v.id); ok && c.Log != "" && recent(c.Start) {
		v.log, v.cur = c.Log, c
		return
	}
	if runs := auto.Runs(v.id); len(runs) > 0 && recent(runs[0].Start) {
		r := runs[0]
		v.run, v.log = &r, r.Log
		return
	}
	if !v.after.IsZero() && time.Since(v.opened) > startWait {
		v.gaveUp = true
	}
}

// read reads what the log got since the last read.
func (v *outputView) read() {
	if v.log == "" {
		return
	}
	f, err := os.Open(auto.LogPath(v.id, v.log))
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Seek(v.off, io.SeekStart); err != nil {
		return
	}
	b, _ := io.ReadAll(io.LimitReader(f, 16<<20))
	v.off += int64(len(b))
	v.out.Feed(b)
}

// live is the pager's Live: the header and the output for w columns, and
// whether more is to come.
func (v *outputView) live(w int) ([]string, []string, bool) {
	v.tick++
	if v.run == nil && v.log == "" && !v.gaveUp {
		v.find()
	}
	v.read()
	if v.run == nil && v.log != "" {
		if c, ok := auto.Running(v.id); ok && (c.Log == v.log || c.Log == "") {
			if c.Log != "" {
				v.cur = c
			}
		} else {
			for _, r := range auto.Runs(v.id) {
				if r.Log == v.log {
					r := r
					v.run = &r
					break
				}
			}
			if v.run != nil {
				v.read()
				v.out.Flush()
			}
		}
	}
	h := autoview.Run{Name: v.name, Tick: v.tick, Result: v.run, GaveUp: v.gaveUp}
	switch {
	case v.run != nil:
		h.CLI, h.Model, h.Command, h.Start = session.AgentID(v.run.CLI), firstNonEmpty(v.run.Model, v.out.Model), v.run.Command, v.run.Start
	case v.cur.Log != "":
		h.CLI, h.Model, h.Command, h.Start = session.AgentID(v.cur.CLI), firstNonEmpty(v.cur.Model, v.out.Model), v.cur.Command, v.cur.Start
	default:
		h.Starting = true
	}
	more := v.run == nil && !v.gaveUp
	header := autoview.Header(h, time.Now(), w)
	lines := autoview.Lines(v.out, w)
	if len(lines) == 0 {
		msg := i18n.T("auto.no_output")
		if more {
			msg = i18n.T("auto.view.waiting")
		}
		lines = []string{" " + theme.S().Faint + msg + "\x1b[0m"}
	}
	if v.p != nil {
		v.p.Hint = autoview.Hint(more)
	}
	return header, lines, more
}

// stop stops the run shown.
func (v *outputView) stop() string {
	if v.run != nil || v.gaveUp {
		return ""
	}
	if auto.Stop(v.id) {
		return i18n.T("auto.view.stopped")
	}
	return ""
}

// show is the output view on this terminal (a popup): it follows the run
// as it goes; s stops it. Not on a terminal, the output is printed.
func (a *autoCmd) show(id string) error {
	v := a.newRunView(id)
	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		h, ls, _ := v.live(100)
		for _, l := range append(h, ls...) {
			fmt.Fprintln(a.e.Stdout, strings.TrimRight(sidebar.Plain(l), " "))
		}
		return nil
	}
	p := &pager.Pager{Live: v.live, Keys: map[string]func() string{"s": v.stop}}
	v.p = p
	return pager.Run(p, os.Stdin, os.Stdout)
}

// open edits the definition in a popup in the project.
func (a *autoCmd) open(id string) error {
	t := a.find(id)
	if !t.found {
		return a.gone(t)
	}
	p := t.def.File
	if r, err := filepath.Rel(t.root, p); err == nil && !strings.HasPrefix(r, "..") {
		p = r
	}
	c := tmux.Cmd{"display-popup"}
	if a.c.Client != "" {
		c = append(c, "-c", a.c.Client)
	}
	return a.w.T.Spawn(append(c, "-E", "-w", "90%", "-h", "90%", "-d", tmux.EscapeFormat(t.root), tmux.ShJoin(a.c.Self, "__edit", p)))
}

// cont continues the last run's conversation in a pane beside the one the
// sidebar follows, with the CLI it ran with.
func (a *autoCmd) cont(id string) error {
	runs := auto.Runs(id)
	if len(runs) == 0 || !canContinue(runs[0]) {
		a.say(i18n.T("auto.no_session"))
		return nil
	}
	if a.c.Pane == "" {
		return errors.New("no pane chosen")
	}
	r := runs[0]
	args := []string{"split", r.CLI, "-h", "--from-session", r.CLI + ":" + r.Session, "--pane", a.c.Pane}
	if a.c.Client != "" {
		args = append(args, "--client", a.c.Client)
	}
	return exec.Command(a.c.Self, args...).Run()
}

// exec runs schedule id (`imux __auto exec`, started on its own by the
// watcher or the Run panel): its definition with its CLI and model in its
// project, headless, holding its run lock; then records the run (its
// answer read from the CLI's stream), redraws the sidebars and notifies.
func (a *autoCmd) exec(id string) error {
	ss, err := auto.Load()
	if err != nil {
		return err
	}
	s, ok := auto.Get(ss, id)
	if !ok {
		return fmt.Errorf("no schedule %s", id)
	}
	ex, err := auto.Begin(id)
	if errors.Is(err, auto.ErrRunning) {
		return nil
	}
	if err != nil {
		return err
	}
	// Started on schedule twice at once (the watcher, and the Run panel
	// starting it at once): the second, which waited for the first's lock,
	// is not due any more.
	if due, _ := s.Parsed().Due(s.Created, auto.LastStart(id), time.Now()); !a.manual && (!due || s.Paused) {
		ex.Abort()
		return nil
	}
	r := auto.Run{Start: time.Now(), Manual: a.manual, CLI: s.CLI, Model: s.Model}
	finish := func() error {
		r.End = time.Now()
		prev := auto.Last(id)
		err := ex.Finish(r)
		a.refresh()
		a.tell(s, r, prev)
		return err
	}
	skip := func(why string) error {
		r.Status, r.Summary = auto.StatusSkipped, why
		return finish()
	}
	d, found := auto.Find(auto.List(s.Project, homeDir()), id)
	if !found {
		return skip(i18n.T("auto.gone_short"))
	}
	cli := session.AgentID(s.CLI)
	if cli == "" {
		var order []session.AgentID
		for _, ag := range agent.Ordered(a.cfg) {
			order = append(order, ag.ID)
		}
		cli = auto.DefaultCLI(d, order, a.w.Installed)
		r.CLI = string(cli)
	}
	if _, ok := agent.Get(cli); !ok {
		return skip(i18n.T("auto.not_installed", string(cli)))
	}
	bin := a.w.Bin(cli)
	if !bin.Installed {
		return skip(i18n.T("auto.not_installed", session.ShortName(cli)))
	}
	if l := a.w.State().ActiveLimit(string(cli), a.w.Now()); l != nil && l.Blocking() {
		return skip(i18n.T("auto.limited", session.ShortName(cli), a.w.LimitedUntil(cli)))
	}
	var prev *auto.Run
	for _, old := range auto.Runs(id) {
		if old.Report != "" {
			prev = &old
			break
		}
	}
	prompt := auto.Prompt(auto.PromptInput{
		Def: d, CLI: cli, When: s.Parsed().English(), Dir: s.Project, Prev: prev,
		Lang: i18n.T("skills.language"), Now: r.Start, Manual: a.manual,
	})
	uuid, outFile := "", ""
	switch cli {
	case session.Claude:
		uuid = agent.NewUUID()
	case session.Codex:
		outFile = filepath.Join(auto.Dir(id), "reply.txt")
		_ = os.Remove(outFile)
	}
	inv, err := auto.Command(cli, binPath(bin, cli), d, s.Model, prompt, s.ReadOnly(), outFile, uuid)
	if err != nil {
		return skip(err.Error())
	}
	r.Log, r.Session, r.Command = auto.LogName(r.Start), inv.Session, auto.Shown(inv.Argv)
	o := auto.Execute(a.ctx, inv.Argv, autoEnv(bin.PathEnv), s.Project, auto.LogPath(id, r.Log), s.RunTimeout(), func(pgid int) {
		ex.Started(auto.Current{Start: r.Start, PGID: pgid, Log: r.Log, CLI: string(cli), Model: s.Model, Command: r.Command, Manual: a.manual})
		a.refresh()
	})
	out := auto.ParseOutput(o.Stdout, s.Project)
	r.Exit, r.Session = o.Exit, firstNonEmpty(out.Session, r.Session)
	switch {
	case ex.Stopped():
		r.Status = auto.StatusStopped
	case o.TimedOut:
		r.Status, r.Summary = auto.StatusTimeout, i18n.T("auto.timed_out_after", auto.Took(s.RunTimeout()))
	case o.Err != nil:
		r.Status, r.Summary = auto.StatusFailed, auto.FirstLine(o.Err.Error())
	case o.Exit != 0 || out.Failed:
		r.Status = auto.StatusFailed
		r.Summary = firstNonEmpty(auto.FirstLine(out.Error), lastLine(o.Stderr))
		if !out.Streamed {
			r.Summary = firstNonEmpty(r.Summary, auto.FirstLine(o.Stdout))
		}
		if r.Summary == "" {
			r.Summary = i18n.T("auto.failed", o.Exit)
		}
	default:
		report := o.Stdout
		if out.Streamed {
			report = out.Report
		} else if inv.OutFile != "" {
			if b, err := os.ReadFile(inv.OutFile); err == nil && len(strings.TrimSpace(string(b))) > 0 {
				report = string(b)
			}
		}
		report = auto.Clean(report)
		sum, quiet := auto.Summarize(report)
		r.Status, r.Summary = auto.StatusOK, sum
		if quiet {
			r.Status = auto.StatusQuiet
		} else {
			r.Report = report
		}
	}
	return finish()
}

// lastLine is the last non-empty line of s (an error's message usually
// ends the output).
func lastLine(s string) string {
	ls := strings.Split(auto.Clean(s), "\n")
	for i := len(ls) - 1; i >= 0; i-- {
		if t := strings.TrimSpace(ls[i]); t != "" {
			return auto.FirstLine(t)
		}
	}
	return ""
}

// autoEnv is a run's environment: i-mux's, without tmux's (it runs on its
// own), with the agent CLI's PATH and no colors.
func autoEnv(pathEnv string) []string {
	var env []string
	for _, kv := range os.Environ() {
		switch k, _, _ := strings.Cut(kv, "="); k {
		case "TMUX", "TMUX_PANE", "NO_COLOR":
			continue
		case "PATH":
			if pathEnv != "" {
				continue
			}
		}
		env = append(env, kv)
	}
	if pathEnv != "" {
		env = append(env, "PATH="+pathEnv)
	}
	return append(env, "NO_COLOR=1")
}

// tell notifies about a run (§9.3 "Notifications"): a report, or a failure
// after one that did not fail; on every client's status line, and the
// system's notification when [notify] is on.
func (a *autoCmd) tell(s auto.Schedule, r auto.Run, prev *auto.Run) {
	switch {
	case r.Status == auto.StatusOK:
	case r.Status.Failed() && (prev == nil || !prev.Status.Failed()):
	default:
		return
	}
	project := filepath.Base(s.Project)
	title := i18n.T("auto.notify_ok", s.Name, project)
	if r.Status.Failed() {
		title = i18n.T("auto.notify_failed", s.Name, project)
	}
	body := r.Text()
	out, err := a.w.T.Run(a.ctx,
		tmux.DelimCmd("clients"), tmux.Cmd{"list-clients", "-F", "#{client_tty}\t#{client_name}"},
		tmux.DelimCmd("term"), tmux.Cmd{"show-options", "-gqv", "@imux_terminal"})
	if err != nil {
		return
	}
	sec := tmux.Sections(out)
	rows := tmux.Rows(sec["clients"], 2)
	for _, c := range rows {
		_, _ = a.w.T.Run(a.ctx, tmux.Cmd{"display-message", "-c", c[1], tmux.EscapeMessage("🔔 " + title + ": " + body)})
	}
	if !a.cfg.Notify.Enabled {
		return
	}
	n := &notify.Notifier{Command: strings.Fields(a.cfg.Notify.Command)}
	m := notify.Message{Title: title, Body: body, Sound: a.cfg.Notify.Sound}
	if _, err := n.System(a.ctx, m); err != nil {
		var clients []notify.Client
		for _, c := range rows {
			clients = append(clients, notify.Client{TTY: c[0], Program: strings.TrimSpace(sec["term"])})
		}
		n.Terminals(clients, m)
	}
}
