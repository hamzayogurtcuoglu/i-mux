package gitops

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestWorktreeHelpers (T77.1): a linked worktree knows its main one; the
// main one and a plain folder have none; branches, a clean tree, a merge.
func TestWorktreeHelpers(t *testing.T) {
	ctx := context.Background()
	dir := repo(t)
	if m := MainWorktree(ctx, dir); m != "" {
		t.Errorf("the main worktree has a main one: %q", m)
	}
	if m := MainWorktree(ctx, t.TempDir()); m != "" {
		t.Errorf("a plain folder: %q", m)
	}
	top, _ := TopLevel(ctx, dir)
	path := WorktreePath(top, "feat/x")
	if err := AddWorktree(ctx, dir, path, "feat/x", true); err != nil {
		t.Fatal(err)
	}
	if m := MainWorktree(ctx, path); !same(m, top) {
		t.Errorf("worktree's main %q, want %q", m, top)
	}
	if sub := filepath.Join(path, "sub"); os.MkdirAll(sub, 0o755) == nil && !same(MainWorktree(ctx, sub), top) {
		t.Error("from a subfolder of the worktree")
	}
	if CurrentBranch(ctx, path) != "feat/x" || CurrentBranch(ctx, dir) != "main" {
		t.Errorf("branches %q %q", CurrentBranch(ctx, path), CurrentBranch(ctx, dir))
	}
	if !BranchExists(ctx, dir, "feat/x") || BranchExists(ctx, dir, "nope") {
		t.Error("BranchExists")
	}
	if !Clean(ctx, path) {
		t.Error("a new worktree is not clean")
	}
	write(t, path, "new.go", "new\n")
	if Clean(ctx, path) {
		t.Error("an untracked file is clean")
	}
	if _, err := git(ctx, path, "add", "-A"); err != nil {
		t.Fatal(err)
	}
	if _, err := git(ctx, path, "commit", "-q", "-m", "new"); err != nil {
		t.Fatal(err)
	}
	if err := Merge(ctx, dir, "feat/x"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.go")); err != nil {
		t.Error("not merged into main")
	}
}
