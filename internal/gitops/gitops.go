// Package gitops is the source control behind the sidebar (ARCHITECTURE D27,
// D58): it reads a repository's changes, branch, upstream, the operation
// under way, its refs and its history as a graph, and runs VS Code's source
// control actions with the git CLI.
package gitops

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Change is one changed file on one side (staged or not), or a conflict.
type Change struct {
	Status    string // M, A, D, R, C, ? (untracked), or U (a conflict)
	Path      string // relative to the folder git ran in
	Added     int
	Removed   int
	Untracked bool
}

// Operations that can be under way (Status.Op).
const (
	OpMerge      = "merge"
	OpRebase     = "rebase"
	OpCherryPick = "cherry-pick"
	OpRevert     = "revert"
)

// Status is a repository as the source control view shows it.
type Status struct {
	Branch    string // "" when detached
	Upstream  string // "" without one
	Ahead     int
	Behind    int
	NoCommits bool     // the branch has no commit yet
	Op        string   // the operation under way (OpMerge ...), "" for none
	Conflicts []Change // unmerged paths: MERGE CHANGES
	Staged    []Change
	Changes   []Change
	Graph     []GraphRow // HEAD's and its upstream's history (HISTORY)
}

// Timeout bounds every git call.
var Timeout = 10 * time.Second

// git runs a read in dir and returns its stdout; stderr is the error text.
// Reads take no lock (NoLocks): the sidebar reads every few seconds, and an
// action run meanwhile would find .git/index.lock taken.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	out, _, err := gitOut(ctx, dir, NoLocksEnv, append(NoLocksArgs[:len(NoLocksArgs):len(NoLocksArgs)], args...)...)
	return out, err
}

// NoLocksEnv and NoLocksArgs make a git read take no lock: git status and
// git diff refresh the index as a side effect and write it back under
// .git/index.lock; git diff does even with GIT_OPTIONAL_LOCKS=0, unless
// diff.autoRefreshIndex is off. Every read i-mux runs in the background
// passes both. core.quotePath=false keeps non-ASCII paths as they are
// (UTF-8) where a read has no -z.
var (
	NoLocksEnv  = []string{"GIT_OPTIONAL_LOCKS=0"}
	NoLocksArgs = []string{"-c", "diff.autoRefreshIndex=false", "-c", "core.quotePath=false"}
)

// gitOut runs git in dir with env added and returns its stdout and stderr;
// on failure the error is stderr's text.
func gitOut(ctx context.Context, dir string, env []string, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir, "-c", "color.ui=false"}, args...)...)
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), errb.String(), errors.New(msg)
	}
	return out.String(), errb.String(), nil
}

// Read reads the repository that contains dir (an error outside one).
func Read(ctx context.Context, dir string) (*Status, error) {
	out, err := git(ctx, dir, "status", "--porcelain=v1", "-b", "-z")
	if err != nil {
		return nil, err
	}
	st := ParseStatus(out)
	staged, _ := git(ctx, dir, "diff", "--cached", "--numstat", "-z")
	work, _ := git(ctx, dir, "diff", "--numstat", "-z")
	addCounts(st.Staged, ParseNumstat(staged))
	addCounts(st.Changes, ParseNumstat(work))
	if gd, err := git(ctx, dir, "rev-parse", "--absolute-git-dir"); err == nil {
		st.Op = operation(strings.TrimSpace(gd))
	}
	if !st.NoCommits {
		st.Graph, _ = Graph(ctx, dir, st.Upstream != "", GraphLimit)
	}
	return st, nil
}

// operation is the operation under way in the repository whose git
// directory is gitDir ("" for none; `git am` counts as none).
func operation(gitDir string) string {
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(gitDir, name))
		return err == nil
	}
	switch {
	case has("rebase-merge"), has("rebase-apply") && !has(filepath.Join("rebase-apply", "applying")):
		return OpRebase
	case has("MERGE_HEAD"):
		return OpMerge
	case has("CHERRY_PICK_HEAD"):
		return OpCherryPick
	case has("REVERT_HEAD"):
		return OpRevert
	}
	return ""
}

func addCounts(cs []Change, n map[string][2]int) {
	for i := range cs {
		if v, ok := n[cs[i].Path]; ok {
			cs[i].Added, cs[i].Removed = v[0], v[1]
		}
	}
}

// ParseStatus reads `git status --porcelain=v1 -b -z`.
func ParseStatus(out string) *Status {
	st := &Status{}
	f := strings.Split(out, "\x00")
	for i := 0; i < len(f); i++ {
		e := f[i]
		if len(e) < 3 {
			continue
		}
		if strings.HasPrefix(e, "## ") {
			h := e[3:]
			st.NoCommits = strings.HasPrefix(h, "No commits yet on ") || strings.HasPrefix(h, "Initial commit on ")
			parseBranch(st, h)
			continue
		}
		x, y, path := e[0], e[1], e[3:]
		if x == 'R' || x == 'C' {
			i++ // the old path follows
		}
		if x == '?' && y == '?' {
			st.Changes = append(st.Changes, Change{Status: "?", Path: path, Untracked: true})
			continue
		}
		if x == '!' {
			continue
		}
		conflict := x == 'U' || y == 'U' || x == 'A' && y == 'A' || x == 'D' && y == 'D'
		if conflict {
			st.Conflicts = append(st.Conflicts, Change{Status: "U", Path: path})
			continue
		}
		if x != ' ' {
			st.Staged = append(st.Staged, Change{Status: string(x), Path: path})
		}
		if y != ' ' {
			st.Changes = append(st.Changes, Change{Status: string(y), Path: path})
		}
	}
	sortChanges := func(cs []Change) {
		sort.SliceStable(cs, func(i, j int) bool { return cs[i].Path < cs[j].Path })
	}
	sortChanges(st.Conflicts)
	sortChanges(st.Staged)
	sortChanges(st.Changes)
	return st
}

// parseBranch reads "main...origin/main [ahead 1, behind 2]",
// "No commits yet on main" or "HEAD (no branch)".
func parseBranch(st *Status, h string) {
	h = strings.TrimPrefix(h, "No commits yet on ")
	h = strings.TrimPrefix(h, "Initial commit on ")
	rest := ""
	if i := strings.Index(h, " ["); i >= 0 {
		h, rest = h[:i], h[i+2:]
	}
	if b, up, ok := strings.Cut(h, "..."); ok {
		st.Branch, st.Upstream = b, up
	} else if !strings.HasPrefix(h, "HEAD") {
		st.Branch = h
	}
	for _, part := range strings.Split(strings.TrimSuffix(rest, "]"), ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), " ")
		n, _ := strconv.Atoi(v)
		switch k {
		case "ahead":
			st.Ahead = n
		case "behind":
			st.Behind = n
		}
	}
}

// ParseNumstat reads `git diff --numstat -z`: added and removed lines per
// path (binary files count 0; a rename keeps the new path). With -z git
// writes paths as they are, never quoted: "12\t3\tpath\0", and a rename
// "4\t0\t\0old\0new\0".
func ParseNumstat(out string) map[string][2]int {
	m := map[string][2]int{}
	f := strings.Split(out, "\x00")
	for i := 0; i < len(f); i++ {
		p := strings.SplitN(f[i], "\t", 3)
		if len(p) != 3 {
			continue
		}
		a, _ := strconv.Atoi(p[0])
		r, _ := strconv.Atoi(p[1])
		path := p[2]
		if path == "" { // a rename: its old path, then its new one
			if i+2 >= len(f) {
				break
			}
			path, i = f[i+2], i+2
		}
		m[path] = [2]int{a, r}
	}
	return m
}

// Stage adds paths (deletions too) to the index.
func Stage(ctx context.Context, dir string, paths ...string) error {
	_, err := run(ctx, dir, append([]string{"add", "-A", "--"}, paths...)...)
	return err
}

// StageAll stages every change.
func StageAll(ctx context.Context, dir string) error {
	_, err := run(ctx, dir, "add", "-A")
	return err
}

// Unstage takes paths out of the index (keeping the work tree).
func Unstage(ctx context.Context, dir string, paths ...string) error {
	if _, err := run(ctx, dir, append([]string{"restore", "--staged", "--"}, paths...)...); err != nil {
		// A repository without commits has nothing to restore from.
		_, err2 := run(ctx, dir, append([]string{"rm", "--cached", "-r", "-q", "--"}, paths...)...)
		if err2 != nil {
			return err
		}
	}
	return nil
}

// UnstageAll empties the index back to HEAD.
func UnstageAll(ctx context.Context, dir string) error {
	if _, err := run(ctx, dir, "restore", "--staged", "--", "."); err != nil {
		_, err2 := run(ctx, dir, "rm", "--cached", "-r", "-q", "--", ".")
		if err2 != nil {
			return err
		}
	}
	return nil
}

// Discard throws away the work-tree changes of tracked paths and deletes
// untracked ones.
func Discard(ctx context.Context, dir string, untracked bool, paths ...string) error {
	if untracked {
		// -d as DiscardAll: an untracked folder ("?? dir/") goes too.
		_, err := run(ctx, dir, append([]string{"clean", "-f", "-d", "-q", "--"}, paths...)...)
		return err
	}
	_, err := run(ctx, dir, append([]string{"restore", "--worktree", "--"}, paths...)...)
	return err
}

// DiscardPath throws away every unstaged change under path (a folder):
// tracked files back to the index, untracked ones deleted.
func DiscardPath(ctx context.Context, dir, path string) error {
	if _, err := run(ctx, dir, "restore", "--worktree", "--", path); err != nil && !strings.Contains(err.Error(), "did not match") {
		return err
	}
	_, err := run(ctx, dir, "clean", "-f", "-d", "-q", "--", path)
	return err
}

// DiscardAll throws away every unstaged change, untracked files included.
func DiscardAll(ctx context.Context, dir string) error {
	if _, err := run(ctx, dir, "restore", "--worktree", "--", "."); err != nil && !strings.Contains(err.Error(), "did not match") {
		return err
	}
	_, err := run(ctx, dir, "clean", "-f", "-d", "-q", "--", ".")
	return err
}

// Commit commits the index with message; with nothing staged it stages
// every change first (as an editor's "commit all"). It reports whether it
// staged.
func Commit(ctx context.Context, dir, message string) (bool, error) {
	return CommitWith(ctx, dir, CommitOpts{Message: message})
}

// Branches lists the local branches, the current one first.
func Branches(ctx context.Context, dir string) ([]string, string, error) {
	out, err := git(ctx, dir, "branch", "--format=%(HEAD)%(refname:short)")
	if err != nil {
		return nil, "", err
	}
	var bs []string
	cur := ""
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "*") {
			cur = l[1:]
			bs = append([]string{cur}, bs...)
			continue
		}
		bs = append(bs, strings.TrimPrefix(l, " "))
	}
	return bs, cur, nil
}
