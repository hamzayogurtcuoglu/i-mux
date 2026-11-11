package commitmsg

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func repo(t *testing.T) (string, func(args ...string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir, git
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCollect(t *testing.T) {
	dir, git := repo(t)
	write(t, filepath.Join(dir, "a.txt"), "one\n")
	git("init", "-q", "-b", "main")
	git("add", "-A")
	git("commit", "-q", "-m", "feat: first")
	write(t, filepath.Join(dir, "a.txt"), "one\ntwo token=ghp_"+strings.Repeat("Ab1", 12)+"\n")
	write(t, filepath.Join(dir, "new.txt"), "hello new\n")
	write(t, filepath.Join(dir, "bin.dat"), "a\x00b")

	c, err := Collect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Staged || c.Empty() || c.Files() != 3 {
		t.Fatalf("work tree changes: %+v", c)
	}
	for _, want := range []string{"+two token=", "--- new file: new.txt", "hello new"} {
		if !strings.Contains(c.Patch, want) {
			t.Errorf("patch lacks %q:\n%s", want, c.Patch)
		}
	}
	if strings.Contains(c.Patch, "ghp_Ab1") || strings.Contains(c.Patch, "a\x00b") {
		t.Errorf("secret or binary content in the patch:\n%q", c.Patch)
	}
	if !slices.Equal(c.Untracked, []string{"bin.dat", "new.txt"}) || !slices.Equal(c.Subjects, []string{"feat: first"}) {
		t.Errorf("untracked %v, subjects %v", c.Untracked, c.Subjects)
	}

	git("add", "new.txt")
	c, err = Collect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Staged || c.Files() != 1 || !strings.Contains(c.Patch, "+hello new") || strings.Contains(c.Patch, "two") || len(c.Untracked) != 0 {
		t.Errorf("staged changes only: %+v", c)
	}

	git("commit", "-q", "-m", "x")
	git("add", "-A")
	git("commit", "-q", "-m", "y")
	if c, err = Collect(context.Background(), dir); err != nil || !c.Empty() {
		t.Errorf("clean tree: %+v %v", c, err)
	}
}

func TestCut(t *testing.T) {
	dir, git := repo(t)
	git("init", "-q", "-b", "main")
	write(t, filepath.Join(dir, "big.txt"), strings.Repeat("a line of text\n", 10000))
	git("add", "-A")
	c, err := Collect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Cut || len(c.Patch) > MaxPatch || !strings.HasSuffix(c.Patch, "\n") || len(c.Subjects) != 0 {
		t.Fatalf("cut %v, %d bytes", c.Cut, len(c.Patch))
	}
	b := Body(c)
	for _, want := range []string{"(no commits yet)", "The staged changes:", "big.txt", "[the rest of the patch is left out]"} {
		if !strings.Contains(b, want) {
			t.Errorf("body lacks %q", want)
		}
	}
}

func TestCommand(t *testing.T) {
	dir := "/tmp/x"
	for _, c := range []struct {
		id         session.AgentID
		model      string
		argv       []string
		stdinBody  bool // the body (not the instructions) on stdin
		stdinFull  bool
		outFile    bool
		promptLast bool
	}{
		{session.Claude, "haiku", []string{"claude", "-p", "--no-session-persistence", "--tools", "", "--model", "haiku"}, true, false, false, true},
		{session.Codex, "", []string{"codex", "exec", "--skip-git-repo-check", "--sandbox", "read-only", "--color", "never", "--output-last-message", filepath.Join(dir, "message.txt"), "-"}, false, true, true, false},
		{session.Copilot, "", []string{"copilot", "-p"}, false, false, false, false},
		{session.Antigravity, "m1", []string{"agy", "-p"}, false, false, false, false},
		{session.OpenCode, "groq/llama", []string{"opencode", "run", "--archived", "--title", "imux commit message", "-m", "groq/llama"}, false, false, false, true},
	} {
		bin := c.argv[0]
		r, err := Command(c.id, bin, c.model, dir, "INSTR", "BODY")
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(r.Argv[:len(c.argv)], c.argv) {
			t.Errorf("%s argv %q", c.id, r.Argv)
		}
		joined := strings.Join(r.Argv, "\x00")
		switch {
		case c.stdinBody:
			if r.Stdin != "BODY" || r.Argv[len(r.Argv)-1] != "INSTR" {
				t.Errorf("%s: instructions in argv, body on stdin: %q %q", c.id, r.Argv, r.Stdin)
			}
		case c.stdinFull:
			if r.Stdin != "INSTR\n\nBODY" || strings.Contains(joined, "BODY") {
				t.Errorf("%s: prompt on stdin: %q", c.id, r.Stdin)
			}
		default:
			if !strings.Contains(joined, "INSTR\n\nBODY") || r.Stdin != "" {
				t.Errorf("%s: prompt in argv: %q", c.id, r.Argv)
			}
		}
		if c.promptLast && !strings.HasPrefix(r.Argv[len(r.Argv)-1], "INSTR") {
			t.Errorf("%s: the prompt is not last: %q", c.id, r.Argv)
		}
		if (r.OutFile != "") != c.outFile {
			t.Errorf("%s out file %q", c.id, r.OutFile)
		}
		if c.model != "" && !strings.Contains(joined, c.model) {
			t.Errorf("%s: model missing: %q", c.id, r.Argv)
		}
	}
	if _, err := Command("shell", "sh", "", dir, "i", "b"); err == nil {
		t.Error("a shell cannot write messages")
	}
}

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"feat: add commit suggestions\n":                                       "feat: add commit suggestions",
		"Here is a commit message:\n\n```\nfix: keep the tab × clickable\n```": "fix: keep the tab × clickable",
		"\x1b[1m\"Add OpenCode round-trip tests\"\x1b[0m":                      "Add OpenCode round-trip tests",
		"Commit message: `docs: explain the panel`":                            "docs: explain the panel",
		"**refactor: split the sidebar buttons**":                              "refactor: split the sidebar buttons",
		"Sure! Here's a suggestion:\nchore: bump tmux":                         "chore: bump tmux",
		"   \n\n":                                    "",
		strings.Repeat("x", 300):                     strings.Repeat("x", MaxMessage),
		"feat: a, b and c\nwith a body line\n":       "feat: a, b and c",
		"Suggested commit message: test: add a case": "test: add a case",
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPick(t *testing.T) {
	cfg := config.Default()
	all := func(session.AgentID) bool { return true }
	only := func(ids ...session.AgentID) func(session.AgentID) bool {
		return func(id session.AgentID) bool { return slices.Contains(ids, id) }
	}
	if id, m, err := Pick(cfg, "codex", all); id != session.Codex || m != "" || err != nil {
		t.Errorf("auto takes the pane's agent: %s %q %v", id, m, err)
	}
	if id, _, err := Pick(cfg, "", only(session.Copilot, session.OpenCode)); id != session.Copilot || err != nil {
		t.Errorf("auto without a pane agent: first installed in order: %s %v", id, err)
	}
	if id, _, err := Pick(cfg, "claude", only(session.OpenCode)); id != session.OpenCode || err != nil {
		t.Errorf("auto skips a pane agent that is not installed: %s %v", id, err)
	}
	if _, _, err := Pick(cfg, "", only()); err == nil {
		t.Error("nothing installed")
	}
	cfg.Git.CommitAgent, cfg.Git.CommitModel = "opencode", "groq/llama"
	if id, m, err := Pick(cfg, "claude", all); id != session.OpenCode || m != "groq/llama" || err != nil {
		t.Errorf("named agent: %s %q %v", id, m, err)
	}
	if _, _, err := Pick(cfg, "claude", only(session.Claude)); err == nil {
		t.Error("a named agent that is not installed is an error")
	}
}

func TestExecute(t *testing.T) {
	dir := t.TempDir()
	// script writes the shell script body as name; on Windows the fake
	// program name (see TestMain) instead.
	script := func(name, body string) string {
		p := filepath.Join(dir, name)
		if runtime.GOOS == "windows" {
			return testutil.WriteFake(t, p, name, "")
		}
		write(t, p, "#!/bin/sh\n"+body)
		if err := os.Chmod(p, 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ctx := context.Background()
	ok := script("ok", "cat >/dev/null; echo 'Here is the message:'; echo; echo 'feat: from stdout'\n")
	if msg, err := Execute(ctx, Run{Argv: []string{ok}, Stdin: "x"}, dir, ""); err != nil || msg != "feat: from stdout" {
		t.Errorf("stdout: %q %v", msg, err)
	}
	file := script("file", "echo progress; printf 'fix: from the file\\n' > \"$1\"\n")
	out := filepath.Join(dir, "m.txt")
	if msg, err := Execute(ctx, Run{Argv: []string{file, out}, OutFile: out}, dir, ""); err != nil || msg != "fix: from the file" {
		t.Errorf("out file: %q %v", msg, err)
	}
	bad := script("bad", "echo 'Error: not logged in' >&2; exit 1\n")
	if _, err := Execute(ctx, Run{Argv: []string{bad}}, dir, ""); err == nil || err.Error() != "Error: not logged in" {
		t.Errorf("failure: %v", err)
	}
	empty := script("empty", "echo '```'; echo '```'\n")
	if _, err := Execute(ctx, Run{Argv: []string{empty}}, dir, ""); err == nil {
		t.Error("an empty reply is an error")
	}
	old := Timeout
	Timeout = 200_000_000 // 200 ms
	defer func() { Timeout = old }()
	slow := script("slow", "sleep 5\n")
	if _, err := Execute(ctx, Run{Argv: []string{slow}}, dir, ""); err == nil || !strings.Contains(err.Error(), "no reply") {
		t.Errorf("timeout: %v", err)
	}
}
