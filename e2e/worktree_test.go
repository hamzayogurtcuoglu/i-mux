//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestWorktree (M77, D72): the Agents "+" → Codex → 🌿 In a new worktree…
// asks a branch and opens <project>.worktrees/<branch> as a project with
// Codex in it, listed under the project; what the agent committed shows in
// the diff, merges into the base, and the worktree is removed with its
// project.
func TestWorktree(t *testing.T) {
	h := newH(t, sidebarConfig)
	p := h.Homes.Project
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t",
			"GIT_COMMITTER_EMAIL=t@example.com", "GIT_CONFIG_GLOBAL=/dev/null")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return string(out)
	}
	if err := os.WriteFile(filepath.Join(p, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(p, "init", "-q", "-b", "main")
	git(p, "add", "-A")
	git(p, "commit", "-q", "-m", "first")

	_, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)

	time.Sleep(500 * time.Millisecond)
	x, y := h.plusOf(bar, "AGENTS")
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "New agent with:", 5*time.Second)
	c.Write("x")
	c.waitScreenAfter(h, mark, "In a new worktree", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	c.Write("w")
	c.waitScreenAfter(h, mark, "Branch of the new worktree:", 5*time.Second)
	c.Write("fix-login\r")
	h.run("codex", 1)
	wt := filepath.Join(filepath.Dir(p), filepath.Base(p)+".worktrees", "fix-login")
	if b := strings.TrimSpace(git(wt, "branch", "--show-current")); b != "fix-login" {
		t.Fatalf("worktree branch %q", b)
	}
	session := ""
	h.waitFor("the worktree's project", 10*time.Second, func() bool {
		for _, l := range strings.Split(h.mustTmux("list-sessions", "-F", "#{session_name}\t#{session_path}"), "\n") {
			if f := strings.SplitN(l, "\t", 2); len(f) == 2 && strings.HasSuffix(f[1], "fix-login") {
				session = f[0]
			}
		}
		return session != ""
	})
	// Listed under its repository's project in a sidebar of the worktree.
	h.waitFor("🌿 under the project", 10*time.Second, func() bool {
		out := h.mustTmux("list-panes", "-s", "-t", "="+session, "-F", "#{pane_id} #{@imux_role}")
		for _, l := range strings.Split(out, "\n") {
			if f := strings.Fields(l); len(f) == 2 && f[1] == "sidebar" {
				s := h.screen(f[0])
				i, j := strings.Index(s, filepath.Base(p)), strings.Index(s, "🌿 fix-login")
				return i >= 0 && j > i
			}
		}
		return false
	})

	// The agent commits a file there.
	if err := os.WriteFile(filepath.Join(wt, "login.go"), []byte("package login\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(wt, "add", "-A")
	git(wt, "commit", "-q", "-m", "fix login")
	if out := h.mustImux("worktree", "diff", "--session", session); !strings.Contains(out, "login.go") || !strings.Contains(out, "fix-login against main") {
		t.Fatalf("diff:\n%s", out)
	}
	h.mustImux("worktree", "merge", "--session", session)
	if _, err := os.Stat(filepath.Join(p, "login.go")); err != nil {
		t.Fatal("not merged into main")
	}
	h.mustImux("worktree", "remove", "--session", session)
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Error("the worktree's folder is still there")
	}
	h.waitFor("the worktree's project closed", 5*time.Second, func() bool {
		return !strings.Contains(h.mustTmux("list-sessions", "-F", "#{session_name}"), session)
	})
	if !strings.Contains(git(p, "branch"), "fix-login") {
		t.Error("the branch went with the worktree")
	}
	_ = bar
}
