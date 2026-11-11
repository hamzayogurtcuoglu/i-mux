package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/completion"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/diffview"
	"github.com/hamzayogurtcuoglu/i-mux/internal/editor"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/pager"
	"github.com/hamzayogurtcuoglu/i-mux/internal/searchview"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

func init() {
	register(&command{
		Name:   "__diff",
		Args:   "<status> <path>",
		Hidden: true,
		Run:    runDiff,
	})
	register(&command{
		Name:   "__search",
		Hidden: true,
		Run:    runSearch,
	})
	register(&command{
		Name:   "__edit",
		Args:   "[--line N] <path>",
		Hidden: true,
		Run:    runEdit,
	})
	register(&command{
		Name:   "__view",
		Args:   "[--line N] <path>",
		Hidden: true,
		Run:    runView,
	})
	register(&command{
		Name:   "__page",
		Args:   "<imux command...>",
		Hidden: true,
		Run:    runPage,
	})
}

// runDiff shows a changed file's diff the way an editor does (D26): the
// whole file, line numbers, colored changes, side by side when wide. It runs
// in a popup in the project folder.
func runDiff(e *Env, args []string) error {
	fs := newFlags(e, "__diff")
	staged := fs.Bool("staged", false, "the staged side (index against HEAD)")
	worktree := fs.Bool("worktree", false, "the unstaged side (work tree against the index)")
	compare := fs.Bool("compare", false, "compare two files: <a> <b>")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return &UsageError{Msg: i18n.T("cli.usage", "__diff <status> <path>")}
	}
	status, path := pos[0], pos[1]
	var lines []diffview.Line
	if *compare {
		lines = diffview.Parse(compareDiff(status, path))
		path = filepath.Base(status) + " ↔ " + filepath.Base(path)
	} else if status == "?" {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines = diffview.AllAdded(string(b))
	} else {
		mode := "head"
		switch {
		case *staged:
			mode = "staged"
		case *worktree:
			mode = "worktree"
		}
		lines = diffview.Parse(gitDiff(path, mode))
	}
	w, _ := sidebar.Size(int(os.Stdout.Fd()))
	rows, first := diffview.Render(lines, w-1, w >= 120) // a column for the scroll bar
	added, removed := diffview.Stats(lines)
	header := []string{fmt.Sprintf(" \x1b[1m%s\x1b[0m   \x1b[32m+%d\x1b[0m \x1b[31m−%d\x1b[0m", path, added, removed), ""}
	if len(rows) == 0 {
		rows = []string{"  " + i18n.T("view.no_diff")}
	}
	return page(e, header, rows, max(first-3, 0)) // a little context above the first change
}

// gitDiff is the file's whole-file diff: against HEAD ("head"; against the
// index in a repository without commits), the staged side or the work tree.
func gitDiff(path, mode string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	bases := [][]string{{"HEAD"}, {}}
	switch mode {
	case "staged":
		bases = [][]string{{"--cached"}}
	case "worktree":
		bases = [][]string{{}}
	}
	for _, base := range bases {
		args := append([]string{"diff", "--no-color", "--no-ext-diff", "-U1000000"}, base...)
		out, err := exec.CommandContext(ctx, "git", append(args, "--", path)...).Output()
		if err == nil {
			return string(out)
		}
	}
	return ""
}

// compareDiff is the whole-file diff of two files (Compare with Selected):
// `git diff --no-index`, else `diff -u`. Both exit 1 when the files differ.
func compareDiff(a, b string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, argv := range [][]string{
		{"git", "diff", "--no-index", "--no-color", "--no-ext-diff", "-U1000000", "--", a, b},
		{"diff", "-U1000000", a, b},
	} {
		out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).Output()
		var ee *exec.ExitError
		if err == nil || errors.As(err, &ee) && ee.ExitCode() == 1 {
			return string(out)
		}
	}
	return ""
}

// runEdit opens a file in the editor (D27), at --line; without an editor it
// shows the file.
func runEdit(e *Env, args []string) error {
	fs := newFlags(e, "__edit")
	line := fs.Int("line", 0, "line to open at")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "__edit [--line N] <path>")}
	}
	cfg, err := loadConfig(e)
	if err != nil {
		return err
	}
	argv, env := editorCmd(cfg, pos[0], *line)
	if offerMicro(cfg, argv) {
		askMicro(e, cfg, filepath.Base(argv[0]))
		argv, env = editorCmd(cfg, pos[0], *line)
	}
	if argv == nil {
		return runView(e, args)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// editorCmd is the editor argv and extra environment for path at line:
// Neovim with Copilot when AI completion is on and set up (D30), else the
// configured editor.
func editorCmd(cfg *config.Config, path string, line int) ([]string, []string) {
	if cfg.Completion.Enabled {
		if argv, env := completion.Editor(path, line); argv != nil {
			return argv, env
		}
	}
	return editor.Command(editor.Choose(cfg.Editor), path, line), nil
}

// runView shows a file with line numbers, at --line: bat when installed,
// else less -N.
func runView(e *Env, args []string) error {
	fs := newFlags(e, "__view")
	line := fs.Int("line", 0, "line to show first")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "__view [--line N] <path>")}
	}
	top := max(*line-5, 0)
	if bat, err := exec.LookPath("bat"); err == nil {
		args := []string{"--color=always", "--paging=never", "--style=numbers,changes"}
		if *line > 0 {
			args = append(args, "--highlight-line", strconv.Itoa(*line))
		}
		out, err := exec.Command(bat, append(args, "--", pos[0])...).Output()
		if err == nil {
			return pageFile(e, pos[0], pager.Text(string(out)), top)
		}
	}
	b, err := os.ReadFile(pos[0])
	if err != nil {
		return err
	}
	lines := pager.Text(string(b))
	for i, l := range lines {
		mark := "\x1b[38;5;243m"
		if i+1 == *line {
			mark = "\x1b[33m"
		}
		lines[i] = fmt.Sprintf("%s%5d\x1b[0m  %s", mark, i+1, l)
	}
	return pageFile(e, pos[0], lines, top)
}

// pageFile shows a file's lines in the pager, where v opens the editor at
// the first line shown.
func pageFile(e *Env, path string, lines []string, top int) error {
	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		return page(e, nil, lines, top)
	}
	p := &pager.Pager{Lines: lines, Top: top, Editable: true}
	if err := pager.Run(p, os.Stdin, os.Stdout); err != nil || !p.Edit {
		return err
	}
	return runEdit(e, []string{"--line", strconv.Itoa(p.Top + 1), path})
}

// runSearch is the project search popup (D26); a chosen result opens in the
// file viewer at its line.
func runSearch(e *Env, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	theme := sidebar.IconsEmoji
	if cfg, err := loadConfig(e); err == nil {
		theme = sidebar.IconTheme(cfg)
	}
	m, err := searchview.Run(searchview.Options{In: os.Stdin, Out: os.Stdout, Dir: dir, Theme: theme})
	if err != nil || m == nil {
		return err
	}
	return runEdit(e, []string{"--line", strconv.Itoa(m.Line), m.Path})
}

// ViewArgs is how a view command for path at line is passed to a popup.
func ViewArgs(self, path string, line int) []string {
	a := []string{self, "__view"}
	if line > 0 {
		a = append(a, "--line", strconv.Itoa(line))
	}
	return append(a, path)
}

// page shows drawn lines in i-mux's pager (§9.3 "Scroll bars") from line
// top, under header; printed as they are when not on a terminal.
func page(e *Env, header, lines []string, top int) error {
	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		for _, l := range append(header, lines...) {
			fmt.Fprintln(e.Stdout, l)
		}
		return nil
	}
	return pager.Run(&pager.Pager{Header: header, Lines: lines, Top: top}, os.Stdin, os.Stdout)
}

// runPage runs an imux command and shows what it prints, its errors too, in
// i-mux's pager: a popup's `imux ... 2>&1 | less` where the shell's pipe
// would not keep UTF-8 (PowerShell's re-encodes it).
func runPage(e *Env, args []string) error {
	if len(args) == 0 {
		return &UsageError{Msg: i18n.T("cli.usage", "__page <imux command...>")}
	}
	var out bytes.Buffer
	Run(&Env{Stdin: e.Stdin, Stdout: &out, Stderr: &out}, args)
	return page(e, nil, pager.Text(out.String()), 0)
}
