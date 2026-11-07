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
		lines = diffview.Parse(gitDiff(path))
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
	return page(e, &b, "-R", "-S", "+"+strconv.Itoa(start))
}

// gitDiff is the file's whole-file diff against HEAD (against the index in a
// repository without commits).
func gitDiff(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, base := range [][]string{{"HEAD"}, {}} {
		args := append([]string{"diff", "--no-color", "--no-ext-diff", "-U1000000"}, base...)
		out, err := exec.CommandContext(ctx, "git", append(args, "--", path)...).Output()
		if err == nil {
			return string(out)
		}
	}
	return ""
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
			return page(e, bytes.NewBuffer(out), "-R", "-S", start)
		}
	}
	return pageFile(e, pos[0], "-N", "-R", "-S", start)
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
	return runView(e, []string{"--line", strconv.Itoa(m.Line), m.Path})
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
	cmd.Env = append(os.Environ(), "LESS="+strings.TrimSpace(os.Getenv("LESS")+" -X -K"))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = text, os.Stdout, os.Stderr
	return cmd.Run()
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
	cmd.Env = append(os.Environ(), "LESS="+strings.TrimSpace(os.Getenv("LESS")+" -K"))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
