package gitops

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RefInfo is a branch or tag as Checkout to and the other Quick Picks list
// them.
type RefInfo struct {
	Name     string // short: "main", "origin/main", "v1.0"
	Kind     string // RefBranch, RefRemote, RefTag
	Hash     string // short
	Subject  string // its commit's, or an annotated tag's
	When     time.Time
	Upstream string // a local branch's, short
	Current  bool   // the branch HEAD is on
}

// Refs lists the local branches, the remote-tracking branches (not the
// remotes' HEAD) and the tags, newest first.
func Refs(ctx context.Context, dir string) ([]RefInfo, error) {
	out, err := git(ctx, dir, "for-each-ref", "--sort=-creatordate",
		"--format=%(refname)%1f%(objectname:short)%1f%(subject)%1f%(creatordate:unix)%1f%(upstream:short)%1f%(HEAD)%1f%(symref)",
		"refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		return nil, err
	}
	return ParseRefs(out), nil
}

// ParseRefs reads Refs's for-each-ref output.
func ParseRefs(out string) []RefInfo {
	var rs []RefInfo
	for _, l := range strings.Split(out, "\n") {
		f := strings.Split(l, "\x1f")
		if len(f) != 7 || f[6] != "" {
			continue // a symbolic ref: origin/HEAD
		}
		r := RefInfo{Hash: f[1], Subject: f[2], Upstream: f[4], Current: f[5] == "*"}
		ts, _ := strconv.ParseInt(f[3], 10, 64)
		r.When = time.Unix(ts, 0)
		switch {
		case strings.HasPrefix(f[0], "refs/heads/"):
			r.Name, r.Kind = strings.TrimPrefix(f[0], "refs/heads/"), RefBranch
		case strings.HasPrefix(f[0], "refs/remotes/"):
			r.Name, r.Kind = strings.TrimPrefix(f[0], "refs/remotes/"), RefRemote
		case strings.HasPrefix(f[0], "refs/tags/"):
			r.Name, r.Kind = strings.TrimPrefix(f[0], "refs/tags/"), RefTag
		default:
			continue
		}
		rs = append(rs, r)
	}
	return rs
}

// Stash is one entry of `git stash list`.
type Stash struct {
	Ref     string // "stash@{0}"
	Message string // "On main: message", "WIP on main: 1a2b3c4 subject"
	When    time.Time
}

// Stashes lists the stashes, newest first.
func Stashes(ctx context.Context, dir string) ([]Stash, error) {
	out, err := git(ctx, dir, "stash", "list", "--format=%gd%x1f%gs%x1f%ct")
	if err != nil {
		return nil, err
	}
	var ss []Stash
	for _, l := range strings.Split(out, "\n") {
		f := strings.Split(l, "\x1f")
		if len(f) != 3 {
			continue
		}
		ts, _ := strconv.ParseInt(f[2], 10, 64)
		ss = append(ss, Stash{Ref: f[0], Message: f[1], When: time.Unix(ts, 0)})
	}
	return ss, nil
}

// Remote is a configured remote.
type Remote struct {
	Name     string
	FetchURL string
	PushURL  string
}

// Remotes lists the remotes (`git remote -v`).
func Remotes(ctx context.Context, dir string) ([]Remote, error) {
	out, err := git(ctx, dir, "remote", "-v")
	if err != nil {
		return nil, err
	}
	return ParseRemotes(out), nil
}

// ParseRemotes reads `git remote -v`: "origin\turl (fetch)".
func ParseRemotes(out string) []Remote {
	var rs []Remote
	idx := map[string]int{}
	for _, l := range strings.Split(out, "\n") {
		name, rest, ok := strings.Cut(l, "\t")
		if !ok {
			continue
		}
		url, kind, _ := strings.Cut(rest, " ")
		i, seen := idx[name]
		if !seen {
			i = len(rs)
			idx[name] = i
			rs = append(rs, Remote{Name: name})
		}
		if kind == "(push)" {
			rs[i].PushURL = url
		} else {
			rs[i].FetchURL = url
		}
	}
	return rs
}

// Worktree is one entry of `git worktree list`.
type Worktree struct {
	Path     string
	Head     string
	Branch   string // short; "" when detached or bare
	Main     bool   // the main worktree (listed first)
	Current  bool   // the one dir is in
	Bare     bool
	Detached bool
	Locked   bool
	Prunable bool
}

// Worktrees lists the repository's worktrees.
func Worktrees(ctx context.Context, dir string) ([]Worktree, error) {
	out, err := git(ctx, dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	top, _ := TopLevel(ctx, dir)
	return ParseWorktrees(out, top), nil
}

// ParseWorktrees reads `git worktree list --porcelain`; current is the
// working tree's top folder, which marks the current worktree.
func ParseWorktrees(out, current string) []Worktree {
	var ws []Worktree
	for _, block := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n\n") {
		var w Worktree
		for _, l := range strings.Split(block, "\n") {
			k, v, _ := strings.Cut(l, " ")
			switch k {
			case "worktree":
				w.Path = v
			case "HEAD":
				w.Head = v
			case "branch":
				w.Branch = strings.TrimPrefix(v, "refs/heads/")
			case "bare":
				w.Bare = true
			case "detached":
				w.Detached = true
			case "locked":
				w.Locked = true
			case "prunable":
				w.Prunable = true
			}
		}
		if w.Path == "" {
			continue
		}
		w.Main = len(ws) == 0
		w.Current = current != "" && filepath.Clean(w.Path) == filepath.Clean(current)
		ws = append(ws, w)
	}
	return ws
}

// TopLevel is the top folder of the working tree dir is in.
func TopLevel(ctx context.Context, dir string) (string, error) {
	out, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	return filepath.FromSlash(strings.TrimSpace(out)), err
}

// LastCommit is HEAD's subject and whole message.
func LastCommit(ctx context.Context, dir string) (subject, message string, err error) {
	out, err := git(ctx, dir, "log", "-1", "--format=%s%x00%B")
	if err != nil {
		return "", "", err
	}
	subject, message, _ = strings.Cut(out, "\x00")
	return subject, strings.TrimSpace(message), nil
}

// gitVersion caches `git version` as major*1000+minor.
var gitVersion struct {
	sync.Once
	v int
}

// AtLeast reports whether git is version major.minor or newer (false when
// git does not say).
func AtLeast(ctx context.Context, major, minor int) bool {
	gitVersion.Do(func() {
		out, err := git(ctx, ".", "version")
		if err != nil {
			return
		}
		f := strings.Fields(out) // git version 2.34.1 (Apple Git-...)
		if len(f) < 3 {
			return
		}
		p := strings.SplitN(f[2], ".", 3)
		if len(p) < 2 {
			return
		}
		ma, _ := strconv.Atoi(p[0])
		mi, _ := strconv.Atoi(p[1])
		gitVersion.v = ma*1000 + mi
	})
	return gitVersion.v >= major*1000+minor
}
