package sidebar

import (
	"context"
	"path/filepath"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/skills"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Global options of the skills' menus and prompts (§9.3 "Skills"; in Jobs
// since v0.56, D66).
// A menu's commands and a prompt's template are formats, so paths and pane
// ids are kept here instead of in them.
const (
	OptSkill     = "@imux_skill"      // the skill folder a menu is about (absolute)
	OptSkillRoot = "@imux_skill_root" // the project's root folder
	OptSkillPane = "@imux_skill_pane" // the target agent pane
	OptSkillText = "@imux_skill_text" // a prompt's answer
)

// skillProblem is a short explanation of a skill's problem.
func skillProblem(p string) string {
	switch p {
	case skills.ProbNoHeader:
		return i18n.T("skills.problem.no_header")
	case skills.ProbNoName:
		return i18n.T("skills.problem.no_name")
	case skills.ProbNoDesc:
		return i18n.T("skills.problem.no_description")
	case skills.ProbNameFolder:
		return i18n.T("skills.problem.name_folder")
	case skills.ProbBadName:
		return i18n.T("skills.problem.bad_name")
	case skills.ProbLongDesc:
		return i18n.T("skills.problem.long_description")
	}
	return ""
}

// SkillMenuState decides which items a skill menu has.
type SkillMenuState struct {
	Root   bool // empty space and the title
	Agent  bool // the target pane runs an agent
	Shared bool // already in .agents/skills and linked for Claude
}

// SkillMenuItems lists a skill menu as display-menu arguments: name, key,
// command; "" alone is a separator. Every command is `imux __skill`.
func SkillMenuItems(self, client string, st SkillMenuState) []string {
	var items []string
	sep := func() {
		if len(items) > 0 && items[len(items)-1] != "" {
			items = append(items, "")
		}
	}
	add := func(label, key string, op ...string) {
		args := append(append([]string{"__skill"}, op...), clientArgs(client)...)
		items = append(items, tmux.EscapeFormat(i18n.T(label)), key, tmux.RunShell(self, args...))
	}
	if st.Root {
		add("skills.new_button", "n", "dialog")
		add("skills.menu_template", "t", "dialog", "--template")
		return items
	}
	add("file.open", "o", "open")
	add("file.view", "v", "view")
	if st.Agent || !st.Shared {
		sep()
	}
	if st.Agent {
		add("file.add_to_agent", "a", "add")
		add("skills.improve", "i", "improve")
	}
	if !st.Shared {
		add("skills.share", "s", "share")
	}
	sep()
	add("file.delete", "d", "delete")
	return items
}

// skillStart sets the skill options and runs `imux __skill <op>`.
func (s *sidebar) skillStart(ctx context.Context, client, op string) {
	if s.model.CWD == "" {
		return
	}
	root := skills.Root(s.model.CWD)
	_, _ = s.t.Run(ctx,
		tmux.Cmd{"set-option", "-g", OptSkillRoot, root},
		tmux.Cmd{"set-option", "-g", OptSkillPane, s.model.Target})
	s.spawn(append([]string{"__skill", op}, clientArgs(client)...))
}

// skillMenu opens the menu of a skill (or, with no skill, New skill… and
// Blank SKILL.md…: Jobs' title and empty space) at (x, y) in the sidebar.
func (s *sidebar) skillMenu(ctx context.Context, client string, a Action, x, y int) {
	if s.model.CWD == "" {
		return
	}
	root := skills.Root(s.model.CWD)
	dir := a.Path
	shared := true
	for _, sk := range skills.List(root) {
		if sk.Dir == dir {
			shared = sk.Shared
			break
		}
	}
	_, err := s.t.Run(ctx,
		tmux.Cmd{"set-option", "-g", OptSkill, dir},
		tmux.Cmd{"set-option", "-g", OptSkillRoot, root},
		tmux.Cmd{"set-option", "-g", OptSkillPane, s.model.Target})
	if err != nil {
		return
	}
	st := SkillMenuState{
		Root:   dir == "",
		Agent:  s.targetAgent() != "",
		Shared: shared,
	}
	items := SkillMenuItems(s.o.Self, client, st)
	title := i18n.T("rail.skills")
	if dir != "" {
		title = filepath.Base(dir)
	}
	c := append(s.t.Menu(ctx), "-t", s.o.Pane)
	if client != "" {
		c = append(c, "-c", client)
	}
	if x > 0 && y > 0 {
		px, py := MenuAt(x, y)
		c = append(c, "-T", tmux.EscapeFormat(title), "-x", px, "-y", py, "--")
	} else {
		c = append(c, "-T", tmux.EscapeFormat(title), "-x", "C", "-y", "C", "--")
	}
	_ = s.t.ShowMenu(ctx, append(c, items...))
}
