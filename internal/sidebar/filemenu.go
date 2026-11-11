package sidebar

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/editor"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/webview"
)

// Global options of the explorer's menus (§9.3 "Explorer menus and hover").
// A menu's commands are formats, so the path they act on is not in them: it
// is set here just before the menu opens.
const (
	OptFile     = "@imux_file"      // the path a menu is about (absolute)
	OptFileRoot = "@imux_file_root" // its project folder
	OptFileName = "@imux_file_name" // a name typed in a prompt
	OptFileClip = "@imux_file_clip" // "cut:<path>" or "copy:<path>"
	OptCompare  = "@imux_compare"   // the file selected for compare
	OptReveal   = "@imux_reveal"    // "<n>:reveal:<path>" or "<n>:collapse:<root>"
)

// Menu kinds of the explorer.
const (
	MenuFile   = "file"
	MenuFolder = "folder"
	MenuRoot   = "root" // empty space and the title: the project folder
)

// MenuState is what decides which items an explorer menu has.
type MenuState struct {
	Kind    string // MenuFile, MenuFolder, MenuRoot
	Agent   bool   // the target pane runs an agent (Add to Agent)
	Clip    bool   // something was cut or copied (Paste)
	Compare string // the file selected for compare, "" for none
	Path    string // the menu's path (absolute)
	Reveal  string // i18n key of the reveal item, "" without a file manager
	VSCode  bool   // VS Code's CLI is there
}

// FileMenuItems lists an explorer menu as display-menu arguments: name,
// key, command; "" alone is a separator. Every command is `imux __file`.
func FileMenuItems(self, client, pane string, st MenuState) []string {
	var items []string
	sep := func() {
		if len(items) > 0 && items[len(items)-1] != "" {
			items = append(items, "")
		}
	}
	add := func(label, key, op string) {
		args := []string{"__file", op}
		if client != "" {
			args = append(args, "--client", client)
		}
		if pane != "" {
			args = append(args, "--pane", pane)
		}
		items = append(items, tmux.EscapeFormat(i18n.T(label)), key, tmux.RunShell(self, args...))
	}
	file, root := st.Kind == MenuFile, st.Kind == MenuRoot
	if file {
		add("file.open", "o", "open")
		if pane != "" {
			add("file.open_side", "s", "side")
		}
		if st.VSCode {
			add("file.vscode", "c", "vscode")
		}
		add("file.view", "v", "view")
		if webview.Previewable(st.Path) {
			add("file.preview", "w", "preview") // the live preview in the browser (D47)
		}
	} else {
		add("file.new_file", "n", "new-file")
		add("file.new_folder", "N", "new-folder")
	}
	sep()
	if st.Reveal != "" {
		add(st.Reveal, "f", "reveal")
	}
	if pane != "" {
		add("file.terminal", "t", "terminal")
	}
	if !file {
		add("file.find", "/", "find")
	}
	if st.Agent && !root {
		add("file.add_to_agent", "a", "add")
	}
	if file {
		sep()
		add("file.compare_select", "k", "compare-select")
		if st.Compare != "" && st.Compare != st.Path {
			items = append(items, tmux.EscapeFormat(i18n.T("file.compare_with", filepath.Base(st.Compare))), "K",
				tmux.RunShell(self, append([]string{"__file", "compare"}, clientArgs(client)...)...))
		}
	}
	sep()
	if !root {
		add("file.cut", "x", "cut")
		add("file.copy", "y", "copy")
	}
	if st.Clip {
		add("file.paste", "p", "paste")
	}
	sep()
	add("file.copy_path", "P", "copy-path")
	if !root {
		add("file.copy_relative_path", "l", "copy-relative-path")
		sep()
		add("file.rename", "r", "rename")
		add("file.delete", "d", "delete")
	} else {
		sep()
		add("file.collapse", "z", "collapse")
	}
	if n := len(items); n > 0 && items[n-1] == "" {
		items = items[:n-1]
	}
	return items
}

func clientArgs(client string) []string {
	if client == "" {
		return nil
	}
	return []string{"--client", client}
}

// fileMenu opens the explorer's menu for a, at the pointer (x, y in the
// sidebar, 1-based).
func (s *sidebar) fileMenu(ctx context.Context, client string, a Action, x, y int) {
	root := s.model.CWD
	if root == "" {
		return
	}
	abs := root
	if a.Section != MenuRoot {
		abs = filepath.Join(root, a.Path)
	}
	out, err := s.t.Run(ctx,
		tmux.Cmd{"set-option", "-g", OptFile, abs},
		tmux.Cmd{"set-option", "-g", OptFileRoot, root},
		tmux.DelimCmd("clip"), tmux.Cmd{"show-options", "-gqv", OptFileClip},
		tmux.DelimCmd("cmp"), tmux.Cmd{"show-options", "-gqv", OptCompare})
	if err != nil {
		return
	}
	sec := tmux.Sections(out)
	st := MenuState{
		Kind:    a.Section,
		Agent:   s.targetAgent() != "",
		Clip:    strings.TrimSpace(sec["clip"]) != "",
		Compare: strings.TrimSpace(sec["cmp"]),
		Path:    abs,
		Reveal:  s.revealKey,
		VSCode:  editor.VSCode() != "",
	}
	items := FileMenuItems(s.o.Self, client, s.model.Target, st)
	title := filepath.Base(abs)
	c := append(s.t.Menu(ctx), "-t", s.o.Pane)
	if client != "" {
		c = append(c, "-c", client)
	}
	px, py := MenuAt(x, y)
	c = append(append(c, "-T", tmux.EscapeFormat(title), "-x", px, "-y", py, "--"), items...)
	_ = s.t.Spawn(c)
}

// MenuAt is display-menu's -x and -y for a menu whose top left corner is
// at column x, row y (1-based) of the target pane. tmux works them out:
// popup_pane_left / popup_pane_top are in its own coordinates (the
// terminal's up to 3.7, the window's from 3.8, where the status line is not
// counted), and popup_pane_top already holds the menu's height (-y is its
// bottom line).
func MenuAt(x, y int) (string, string) {
	return "#{e|+:#{popup_pane_left}," + strconv.Itoa(x-1) + "}",
		"#{e|+:#{popup_pane_top}," + strconv.Itoa(y-1) + "}"
}

// targetAgent is the agent of the target pane ("" for none).
func (s *sidebar) targetAgent() string {
	for _, p := range s.model.Panes {
		if p.ID == s.model.Target {
			return string(p.Agent)
		}
	}
	return ""
}

// reveal acts on OptReveal: the folders above a created, renamed or pasted
// path open and it is selected and scrolled to; "collapse" closes them all.
// The first value a sidebar sees is only remembered.
func (s *sidebar) reveal(v string) bool {
	if v == s.lastReveal {
		return false
	}
	first := !s.revealSeen
	s.lastReveal, s.revealSeen = v, true
	if first {
		return false
	}
	f := strings.SplitN(v, ":", 3)
	if len(f) != 3 || s.model.CWD == "" {
		return false
	}
	switch f[1] {
	case "collapse":
		if filepath.Clean(f[2]) != filepath.Clean(s.model.CWD) {
			return false
		}
		s.o.Source.Expanded = map[string]bool{}
		s.scroll = 0
		return true
	case "reveal":
		rel, err := filepath.Rel(s.model.CWD, f[2])
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return false
		}
		if s.o.Source.Expanded == nil {
			s.o.Source.Expanded = map[string]bool{}
		}
		for d := filepath.Dir(rel); d != "." && d != "/"; d = filepath.Dir(d) {
			s.o.Source.Expanded[d] = true
		}
		s.selected = rel
		s.revealRow = rel
		return true
	}
	return false
}

// keepVisible scrolls the explorer so the revealed row shows.
func (s *sidebar) keepVisible() {
	if s.revealRow == "" {
		return
	}
	defer func() { s.revealRow = "" }()
	n := s.win.Height - 1 // the title row
	for i, r := range s.model.Tree {
		if r.Path != s.revealRow {
			continue
		}
		if n > 0 && (i < s.scroll || i >= s.scroll+n) {
			s.scroll = max(i-n/2, 0)
			s.model.Scroll = s.scroll
		}
		return
	}
}
