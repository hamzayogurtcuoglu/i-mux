package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/fileops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/skills"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:   "__skill",
		Args:   "<op> [--client C]",
		Hidden: true,
		Run:    runSkill,
	})
}

// skillCtx is what a Skills view action or menu item works on.
type skillCtx struct {
	ctx    context.Context
	w      *workspace.WS
	client string
	dir    string // sidebar.OptSkill (the skill's folder, absolute)
	root   string // sidebar.OptSkillRoot (the project root)
	pane   string // sidebar.OptSkillPane (the target agent pane)
	text   string // sidebar.OptSkillText (a prompt's answer)
	sys    *fileops.System
}

// runSkill runs an operation of the Skills view (§9.3 "Skills"). The skill's
// folder, the project root and the target pane come from the options the
// sidebar set before opening the menu or prompt.
func runSkill(e *Env, args []string) error {
	fs := newFlags(e, "__skill")
	clientF := fs.String("client", "", "client for messages, prompts and popups")
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
	s := &skillCtx{ctx: context.Background(), w: w, client: *clientF, sys: &fileops.System{}}
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

// prompt asks on the client; the answer goes to OptSkillText, then `imux __skill <then>` runs.
func (s *skillCtx) prompt(label, initial, then string) error {
	c := tmux.Cmd{"command-prompt"}
	if s.client != "" {
		c = append(c, "-t", s.client)
	}
	if initial != "" && !strings.Contains(initial, ",") {
		c = append(c, "-I", initial)
	}
	tpl := "set-option -g " + sidebar.OptSkillText + " \"%%%\" ; " + tmux.RunShell(s.w.Self, s.again(then)...)
	_, err := s.w.T.Run(s.ctx, append(c, "-p", label, tpl))
	return err
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
	case "write-prompt":
		if _, err := s.agentPane(); err != nil {
			s.say(i18n.T("skills.no_agent"))
			return nil
		}
		return s.prompt(i18n.T("skills.write_prompt"), "", "write-done")
	case "write-done":
		ask := strings.TrimSpace(s.text)
		if ask == "" {
			return nil
		}
		p, err := s.agentPane()
		if err != nil {
			s.say(i18n.T("skills.no_agent"))
			return nil
		}
		if err := s.w.Paste(ctx, p.ID, skills.WritePrompt(s.root, ask), true); err != nil {
			return err
		}
		_, _ = s.w.T.Run(ctx, tmux.Cmd{"select-pane", "-t", p.ID})
		s.say(i18n.T("skills.writing", session.ShortName(p.Agent)))
		return nil
	case "new-prompt":
		return s.prompt(i18n.T("skills.new_prompt"), "", "new-done")
	case "new-done":
		name := skills.Slug(s.text)
		if name == "" {
			return nil
		}
		p, err := skills.Create(s.root, name, "")
		if errors.Is(err, skills.ErrExists) {
			s.say(i18n.T("skills.exists", name))
			return nil
		}
		if err != nil {
			return err
		}
		_, _ = s.w.T.Run(ctx, tmux.Cmd{"set-option", "-g", sidebar.OptSkill, filepath.Dir(p)})
		sidebar.RefreshAll(ctx, s.w.T)
		return s.popup("__edit", s.rel(p))
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
