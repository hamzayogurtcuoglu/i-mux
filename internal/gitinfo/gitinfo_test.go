package gitinfo

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func needGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func TestRepo(t *testing.T) {
	needGit(t)
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "feature/x")
	for i := 0; i < 50; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%02d.txt", i)), []byte("a\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "init")
	for i := 0; i < 30; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%02d.txt", i)), []byte("a\nb\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 15; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("new%02d.txt", i)), []byte("n\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	info := Collect(context.Background(), dir)
	if info.State != OK || info.Branch != "feature/x" {
		t.Fatalf("%+v", info)
	}
	if len(info.Status) != MaxStatusLines || info.StatusTotal != 45 || info.StatusMore() != 5 {
		t.Fatalf("status %d/%d", len(info.Status), info.StatusTotal)
	}
	if len(info.DiffStat) != MaxDiffLines || info.DiffTotal != 31 || !strings.Contains(info.DiffStat[len(info.DiffStat)-1], "files changed") {
		t.Fatalf("diff %d/%d %q", len(info.DiffStat), info.DiffTotal, info.DiffStat[len(info.DiffStat)-1])
	}
}

func TestRepoWithoutCommits(t *testing.T) {
	needGit(t)
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	_ = os.WriteFile(filepath.Join(dir, "a"), []byte("x"), 0o600)
	info := Collect(context.Background(), dir)
	if info.State != OK || info.Branch != "main" || len(info.Status) != 1 {
		t.Fatalf("%+v", info)
	}
}

func TestNotRepo(t *testing.T) {
	needGit(t)
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(t.TempDir()))
	info := Collect(context.Background(), t.TempDir())
	if info.State != NotRepo {
		t.Fatalf("%+v", info)
	}
}

func TestNoGit(t *testing.T) {
	old := GitBinary
	GitBinary = "definitely-not-git-imux"
	defer func() { GitBinary = old }()
	if info := Collect(context.Background(), t.TempDir()); info.State != NoGit {
		t.Fatalf("%+v", info)
	}
}

func TestTimeout(t *testing.T) {
	dir := t.TempDir()
	slow := filepath.Join(dir, "slowgit")
	if err := os.WriteFile(slow, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldBin, oldT := GitBinary, Timeout
	GitBinary, Timeout = slow, 100*time.Millisecond
	defer func() { GitBinary, Timeout = oldBin, oldT }()
	start := time.Now()
	info := Collect(context.Background(), dir)
	if info.State != TimedOut || time.Since(start) > 3*time.Second {
		t.Fatalf("%+v after %v", info, time.Since(start))
	}
}
