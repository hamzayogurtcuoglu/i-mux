// Package gitops is the source control behind the sidebar (ARCHITECTURE D27):
// it reads a repository's staged and unstaged changes, branch, upstream and
// history, and runs the editor-like actions (stage, unstage, discard,
// commit, checkout) with the git CLI.
package gitops

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Change is one changed file on one side (staged or not).
type Change struct {
	Status    string // M, A, D, R, C, U, or ? (untracked)
	Path      string // relative to the folder git ran in
	Added     int
	Removed   int
	Untracked bool
}

// LogEntry is one history entry.
type LogEntry struct {
	Hash    string
	Subject string
	When    time.Time
}

// Status is a repository as the source control view shows it.
type Status struct {
	Branch   string // "" when detached
	Upstream string // "" without one
	Ahead    int
	Behind   int
	Staged   []Change
	Changes  []Change
	History  []LogEntry
}

// Timeout bounds every git call.
var Timeout = 10 * time.Second

// git runs git in dir and returns its stdout; stderr is the error text.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir, "-c", "color.ui=false"}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), errors.New(msg)
	}
	return out.String(), nil
}

// Read reads the repository that contains dir (an error outside one).
func Read(ctx context.Context, dir string) (*Status, error) {
	out, err := git(ctx, dir, "status", "--porcelain=v1", "-b", "-z")
	if err != nil {
		return nil, err
	}
	st := ParseStatus(out)
	staged, _ := git(ctx, dir, "diff", "--cached", "--numstat")
	work, _ := git(ctx, dir, "diff", "--numstat")
	addCounts(st.Staged, ParseNumstat(staged))
	addCounts(st.Changes, ParseNumstat(work))
	if log, err := git(ctx, dir, "log", "-n", "15", "--format=%h%x1f%s%x1f%ct"); err == nil {
		st.History = ParseLog(log)
	}
	return st, nil
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
			parseBranch(st, e[3:])
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
			st.Changes = append(st.Changes, Change{Status: "U", Path: path})
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

// ParseNumstat reads `git diff --numstat`: added and removed lines per path
// (binary files count 0; a rename keeps the new path).
func ParseNumstat(out string) map[string][2]int {
	m := map[string][2]int{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.SplitN(l, "\t", 3)
		if len(f) != 3 {
			continue
		}
		a, _ := strconv.Atoi(f[0])
		r, _ := strconv.Atoi(f[1])
		path := f[2]
		if i := strings.Index(path, "{"); i >= 0 {
			if j := strings.Index(path[i:], "}"); j > 0 {
				if _, after, ok := strings.Cut(path[i+1:i+j], " => "); ok {
					path = strings.ReplaceAll(path[:i]+after+path[i+j+1:], "//", "/")
				}
			}
		} else if _, after, ok := strings.Cut(path, " => "); ok {
			path = after
		}
		m[path] = [2]int{a, r}
	}
	return m
}

// ParseLog reads `git log --format=%h%x1f%s%x1f%ct`.
func ParseLog(out string) []LogEntry {
	var cs []LogEntry
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(l, "\x1f")
		if len(f) != 3 {
			continue
		}
		ts, _ := strconv.ParseInt(f[2], 10, 64)
		cs = append(cs, LogEntry{Hash: f[0], Subject: f[1], When: time.Unix(ts, 0)})
	}
	return cs
}

// Stage adds paths (deletions too) to the index.
func Stage(ctx context.Context, dir string, paths ...string) error {
	_, err := git(ctx, dir, append([]string{"add", "-A", "--"}, paths...)...)
	return err
}

// StageAll stages every change.
func StageAll(ctx context.Context, dir string) error {
	_, err := git(ctx, dir, "add", "-A")
	return err
}

// Unstage takes paths out of the index (keeping the work tree).
func Unstage(ctx context.Context, dir string, paths ...string) error {
	if _, err := git(ctx, dir, append([]string{"restore", "--staged", "--"}, paths...)...); err != nil {
		// A repository without commits has nothing to restore from.
		_, err2 := git(ctx, dir, append([]string{"rm", "--cached", "-r", "-q", "--"}, paths...)...)
		if err2 != nil {
			return err
		}
	}
	return nil
}

// UnstageAll empties the index back to HEAD.
func UnstageAll(ctx context.Context, dir string) error {
	if _, err := git(ctx, dir, "restore", "--staged", "--", "."); err != nil {
		_, err2 := git(ctx, dir, "rm", "--cached", "-r", "-q", "--", ".")
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
		_, err := git(ctx, dir, append([]string{"clean", "-f", "-q", "--"}, paths...)...)
		return err
	}
	_, err := git(ctx, dir, append([]string{"restore", "--worktree", "--"}, paths...)...)
	return err
}

// DiscardAll throws away every unstaged change, untracked files included.
func DiscardAll(ctx context.Context, dir string) error {
	if _, err := git(ctx, dir, "restore", "--worktree", "--", "."); err != nil && !strings.Contains(err.Error(), "did not match") {
		return err
	}
	_, err := git(ctx, dir, "clean", "-f", "-d", "-q", "--", ".")
	return err
}

// Commit commits the index with message; with nothing staged it stages
// every change first (as an editor's "commit all"). It reports whether it
// staged.
func Commit(ctx context.Context, dir, message string) (bool, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return false, errors.New("empty commit message")
	}
	staged := false
	if _, err := git(ctx, dir, "diff", "--cached", "--quiet"); err == nil {
		// Nothing staged (exit 0): stage everything.
		if err := StageAll(ctx, dir); err != nil {
			return false, err
		}
		staged = true
	}
	_, err := git(ctx, dir, "commit", "-q", "-m", message)
	return staged, err
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

// Checkout switches to branch.
func Checkout(ctx context.Context, dir, branch string) error {
	_, err := git(ctx, dir, "switch", branch)
	return err
}

// NewBranch creates branch from HEAD and switches to it.
func NewBranch(ctx context.Context, dir, branch string) error {
	_, err := git(ctx, dir, "switch", "-c", strings.TrimSpace(branch))
	return err
}
