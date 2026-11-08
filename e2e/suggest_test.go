//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestCommitSuggest (T27.4): ✨ in Source Control asks once whether the
// changes may go to the pane's agent, runs it in its print mode with the
// diff, and offers the message: Commit commits it; Write another asks again;
// Cancel commits nothing; Edit… opens the editor and commits what is saved.
func TestCommitSuggest(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	// A fake editor that rewrites the message, keeping a comment line.
	ed := filepath.Join(t.TempDir(), "fake-editor")
	testutil.WriteFile(t, ed, []byte("#!/bin/sh\nfor f in \"$@\"; do last=\"$f\"; done\nprintf 'fix: edited by hand\\n\\nwith a body line\\n# a comment\\n' >\"$last\"\n"))
	if err := os.Chmod(ed, 0o755); err != nil {
		t.Fatal(err)
	}
	h := newH(t, "editor = \""+ed+"\"\n"+sidebarConfig)
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		cmd.Dir = h.Homes.Project
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "a.txt"), []byte("one\n"))
	git("init", "-q", "-b", "main")
	git("add", "-A")
	git("commit", "-q", "-m", "feat: init")
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "a.txt"), []byte("one\ntwo\n"))
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")

	_, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	x, y := h.rowOf(bar, "🔀")
	c.Click(0, x, y)
	h.rowOf(bar, "CHANGES  1")
	time.Sleep(500 * time.Millisecond)

	// ✨ → the first-use question → yes → the offer → Commit.
	x, y = h.rowOf(bar, "✨")
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Write the commit message with Claude Code?", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("y")
	c.waitScreenAfter(h, mark, "Commit message · Claude Code", 10*time.Second)
	c.waitScreenAfter(h, mark, "feat: fake claude commit message 1", 5*time.Second)
	argv := splitNUL(mustRead(t, filepath.Join(h.Log, "claude-headless-1.argv")))
	if len(argv) < 5 || argv[0] != "-p" || argv[1] != "--no-session-persistence" || argv[2] != "--tools" || argv[3] != "" ||
		!strings.HasPrefix(argv[len(argv)-1], "Write a git commit message") {
		t.Fatalf("claude print-mode argv %q", argv)
	}
	if in := string(mustRead(t, filepath.Join(h.Log, "claude-headless-1.stdin"))); !strings.Contains(in, "+two") ||
		!strings.Contains(in, "a.txt") || !strings.Contains(in, "- feat: init") {
		t.Fatalf("claude was not given the changes:\n%s", in)
	}
	time.Sleep(300 * time.Millisecond)
	c.Write("c")
	h.waitFor("committed", 5*time.Second, func() bool {
		return strings.Contains(git("log", "-n", "1", "--format=%s"), "feat: fake claude commit message 1")
	})

	// Again: no question; Write another, then Cancel commits nothing.
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "b.txt"), []byte("new\n"))
	h.rowOf(bar, "CHANGES  1")
	time.Sleep(500 * time.Millisecond)
	x, y = h.rowOf(bar, "✨")
	mark = c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "feat: fake claude commit message 2", 10*time.Second)
	if strings.Contains(stripANSI(c.Output()[mark:]), "Write the commit message with") {
		t.Fatal("asked twice")
	}
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("r")
	c.waitScreenAfter(h, mark, "feat: fake claude commit message 3", 10*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("q")
	time.Sleep(500 * time.Millisecond)
	if v := strings.TrimSpace(h.mustTmux("show-options", "-gqv", "@imux_git")); v != "" {
		t.Fatalf("@imux_git left after Cancel: %q", v)
	}
	if strings.Contains(git("log", "--format=%s"), "message 3") {
		t.Fatal("Cancel committed")
	}

	// Edit… opens the editor; what it saves is committed, comments left out.
	time.Sleep(300 * time.Millisecond)
	x, y = h.rowOf(bar, "✨")
	mark = c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "feat: fake claude commit message 4", 10*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("e")
	h.waitFor("committed after editing", 10*time.Second, func() bool {
		return git("log", "-n", "1", "--format=%B") == "fix: edited by hand\n\nwith a body line\n\n"
	})
	if out := git("show", "--stat", "--format=", "HEAD"); !strings.Contains(out, "b.txt") {
		t.Fatalf("the untracked file was not committed: %s", out)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
