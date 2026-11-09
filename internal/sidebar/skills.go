package sidebar

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/skills"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Global options of the Skills view's menus and prompts (§9.3 "Skills").
// A menu's commands and a prompt's template are formats, so paths and pane
// ids are kept here instead of in them.
const (
	OptSkill     = "@imux_skill"      // the skill folder a menu is about (absolute)
	OptSkillRoot = "@imux_skill_root" // the project's root folder
	OptSkillPane = "@imux_skill_pane" // the target agent pane
	OptSkillText = "@imux_skill_text" // a prompt's answer
)

// skillOrder is the order of the agent badges on a skill row.
var skillOrder = []session.AgentID{
	session.Claude,
	session.Antigravity,
	session.Copilot,
	session.Codex,
	session.OpenCode,
}

// skillAgents draws one glyph per agent CLI: its colored icon when it sees
// the skill, a dim dot otherwise (5 columns).
func skillAgents(sk skills.Skill) (string, int) {
	var b strings.Builder
	for _, a := range skillOrder {
		if sk.Agents[a] {
			sym, color := icon(a)
			b.WriteString(color + sym + reset)
		} else {
			b.WriteString(faint + "·" + reset)
		}
	}
	return b.String(), len(skillOrder)
}

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

// relSkill is sk's SKILL.md relative to cwd when possible.
func relSkill(cwd string, sk skills.Skill) string {
	p := sk.Path()
	if cwd != "" {
		if r, err := filepath.Rel(cwd, p); err == nil {
			return r
		}
	}
	return p
}

// skillRows draws one skill: its name, agent badges and menu button on the
// first row, its description (or problem) on the second.
func skillRows(sk skills.Skill, cwd string, w int, ic Icons, selected bool) []Line {
	rel := relSkill(cwd, sk)
	open := Action{Kind: ActOpenFile, Path: rel, Status: sk.Dir}
	menu := button(ic.Suggest, cyan, Action{Kind: ActSkillMenu, Path: sk.Dir})

	mark := green + "●" + reset
	nameStyle := bold
	if sk.Problem != "" {
		mark = yellow + "⚠" + reset
		nameStyle = yellow
	}
	badges, bw := skillAgents(sk)
	mw := max(w-menu.W, 1)
	// " " mark " " name " " badges
	nw := max(mw-3-bw-1, 1)
	var left string
	if mw >= 3+bw+4 {
		left = " " + mark + " " + nameStyle + fit(sk.Name, nw) + reset + " " + badges
	} else {
		left = " " + mark + " " + nameStyle + fit(sk.Name, max(mw-3, 1)) + reset
	}
	if selected {
		left = onBg(left, selectBg)
	}
	first := buttons(w, Cell{Text: left, Action: open}, menu)

	sub := sk.Description
	subColor := muted
	if sk.Problem != "" {
		sub = skillProblem(sk.Problem)
		subColor = yellow
	}
	if sub == "" {
		return []Line{first}
	}
	second := "   " + subColor + fit(sub, max(w-3, 1)) + reset
	if selected {
		second = onBg(second, selectBg)
	}
	return []Line{first, {Text: second, Action: open}}
}

// renderSkills is the Skills view (D32): the project's Agent Skills, a
// button that asks the active agent to write one, and a button that creates
// a template SKILL.md.
func renderSkills(m Model, w, h int) []Line {
	ic := m.icons()
	refresh := button(ic.Refresh, muted, Action{Kind: ActRefresh})
	head := i18n.T("sidebar.skills")
	if len(m.Skills) > 0 {
		head += " · " + strconv.Itoa(len(m.Skills))
	} else if m.Project != "" {
		head += " · " + m.Project
	}
	out := []Line{buttons(w, Cell{Text: title + fit(" "+head, w-refresh.W) + reset}, refresh)}

	// Primary button: ✨ Write with Agent…; and + for a blank template.
	newBtn := button(ic.Add, cyan, Action{Kind: ActSkillNew})
	label := ic.Suggest + " " + i18n.T("skills.write_button")
	cw := max(w-newBtn.W, 1)
	pad := max(cw-2-DisplayWidth(label), 0)
	writeCell := Cell{
		W:      cw,
		Text:   " " + "\x1b[48;5;24m\x1b[38;5;255m" + strings.Repeat(" ", pad/2) + fit(label, cw-2-pad) + strings.Repeat(" ", pad-pad/2) + reset + " ",
		Action: Action{Kind: ActSkillWrite},
	}
	newBtn.X = cw
	out = append(out, Line{Text: compose([]Cell{writeCell, newBtn}, w), Cells: []Cell{writeCell, newBtn}})

	var rows []Line
	for _, sk := range m.Skills {
		sel := m.Selected != "" && (m.Selected == sk.Dir || m.Selected == relSkill(m.CWD, sk))
		rows = append(rows, skillRows(sk, m.CWD, w, ic, sel)...)
	}
	if len(rows) == 0 {
		rows = append(rows,
			Line{Text: muted + fit("   "+i18n.T("skills.empty"), w) + reset},
			Line{Text: faint + fit("   .agents/skills/<id>/SKILL.md", w) + reset},
			Line{Text: cyan + fit("   "+i18n.T("skills.empty_hint"), w) + reset, Action: Action{Kind: ActSkillWrite}},
		)
	}
	n := 0
	if h > 0 {
		n = h - len(out)
	}
	return append(out, scrolled(rows, m.Scroll, n)...)
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
	add := func(label, key, op string) {
		args := append([]string{"__skill", op}, clientArgs(client)...)
		items = append(items, tmux.EscapeFormat(i18n.T(label)), key, tmux.RunShell(self, args...))
	}
	if st.Root {
		if st.Agent {
			add("skills.menu_write", "w", "write-prompt")
		}
		add("skills.menu_new", "n", "new-prompt")
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

// skillMenu opens the menu of a skill (or of the Skills view's empty space)
// at (x, y) in the sidebar.
func (s *sidebar) skillMenu(ctx context.Context, client string, a Action, x, y int) {
	if s.model.CWD == "" {
		return
	}
	root := skills.Root(s.model.CWD)
	dir := a.Path
	shared := true
	for _, sk := range s.model.Skills {
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
	_ = s.t.Spawn(append(c, items...))
}
