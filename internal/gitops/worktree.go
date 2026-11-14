package gitops

import (
	"context"
	"path/filepath"
	"strings"
)

// MainWorktree is the main worktree of the repository dir is in, when dir
// is in a linked worktree (git worktree add); "" for a main worktree, or a
// folder outside git (D72).
func MainWorktree(ctx context.Context, dir string) string {
	out, err := git(ctx, dir, "rev-parse", "--git-common-dir", "--show-toplevel")
	if err != nil {
		return ""
	}
	f := strings.Split(strings.TrimSpace(out), "\n")
	if len(f) != 2 {
		return ""
	}
	common, top := f[0], f[1]
	if !filepath.IsAbs(common) {
		common = filepath.Join(dir, common)
	}
	common = filepath.Clean(common)
	if filepath.Base(common) != ".git" {
		return "" // a bare repository's worktree: no main one to go back to
	}
	main := filepath.Dir(common)
	if same(main, top) {
		return ""
	}
	return main
}

// same reports two paths of one folder (macOS's /var is /private/var).
func same(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

// CurrentBranch is the branch checked out in dir ("" when detached).
func CurrentBranch(ctx context.Context, dir string) string {
	out, err := git(ctx, dir, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// BranchExists reports a local branch named name.
func BranchExists(ctx context.Context, dir, name string) bool {
	_, err := git(ctx, dir, "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// Clean reports a work tree without changes (untracked files count).
func Clean(ctx context.Context, dir string) bool {
	out, err := git(ctx, dir, "status", "--porcelain")
	return err == nil && strings.TrimSpace(out) == ""
}

// Merge merges branch into the branch checked out in dir.
func Merge(ctx context.Context, dir, branch string) error {
	_, err := run(ctx, dir, "merge", "--no-edit", branch)
	return err
}
