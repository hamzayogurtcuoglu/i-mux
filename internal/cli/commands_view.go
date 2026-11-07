package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/diffview"
	"github.com/hamzayogurtcuoglu/i-mux/internal/editor"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
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
}

// runDiff shows a changed file's diff the way an editor does (D26): the
// whole file, line numbers, colored changes, side by side when wide. It runs
// in a popup in the project folder.
func runDiff(e *Env, args []string) error {
	fs := newFlags(e, "__diff")
	staged := fs.Bool("staged", false, "the staged side (index against HEAD)")
	worktree := fs.Bool("worktree", false, "the unstaged side (work tree against the index)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 2 {
		return &UsageError{Msg: i18n.T("cli.usage", "__diff <status> <path>")}
	}
	status, path := pos[0], pos[1]
	var lines []diffview.Line
	if status == "?" {
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
	rows, first := diffview.Render(lines, w, w >= 120)
	added, removed := diffview.Stats(lines)
	var b bytes.Buffer
	fmt.Fprintf(&b, " \x1b[1m%s\x1b[0m   \x1b[32m+%d\x1b[0m \x1b[31m−%d\x1b[0m   \x1b[2m%s\x1b[0m\n\n", path, added, removed, i18n.T("view.hint"))
	if len(rows) == 0 {
		b.WriteString("  " + i18n.T("view.no_diff") + "\n")
	}
	for _, r := range rows {
		b.WriteString(r + "\n")
	}
	start := 1
	if first >= 0 {
		start = max(first+2-3, 1) // header lines; a little context above
	}
	return page(e, &b, "-R", "-S", "--mouse", "+"+strconv.Itoa(start))
}

func bytesBuffer(b []byte) *bytes.Buffer { return bytes.NewBuffer(b) }

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
	argv := editor.Command(editor.Choose(cfg.Editor), pos[0], *line)
	if argv == nil {
		return runView(e, args)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
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
	start := "+" + strconv.Itoa(max(*line-5, 1))
	if bat, err := exec.LookPath("bat"); err == nil {
		args := []string{"--color=always", "--paging=never", "--style=numbers,changes"}
		if *line > 0 {
			args = append(args, "--highlight-line", strconv.Itoa(*line))
		}
		out, err := exec.Command(bat, append(args, "--", pos[0])...).Output()
		if err == nil {
			return page(e, bytes.NewBuffer(out), "-R", "-S", "--mouse", start)
		}
	}
	return pageFile(e, pos[0], "-N", "-R", "-S", "--mouse", start)
}

// runSearch is the project search popup (D26); a chosen result opens in the
// file viewer at its line.
func runSearch(e *Env, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	m, err := searchview.Run(searchview.Options{In: os.Stdin, Out: os.Stdout, Dir: dir})
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

// page shows text in less (printed as is when less is missing).
func page(e *Env, text *bytes.Buffer, opts ...string) error {
	less, err := exec.LookPath("less")
	if err != nil {
		_, err := e.Stdout.Write(text.Bytes())
		return err
	}
	cmd := exec.Command(less, opts...)
	cmd.Env = lessEnv("-X -K")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = text, os.Stdout, os.Stderr
	return cmd.Run()
}

// lessEnv is less's environment: $LESS plus flags, and UTF-8 when no
// charset is set (under a C locale less shows "·" as <C2><B7>).
func lessEnv(flags string) []string {
	env := append(os.Environ(), "LESS="+strings.TrimSpace(os.Getenv("LESS")+" "+flags))
	if os.Getenv("LESSCHARSET") == "" {
		env = append(env, "LESSCHARSET=utf-8")
	}
	return env
}

// pageFile shows a file in less.
func pageFile(e *Env, path string, opts ...string) error {
	less, err := exec.LookPath("less")
	if err != nil {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = e.Stdout.Write(b)
		return err
	}
	cmd := exec.Command(less, append(opts, "--", path)...)
	cmd.Env = lessEnv("-K")
	if cfg, err := loadConfig(e); err == nil {
		if ed := editor.Choose(cfg.Editor); ed != nil {
			cmd.Env = append(cmd.Env, "VISUAL="+strings.Join(ed, " ")) // less's v edits
		}
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
