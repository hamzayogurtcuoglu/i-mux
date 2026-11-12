package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/auto"
	"github.com/hamzayogurtcuoglu/i-mux/internal/fileops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/skilldialog"
	"github.com/hamzayogurtcuoglu/i-mux/internal/skills"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:   "__skill",
		Args:   "<op> [--client C] [--agent A | --here] [--template] [--agents]",
		Hidden: true,
		Run:    runSkill,
	})
}

// The New skill dialog's popup: at most this big, inside the client.
const (
	dialogW = 78
	dialogH = 22
)

// skillCtx is what a Skills view action or menu item works on.
type skillCtx struct {
	ctx      context.Context
	e        *Env
	w        *workspace.WS
	client   string
	dir      string // sidebar.OptSkill (the skill's folder, absolute)
	root     string // sidebar.OptSkillRoot (the project root)
	pane     string // sidebar.OptSkillPane (the target agent pane)
	text     string // sidebar.OptSkillText (what the dialog was told)
	agent    string // --agent: who writes the skill in a new pane
	here     bool   // --here: the target pane's agent writes it
	template bool   // --template: the dialog opens at the blank template
	agents   bool   // --agents: the New agent dialog (the Scheduler's, D61)
	sys      *fileops.System
}

// runSkill runs an operation of the Skills view (§9.3 "Skills"). The skill's
// folder, the project root and the target pane come from the options the
// sidebar set before opening the menu or the dialog.
func runSkill(e *Env, args []string) error {
	fs := newFlags(e, "__skill")
	clientF := fs.String("client", "", "client for messages, prompts and popups")
	agentF := fs.String("agent", "", "write: the agent that writes the skill in a new pane")
	hereF := fs.Bool("here", false, "write: the target pane's agent writes the skill")
	templateF := fs.Bool("template", false, "dialog: open at the blank template's name")
	agentsF := fs.Bool("agents", false, "dialog, new, write: an agent definition for the Scheduler instead of a skill")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "__skill <op>")}
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	s := &skillCtx{ctx: context.Background(), e: e, w: w, client: *clientF, agent: *agentF, here: *hereF, template: *templateF, agents: *agentsF, sys: &fileops.System{}}
	out, err := w.T.Run(s.ctx,
		tmux.DelimCmd("dir"), tmux.Cmd{"show-options", "-gqv", sidebar.OptSkill},
		tmux.DelimCmd("root"), tmux.Cmd{"show-options", "-gqv", sidebar.OptSkillRoot},
		tmux.DelimCmd("pane"), tmux.Cmd{"show-options", "-gqv", sidebar.OptSkillPane},
		tmux.DelimCmd("text"), tmux.Cmd{"show-options", "-gqv", sidebar.OptSkillText})
	if err != nil {
		return err
	}
	sec := tmux.Sections(out)
	val := func(k string) string { return strings.TrimRight(sec[k], "\n") }
	s.dir, s.root, s.pane, s.text = val("dir"), val("root"), val("pane"), val("text")
	if s.root == "" {
		return errors.New("no project chosen")
	}
	err = s.do(pos[0])
	if err != nil {
		s.say(i18n.T("file.failed", err.Error()))
	}
	return err
}

func (s *skillCtx) say(msg string) { s.w.Message(s.ctx, s.client, s.pane, msg) }

func (s *skillCtx) again(op string) []string {
	args := []string{"__skill", op}
	if s.client != "" {
		args = append(args, "--client", s.client)
	}
	if s.agents {
		args = append(args, "--agents")
	}
	return args
}

func (s *skillCtx) rel(p string) string {
	if r, err := filepath.Rel(s.root, p); err == nil {
		return r
	}
	return p
}

// popup runs an imux command in a popup in the project root.
func (s *skillCtx) popup(args ...string) error {
	c := tmux.Cmd{"display-popup"}
	if s.client != "" {
		c = append(c, "-c", s.client)
	}
	sh := tmux.ShJoin(append([]string{s.w.Self}, args...)...)
	return s.w.T.Spawn(append(c, "-E", "-w", "90%", "-h", "90%", "-d", tmux.EscapeFormat(s.root), sh))
}

// dialog opens the New skill dialog (`__skill new`) in a popup sized to the
// client, in the project root.
func (s *skillCtx) dialog() error {
	w, h := "80%", "70%"
	if s.client != "" {
		out, err := s.w.T.Run(s.ctx, tmux.Cmd{"display-message", "-p", "-c", s.client, "#{client_width}\t#{client_height}"})
		if f := strings.Split(strings.TrimSpace(out), "\t"); err == nil && len(f) == 2 {
			cw, _ := strconv.Atoi(f[0])
			ch, _ := strconv.Atoi(f[1])
			if cw > 0 && ch > 0 {
				w, h = strconv.Itoa(min(cw, dialogW)), strconv.Itoa(min(ch, dialogH))
			}
		}
	}
	args := append([]string{s.w.Self}, s.again("new")...)
	if s.template {
		args = append(args, "--template")
	}
	c := tmux.Cmd{"display-popup"}
	if s.client != "" {
		c = append(c, "-c", s.client)
	}
	return s.w.T.Spawn(append(c, "-E", "-w", w, "-h", h, "-d", tmux.EscapeFormat(s.root), tmux.ShJoin(args...)))
}

// newSkill runs the New skill dialog on this terminal (the popup) and acts
// on its answer: a template is created and edited right here; an agent's
// answer goes to `__skill write`, which tmux runs once the popup is closed.
func (s *skillCtx) newSkill() error {
	ctx := s.ctx
	var cur session.AgentID
	if p, err := s.agentPane(); err == nil {
		cur = p.Agent
	}
	var agents []skilldialog.Agent
	for _, a := range agent.Ordered(s.w.Cfg) {
		agents = append(agents, skilldialog.Agent{ID: a.ID, Name: a.Short, Key: a.Key, Installed: s.w.Installed(a.ID), Limited: s.w.LimitedUntil(a.ID)})
	}
	o := skilldialog.Options{
		Agents: agents, Current: cur, Template: s.template,
		Exists: func(name string) bool { return skills.Exists(s.root, name) },
	}
	if s.agents {
		o.Texts, o.ExistsKey = "auto.dialog", "auto.exists"
		o.Exists = func(name string) bool { return auto.Exists(s.root, name) }
		o.Path = func(name string) string { return s.rel(auto.AgentFile(s.root, session.Claude, name)) }
	}
	m := skilldialog.New(o)
	res, err := skilldialog.Run(m, os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	switch res.Kind {
	case skilldialog.Template:
		if s.agents {
			p, err := auto.Create(s.root, res.Text)
			if errors.Is(err, auto.ErrExists) {
				s.say(i18n.T("auto.exists", res.Text))
				return nil
			}
			if err != nil {
				return err
			}
			sidebar.RefreshAll(ctx, s.w.T)
			return runEdit(s.e, []string{s.rel(p)})
		}
		p, err := skills.Create(s.root, res.Text, "")
		if errors.Is(err, skills.ErrExists) {
			s.say(i18n.T("skills.exists", res.Text))
			return nil
		}
		if err != nil {
			return err
		}
		_, _ = s.w.T.Run(ctx, tmux.Cmd{"set-option", "-g", sidebar.OptSkill, filepath.Dir(p)})
		sidebar.RefreshAll(ctx, s.w.T)
		return runEdit(s.e, []string{s.rel(p)})
	case skilldialog.NewPane, skilldialog.Here:
		args := s.again("write")
		if res.Kind == skilldialog.Here {
			args = append(args, "--here")
		} else {
			args = append(args, "--agent", string(res.Agent))
		}
		_, err := s.w.T.Run(ctx,
			tmux.Cmd{"set-option", "-g", sidebar.OptSkillText, res.Text},
			tmux.Cmd{"run-shell", "-b", tmux.UICommand(s.w.Self, args...)})
		return err
	}
	return nil
}

// write has an agent write a new skill: i-mux's guide, then what the dialog
// was told. --here sends it to the target pane's agent; --agent opens that
// agent beside the target pane (or in the "No agents" pane), named for it.
func (s *skillCtx) write() error {
	ctx := s.ctx
	lang := i18n.T("skills.language")
	prompt := skills.WritePrompt(s.root, s.text, lang)
	if s.here {
		p, err := s.agentPane()
		if err != nil {
			s.say(i18n.T("skills.no_agent"))
			return nil
		}
		msg := i18n.T("skills.writing", session.ShortName(p.Agent))
		if s.agents {
			prompt = auto.WritePrompt(s.root, p.Agent, s.text, lang)
			msg = i18n.T("auto.writing", session.ShortName(p.Agent), auto.AgentDir(p.Agent))
		}
		if err := s.w.Paste(ctx, p.ID, prompt, true); err != nil {
			return err
		}
		_, _ = s.w.T.Run(ctx, tmux.Cmd{"select-pane", "-t", p.ID})
		s.say(msg)
		return nil
	}
	id, err := parseAgentFlag("agent", s.agent)
	if err != nil || id == "" {
		return &UsageError{Msg: i18n.T("cli.usage", "__skill write --agent <agent> | --here")}
	}
	paneName, msg := i18n.T("skills.pane_name"), i18n.T("skills.writing_new", session.ShortName(id))
	if s.agents {
		prompt = auto.WritePrompt(s.root, id, s.text, lang)
		paneName, msg = i18n.T("auto.pane_name"), i18n.T("auto.writing_new", session.ShortName(id))
	}
	if s.pane == "" {
		return errors.New("no pane chosen")
	}
	p, err := s.w.PaneTarget(ctx, s.pane)
	if err != nil {
		return err
	}
	if _, err := s.w.StartPrompt(ctx, p, id, prompt, paneName); err != nil {
		s.say(workspace.SwitchError(id, err))
		return nil
	}
	s.say(msg)
	return nil
}

// agentPane returns the target pane when it runs a live agent.
func (s *skillCtx) agentPane() (*workspace.Pane, error) {
	if s.pane == "" {
		return nil, errors.New("no agent")
	}
	p, err := s.w.PaneTarget(s.ctx, s.pane)
	if err != nil {
		return nil, err
	}
	if p.Agent == "" || p.Dead {
		return nil, errors.New("no agent")
	}
	return p, nil
}

func (s *skillCtx) file() string { return filepath.Join(s.dir, skills.File) }

func (s *skillCtx) do(op string) error {
	ctx := s.ctx
	switch op {
	case "dialog":
		return s.dialog()
	case "new":
		return s.newSkill()
	case "write":
		return s.write()
	}
	if s.dir == "" {
		return errors.New("no skill chosen")
	}
	switch op {
	case "open":
		return s.popup("__edit", s.rel(s.file()))
	case "view":
		return s.popup("__view", s.rel(s.file()))
	case "add":
		p, err := s.agentPane()
		if err != nil {
			s.say(i18n.T("skills.no_agent"))
			return nil
		}
		text := ImageText(p.Agent, []string{s.file()})
		_, err = s.w.T.Run(ctx,
			tmux.Cmd{"set-buffer", "-b", "imux-skill", "--", text},
			tmux.Cmd{"paste-buffer", "-p", "-d", "-b", "imux-skill", "-t", p.ID},
			tmux.Cmd{"select-pane", "-t", p.ID})
		return err
	case "improve":
		p, err := s.agentPane()
		if err != nil {
			s.say(i18n.T("skills.no_agent"))
			return nil
		}
		if err := s.w.Paste(ctx, p.ID, skills.ImprovePrompt(s.file()), true); err != nil {
			return err
		}
		_, _ = s.w.T.Run(ctx, tmux.Cmd{"select-pane", "-t", p.ID})
		s.say(i18n.T("skills.improving", session.ShortName(p.Agent)))
		return nil
	case "share":
		dst, err := skills.Share(s.root, s.dir)
		if errors.Is(err, skills.ErrExists) {
			s.say(i18n.T("skills.exists", filepath.Base(s.dir)))
			return nil
		}
		if err != nil {
			return err
		}
		_, _ = s.w.T.Run(ctx, tmux.Cmd{"set-option", "-g", sidebar.OptSkill, dst})
		sidebar.RefreshAll(ctx, s.w.T)
		s.say(i18n.T("skills.shared", filepath.Base(dst)))
		return nil
	case "delete":
		name := filepath.Base(s.dir)
		m := &workspace.Menu{
			Title: i18n.T("file.delete_title", name),
			Items: []workspace.MenuItem{
				{Name: i18n.T("file.delete_text"), Disabled: true},
				workspace.Separator(),
				{Name: i18n.T("file.trash"), Key: "y", Cmd: tmux.RunShell(s.w.Self, s.again("trash")...)},
				{Name: i18n.T("click.cancel"), Key: "n"},
			},
		}
		return s.w.Show(ctx, m, s.client, "")
	case "trash":
		for _, l := range skills.Links(s.root, s.dir) {
			_ = os.Remove(l)
		}
		if err := s.sys.Trash(ctx, s.dir); err != nil {
			m := &workspace.Menu{
				Title: i18n.T("file.trash_failed_title"),
				Items: []workspace.MenuItem{
					{Name: i18n.T("file.trash_failed", firstLine(err.Error())), Disabled: true},
					workspace.Separator(),
					{Name: i18n.T("file.delete_permanently"), Key: "d", Cmd: tmux.RunShell(s.w.Self, s.again("delete-permanently")...)},
					{Name: i18n.T("click.cancel"), Key: "n"},
				},
			}
			return s.w.Show(ctx, m, s.client, "")
		}
		sidebar.RefreshAll(ctx, s.w.T)
		return nil
	case "delete-permanently":
		for _, l := range skills.Links(s.root, s.dir) {
			_ = os.Remove(l)
		}
		if err := os.RemoveAll(s.dir); err != nil {
			return err
		}
		sidebar.RefreshAll(ctx, s.w.T)
		return nil
	}
	return &UsageError{Msg: i18n.T("cli.usage", "__skill <op>")}
}
