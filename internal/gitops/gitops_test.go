package gitops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, ".gitconfig-none"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1") // Git for Windows turns core.autocrlf on there
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	run := func(args ...string) {
		t.Helper()
		if _, err := git(context.Background(), dir, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	write(t, dir, "a.go", "one\ntwo\n")
	write(t, dir, "b.go", "b\n")
	run("init", "-q", "-b", "main")
	run("add", "-A")
	run("commit", "-q", "-m", "init")
	return dir
}

func write(t *testing.T, dir, name, s string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(cs []Change) string {
	var n []string
	for _, c := range cs {
		n = append(n, c.Status+" "+c.Path)
	}
	return strings.Join(n, ", ")
}

func TestFlow(t *testing.T) {
	ctx := context.Background()
	dir := repo(t)
	write(t, dir, "a.go", "one\nTWO\nthree\n")
	write(t, dir, "new file.txt", "x\n")
	st, err := Read(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch != "main" || names(st.Staged) != "" || names(st.Changes) != "M a.go, ? new file.txt" {
		t.Fatalf("status %+v", st)
	}
	if c := st.Changes[0]; c.Added != 2 || c.Removed != 1 {
		t.Fatalf("numstat %+v", c)
	}
	if len(st.Graph) != 1 || st.Graph[0].Subject != "init" || st.Graph[0].Lanes != "●" || st.Op != "" {
		t.Fatalf("graph %+v", st.Graph)
	}

	// Stage one file; it moves to the staged side.
	if err := Stage(ctx, dir, "a.go"); err != nil {
		t.Fatal(err)
	}
	st, _ = Read(ctx, dir)
	if names(st.Staged) != "M a.go" || names(st.Changes) != "? new file.txt" || st.Staged[0].Added != 2 {
		t.Fatalf("after stage %+v", st)
	}
	// A further edit shows on both sides.
	write(t, dir, "a.go", "one\nTWO\nthree\nfour\n")
	st, _ = Read(ctx, dir)
	if names(st.Staged) != "M a.go" || names(st.Changes) != "M a.go, ? new file.txt" {
		t.Fatalf("both sides %+v", st)
	}
	// Discard the unstaged edit; the staged one stays.
	if err := Discard(ctx, dir, false, "a.go"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "a.go")); string(b) != "one\nTWO\nthree\n" {
		t.Fatalf("discard kept %q", b)
	}
	if err := Unstage(ctx, dir, "a.go"); err != nil {
		t.Fatal(err)
	}
	st, _ = Read(ctx, dir)
	if names(st.Staged) != "" || names(st.Changes) != "M a.go, ? new file.txt" {
		t.Fatalf("after unstage %+v", st)
	}
	// Commit with nothing staged stages everything.
	staged, err := Commit(ctx, dir, "second")
	if err != nil || !staged {
		t.Fatalf("commit %v %v", staged, err)
	}
	st, _ = Read(ctx, dir)
	if len(st.Staged)+len(st.Changes) != 0 || st.Graph[0].Subject != "second" {
		t.Fatalf("after commit %+v", st)
	}
	if _, err := Commit(ctx, dir, "  "); err == nil {
		t.Fatal("empty message")
	}

	// Untracked discard deletes the file; discard all cleans everything.
	write(t, dir, "tmp.txt", "x")
	if err := Discard(ctx, dir, true, "tmp.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "tmp.txt")); !os.IsNotExist(err) {
		t.Fatal("untracked file not removed")
	}
	write(t, dir, "b.go", "changed\n")
	write(t, dir, "junk.txt", "x")
	if err := StageAll(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if err := UnstageAll(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if err := DiscardAll(ctx, dir); err != nil {
		t.Fatal(err)
	}
	st, _ = Read(ctx, dir)
	if len(st.Staged)+len(st.Changes) != 0 {
		t.Fatalf("after discard all %+v", st)
	}

	// Branches.
	if err := NewBranch(ctx, dir, "feature/x"); err != nil {
		t.Fatal(err)
	}
	bs, cur, err := Branches(ctx, dir)
	if err != nil || cur != "feature/x" || len(bs) != 2 || bs[0] != "feature/x" {
		t.Fatalf("branches %v %q %v", bs, cur, err)
	}
	if err := Checkout(ctx, dir, "main"); err != nil {
		t.Fatal(err)
	}
	if st, _ := Read(ctx, dir); st.Branch != "main" {
		t.Fatalf("checkout %q", st.Branch)
	}
}

func TestParse(t *testing.T) {
	st := ParseStatus("## main...origin/main [ahead 2, behind 1]\x00M  staged.go\x00 M work.go\x00MM both.go\x00R  new.go\x00old.go\x00UU conflict.go\x00?? n.txt\x00")
	if st.Branch != "main" || st.Upstream != "origin/main" || st.Ahead != 2 || st.Behind != 1 {
		t.Fatalf("branch %+v", st)
	}
	if names(st.Staged) != "M both.go, R new.go, M staged.go" || names(st.Changes) != "M both.go, ? n.txt, M work.go" ||
		names(st.Conflicts) != "U conflict.go" || st.NoCommits {
		t.Fatalf("sides %q / %q / %q", names(st.Staged), names(st.Changes), names(st.Conflicts))
	}
	if st := ParseStatus("## No commits yet on main\x00"); st.Branch != "main" || !st.NoCommits {
		t.Fatalf("no commits %+v", st)
	}
	if st := ParseStatus("## HEAD (no branch)\x00"); st.Branch != "" {
		t.Fatalf("detached %+v", st)
	}
}
