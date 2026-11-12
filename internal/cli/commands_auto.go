package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/auto"
	"github.com/hamzayogurtcuoglu/i-mux/internal/autoui"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/notify"
	"github.com/hamzayogurtcuoglu/i-mux/internal/quickpick"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/textfmt"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:   "__auto",
		Args:   "menu|pick|ask|confirm <name> <id> | when|cli|model|run|stop|pause|resume|access|remove|log|show|open|continue|exec <id> [value] [--cwd D] [--client C] [--pane P] [--sidebar S]",
		Hidden: true,
		Run:    runAuto,
	})
}

// autoCmd is one `imux __auto` call: the Scheduler's menus, Quick Picks,
// prompt and popups, or a run (§9.3 "Scheduler", D61).
type autoCmd struct {
	e       *Env
	w       *workspace.WS
	cfg     *config.Config
	ctx     context.Context
	c       autoui.Ctx
	flow    string
	when    string // the time chosen so far (--when)
	cli     string // the agent CLI chosen so far (--cli)
	initial string // what was typed in the model Quick Pick
	log     string // a run's log (--log)
	fromOpt bool
	manual  bool
	now     time.Time
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
	flow := fs.String("flow", autoui.FlowNew, "what the menus ask: new, run, time or cli")
	when := fs.String("when", "", "the time chosen so far")
	cliF := fs.String("cli", "", "the agent CLI chosen so far")
	initial := fs.String("initial", "", "what was typed in the Quick Pick")
	logF := fs.String("log", "", "a run's log (its file name)")
	fromOpt := fs.Bool("from-option", false, "the answer is in @imux_auto_text")
	manual := fs.Bool("manual", false, "exec: started by hand")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) == 0 || !autoui.ValidFlow(*flow) || *logF != "" && !validLog(*logF) {
		return autoUsage()
	}
	verb, rest := pos[0], pos[1:]
	name := ""
	switch verb {
	case "menu", "pick", "ask", "confirm":
		if len(rest) == 0 {
			return autoUsage()
		}
		name, rest = rest[0], rest[1:]
	}
	id, arg := "", ""
	if verb != "menu" || name != "root" {
		if len(rest) == 0 || !auto.ValidID(rest[0]) {
			return autoUsage()
		}
		id = rest[0]
		if len(rest) > 1 {
			arg = rest[1]
		}
	}
	w, cfg, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	self, err := workspace.Self()
	if err != nil {
		return err
	}
	a := &autoCmd{
		e: e, w: w, cfg: cfg, ctx: context.Background(), flow: *flow, when: strings.ReplaceAll(*when, ",", " "),
		cli: *cliF, initial: *initial, log: *logF, fromOpt: *fromOpt, manual: *manual, now: time.Now(),
	}
	a.c = autoui.Ctx{Self: self, Client: *client, Pane: *pane, Sidebar: *sidebarF, X: *x, Y: *y}
	switch verb {
	case "exec":
		return a.exec(id)
	case "show":
		return a.showLog(id)
	}
	// The UI's commands: the project is --cwd, else the one the last
	// command was given.
	if *cwd != "" {
		a.c.Dir = *cwd
		a.setOpt(autoui.OptCwd, *cwd)
	} else {
		a.c.Dir = a.opt(autoui.OptCwd)
	}
	err = a.do(verb, name, id, arg)
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

func (a *autoCmd) do(verb, name, id, arg string) error {
	switch verb {
	case "menu":
		return a.menu(name, id)
	case "pick":
		return a.pick(name, id)
	case "ask":
		if name != "when" {
			return autoUsage()
		}
		return a.ask(id)
	case "confirm":
		if name != "remove" {
			return autoUsage()
		}
		return a.showMenu(autoui.ConfirmRemove(a.c, id, a.find(id).name()))
	case "when":
		return a.setWhen(id, arg)
	case "cli":
		return a.setCLI(id, arg)
	case "model":
		return a.setModel(id, arg)
	case "run":
		return a.run(id)
	case "log":
		return a.logPopup(id)
	case "open":
		return a.open(id)
	case "continue":
		return a.cont(id)
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
	case "access":
		if arg != auto.AccessFull && arg != auto.AccessRead {
			return autoUsage()
		}
		if _, err := a.change(t, func(s *auto.Schedule) { s.Access = arg }); err != nil {
			return err
		}
		if arg == auto.AccessRead {
			a.say(i18n.T("auto.access_read_set", t.name()))
		} else {
			a.say(i18n.T("auto.access_full_set", t.name()))
		}
	case "remove":
		auto.Stop(id)
		if err := auto.Remove(id); err != nil {
			return err
		}
		a.say(i18n.T("auto.removed", t.name()))
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

func (a *autoCmd) showPick(p *quickpick.Pick) error {
	cw, ch := 0, 0
	if a.c.Client != "" {
		out, err := a.w.T.Run(a.ctx, tmux.Cmd{"display-message", "-p", "-c", a.c.Client, "#{client_width}\t#{client_height}"})
		if err == nil {
			_, _ = fmt.Sscanf(strings.TrimSpace(out), "%d\t%d", &cw, &ch)
		}
	}
	return quickpick.Show(a.ctx, a.w.T, a.c.Self, p, cw, ch)
}

func (a *autoCmd) menu(name, id string) error {
	if name == "root" {
		// ✨ New agent… reads the project and the pane from the skill
		// options, as ✨ New skill… does.
		_, _ = a.w.T.Run(a.ctx,
			tmux.Cmd{"set-option", "-g", sidebar.OptSkillRoot, a.c.Dir},
			tmux.Cmd{"set-option", "-g", sidebar.OptSkillPane, a.c.Pane})
		return a.showMenu(autoui.RootMenu(a.c))
	}
	t := a.find(id)
	switch name {
	case "item":
		return a.showMenu(autoui.ItemMenu(a.c, a.item(t)))
	case "when":
		cur := ""
		if t.has {
			cur = t.sched.Parsed().String()
		}
		return a.showMenu(autoui.WhenMenu(a.c, id, t.name(), a.flow, cur))
	case "cli":
		if !t.found {
			return a.gone(t)
		}
		return a.showMenu(autoui.CLIMenu(a.c, id, t.name(), a.flow, a.when, a.clis(t)))
	case "access":
		return a.showMenu(autoui.AccessMenu(a.c, id, t.name(), t.has && t.sched.ReadOnly()))
	}
	return autoUsage()
}

// item is what t's menu shows.
func (a *autoCmd) item(t target) autoui.Item {
	runs := auto.Runs(t.id)
	_, running := auto.Running(t.id)
	x := autoui.Item{ID: t.id, Name: t.name(), Scheduled: t.has, Running: running, Runs: len(runs), Gone: !t.found}
	if t.has {
		x.Manual, x.Paused, x.ReadOnly = t.sched.Parsed().Manual(), t.sched.Paused, t.sched.ReadOnly()
	}
	x.Session = len(runs) > 0 && runs[0].Session != "" && runs[0].CLI == string(session.Claude)
	return x
}

// clis are the agent CLIs to run t with: t's own first.
func (a *autoCmd) clis(t target) []autoui.CLI {
	var cur session.AgentID
	if t.has && a.flow == autoui.FlowCLI {
		cur = session.AgentID(t.sched.CLI)
	}
	var out []autoui.CLI
	for _, ag := range agent.Ordered(a.cfg) {
		out = append(out, autoui.CLI{
			ID: ag.ID, Name: ag.Short, Key: ag.Key, Installed: a.w.Installed(ag.ID), Limited: a.w.LimitedUntil(ag.ID),
			Own: t.def.Owner == ag.ID, Current: ag.ID == cur,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Own && !out[j].Own })
	return out
}

func (a *autoCmd) pick(name, id string) error {
	t := a.find(id)
	switch name {
	case "model":
		return a.modelPick(t)
	case "history":
		return a.showPick(autoui.HistoryPick(a.c, id, t.name(), auto.Runs(id), a.now))
	}
	return autoUsage()
}

// modelPick asks which model a.cli runs t with.
func (a *autoCmd) modelPick(t target) error {
	if !t.found {
		return a.gone(t)
	}
	cli, err := parseAgentFlag("cli", a.cli)
	if err != nil || cli == "" {
		return autoUsage()
	}
	bin := a.w.Bin(cli)
	if !bin.Installed {
		a.say(i18n.T("auto.not_installed", session.ShortName(cli)))
		return nil
	}
	models := auto.Models(a.ctx, cli, binPath(bin, cli), bin.PathEnv, a.now)
	var used []string
	if ss, err := auto.Load(); err == nil {
		for _, s := range ss {
			if s.CLI == string(cli) && s.Model != "" {
				used = append(used, s.Model)
			}
		}
	}
	note := i18n.T("auto.model_cli", session.ShortName(cli))
	if t.def.Model != "" && auto.Native(t.def, cli) {
		note = i18n.T("auto.model_agent", t.def.Model)
	}
	c := autoui.CLI{ID: cli, Name: session.ShortName(cli), Installed: true}
	return a.showPick(autoui.ModelPick(a.c, t.id, t.name(), a.flow, a.when, c, models, used, note))
}

func binPath(r agent.Resolved, id session.AgentID) string {
	if r.Path != "" {
		return r.Path
	}
	return agent.MustGet(id).Bin
}

// ask opens the client's command prompt for a time of one's own. The
// answer reaches imux through OptText ("%%%" escapes it for tmux); the
// template carries no pane id (it would read "%1" as the answer).
func (a *autoCmd) ask(id string) error {
	then := []string{"__auto", "when", id, "--from-option", "--flow", a.flow}
	if a.c.Client != "" {
		then = append(then, "--client", a.c.Client)
	}
	c := tmux.Cmd{"command-prompt"}
	if a.c.Client != "" {
		c = append(c, "-t", a.c.Client)
	}
	if a.flow == autoui.FlowTime {
		// -I is a comma-separated list that tmux expands as a format.
		if t := a.find(id); t.has && t.sched.When != "" && !strings.ContainsAny(t.sched.When, ",#") {
			c = append(c, "-I", t.sched.When)
		}
	}
	c = append(c, "-p", i18n.T("auto.when_prompt"), "set-option -g "+autoui.OptText+" \"%%%\" ; "+tmux.RunShell(a.c.Self, then...))
	_, err := a.w.T.Run(a.ctx, c)
	return err
}

// setWhen takes the time chosen (a preset, or the one typed): Change
// time… sets it; ⏰ goes on to the CLI.
func (a *autoCmd) setWhen(id, v string) error {
	if a.fromOpt {
		v = strings.TrimSpace(a.opt(autoui.OptText))
		a.setOpt(autoui.OptText, "")
		if v == "" {
			return nil
		}
	}
	w, err := auto.ParseWhen(strings.ReplaceAll(v, ",", " "))
	if err != nil {
		if errors.Is(err, auto.ErrTooOften) {
			a.say(i18n.T("auto.too_often"))
		} else {
			a.say(i18n.T("auto.bad_when", v))
		}
		return nil
	}
	t := a.find(id)
	if a.flow == autoui.FlowTime {
		s, err := a.change(t, func(s *auto.Schedule) { s.When, s.Paused = w.String(), false })
		if err != nil {
			return err
		}
		a.say(i18n.T("auto.time_set", t.name(), w.Label()))
		a.startIfDue(s)
		a.refresh()
		return nil
	}
	if !t.found {
		return a.gone(t)
	}
	a.when = w.String()
	return a.showMenu(autoui.CLIMenu(a.c, id, t.name(), a.flow, a.when, a.clis(t)))
}

// setCLI takes the CLI chosen and asks for the model.
func (a *autoCmd) setCLI(id, v string) error {
	cli, err := parseAgentFlag("cli", v)
	if err != nil || cli == "" {
		return autoUsage()
	}
	if !a.w.Installed(cli) {
		a.say(i18n.T("auto.not_installed", session.ShortName(cli)))
		return nil
	}
	a.cli = string(cli)
	return a.modelPick(a.find(id))
}

// setModel ends a flow with the model chosen ("": the default, or what was
// typed): the schedule is made or changed; 🚀 runs it.
func (a *autoCmd) setModel(id, model string) error {
	if model == "" {
		model = strings.TrimSpace(a.initial)
	}
	if strings.ContainsAny(model, " \t\r\n") {
		a.say(i18n.T("auto.bad_model", model))
		return nil
	}
	cli, err := parseAgentFlag("cli", a.cli)
	if err != nil || cli == "" {
		return autoUsage()
	}
	t := a.find(id)
	if !t.found {
		return a.gone(t)
	}
	var w auto.When
	if a.flow == autoui.FlowNew {
		if w, err = auto.ParseWhen(a.when); err != nil {
			a.say(i18n.T("auto.bad_when", a.when))
			return nil
		}
	}
	s, err := a.change(t, func(s *auto.Schedule) {
		s.CLI, s.Model = string(cli), model
		if a.flow == autoui.FlowNew {
			s.When, s.Paused = w.String(), false
		}
	})
	if err != nil {
		return err
	}
	switch a.flow {
	case autoui.FlowRun:
		err = a.launch(t, s)
	case autoui.FlowNew:
		a.say(i18n.T("auto.scheduled", t.name(), s.Parsed().Label(), cliLabel(cli, model)))
		a.startIfDue(s)
	default:
		a.say(i18n.T("auto.cli_set", t.name(), cliLabel(cli, model)))
	}
	a.refresh()
	return err
}

// startIfDue starts s when it is due now: an interval schedule at once.
func (a *autoCmd) startIfDue(s auto.Schedule) {
	w := s.Parsed()
	if s.Paused || w.Manual() {
		return
	}
	if due, _ := w.Due(s.Created, auto.LastStart(s.ID), a.now); !due {
		return
	}
	if _, running := auto.Running(s.ID); !running {
		_ = auto.Launch(a.c.Self, s.ID, false)
	}
}

// run is 🚀: run it now with its CLI and model; the first time, ask them.
func (a *autoCmd) run(id string) error {
	t := a.find(id)
	if !t.found {
		return a.gone(t)
	}
	if _, running := auto.Running(id); running {
		a.say(i18n.T("auto.already", t.name()))
		return nil
	}
	if !t.has || t.sched.CLI == "" {
		a.flow = autoui.FlowRun
		return a.showMenu(autoui.CLIMenu(a.c, id, t.name(), autoui.FlowRun, "", a.clis(t)))
	}
	return a.launch(t, t.sched)
}

func (a *autoCmd) launch(t target, s auto.Schedule) error {
	if err := auto.Launch(a.c.Self, t.id, true); err != nil {
		return err
	}
	a.say(i18n.T("auto.started", t.name(), cliLabel(session.AgentID(s.CLI), s.Model)))
	return nil
}

// logPopup shows a run's output (the last one, the one going on, or --log)
// in a popup.
func (a *autoCmd) logPopup(id string) error {
	t := a.find(id)
	args := []string{a.c.Self, "__auto", "show", id}
	if a.log != "" {
		args = append(args, "--log", a.log)
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

// showLog is the popup's pager over a run's log, under what the run was.
func (a *autoCmd) showLog(id string) error {
	name := id
	if ss, err := auto.Load(); err == nil {
		if s, ok := auto.Get(ss, id); ok {
			name = s.Name
		}
	}
	cur, running := auto.Running(id)
	runs := auto.Runs(id)
	var r *auto.Run
	logName := a.log
	switch {
	case logName != "":
		for i := range runs {
			if runs[i].Log == logName {
				r = &runs[i]
			}
		}
	case running && cur.Log != "":
		logName = cur.Log
	case len(runs) > 0:
		r, logName = &runs[0], runs[0].Log
	}
	head := "\x1b[1m" + name + "\x1b[0m"
	switch {
	case r != nil:
		parts := []string{r.Status.Mark() + " " + textfmt.Clock(a.now, r.Start), auto.Took(r.Duration())}
		if r.CLI != "" {
			parts = append(parts, cliLabel(session.AgentID(r.CLI), r.Model))
		}
		head += "  " + strings.Join(parts, " · ")
	case running:
		head += "  " + i18n.T("auto.running", auto.Took(a.now.Sub(cur.Start)))
	}
	w, _ := sidebar.Size(int(os.Stdout.Fd()))
	if w <= 0 {
		w = 100
	}
	header := []string{" " + head, "\x1b[38;5;244m" + strings.Repeat("─", max(w, 1)) + "\x1b[0m"}
	var lines []string
	if logName != "" {
		if b, err := os.ReadFile(auto.LogPath(id, logName)); err == nil {
			for _, l := range strings.Split(auto.Clean(string(b)), "\n") {
				lines = append(lines, wrapLine(" "+l, w-2)...)
			}
		}
	}
	if len(lines) == 0 {
		lines = []string{" " + i18n.T("auto.no_output")}
	}
	return page(a.e, header, lines, 0)
}

// wrapLine breaks a line of plain text into lines of at most w columns, at
// spaces where it can.
func wrapLine(l string, w int) []string {
	if w < 8 || sidebar.DisplayWidth(l) <= w {
		return []string{l}
	}
	var out []string
	var cur []rune
	col, lastSpace := 0, -1
	for _, r := range l {
		cw := sidebar.DisplayWidth(string(r))
		if col+cw > w {
			if lastSpace > 0 {
				out = append(out, string(cur[:lastSpace]))
				cur = append([]rune(" "), cur[lastSpace+1:]...)
			} else {
				out = append(out, string(cur))
				cur = []rune(" ")
			}
			col, lastSpace = sidebar.DisplayWidth(string(cur)), -1
		}
		if r == ' ' {
			lastSpace = len(cur)
		}
		cur = append(cur, r)
		col += cw
	}
	return append(out, string(cur))
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
// sidebar follows (Claude, which ran with --session-id).
func (a *autoCmd) cont(id string) error {
	runs := auto.Runs(id)
	if len(runs) == 0 || runs[0].Session == "" || runs[0].CLI != string(session.Claude) {
		a.say(i18n.T("auto.no_session"))
		return nil
	}
	if a.c.Pane == "" {
		return errors.New("no pane chosen")
	}
	args := []string{"split", runs[0].CLI, "-h", "--from-session", runs[0].CLI + ":" + runs[0].Session, "--pane", a.c.Pane}
	if a.c.Client != "" {
		args = append(args, "--client", a.c.Client)
	}
	return exec.Command(a.c.Self, args...).Run()
}

// exec runs schedule id (`imux __auto exec`, started on its own by the
// watcher or 🚀): its definition with its CLI and model in its project,
// headless, holding its run lock; then records the run, redraws the
// sidebars and notifies.
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
	// Started on schedule twice at once (the watcher, and ⏰ starting it at
	// once): the second, which waited for the first's lock, is not due any
	// more.
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
	r.Log, r.Session = auto.LogName(r.Start), inv.Session
	logPath := auto.LogPath(id, r.Log)
	o := auto.Execute(a.ctx, inv.Argv, autoEnv(bin.PathEnv), s.Project, logPath, s.RunTimeout(), func(pgid int) {
		ex.Started(auto.Current{Start: r.Start, PGID: pgid, Log: r.Log, CLI: string(cli), Model: s.Model})
		a.refresh()
	})
	r.Exit = o.Exit
	switch {
	case ex.Stopped():
		r.Status = auto.StatusStopped
	case o.TimedOut:
		r.Status, r.Summary = auto.StatusTimeout, i18n.T("auto.timed_out_after", auto.Took(s.RunTimeout()))
	case o.Err != nil:
		r.Status, r.Summary = auto.StatusFailed, auto.FirstLine(o.Err.Error())
	case o.Exit != 0:
		r.Status, r.Summary = auto.StatusFailed, firstNonEmpty(auto.FirstLine(o.Stderr), auto.FirstLine(o.Stdout))
	default:
		report := o.Stdout
		if inv.OutFile != "" {
			if b, err := os.ReadFile(inv.OutFile); err == nil && len(strings.TrimSpace(string(b))) > 0 {
				report = string(b)
				if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_WRONLY, 0); err == nil {
					fmt.Fprintf(f, "\n# reply\n%s\n", strings.TrimSpace(report))
					f.Close()
				}
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
