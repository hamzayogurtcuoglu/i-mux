package gitops

import (
	"context"
	"crypto/sha256"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GraphLimit is how many commits HISTORY shows.
var GraphLimit = 50

// Kinds of refs.
const (
	RefHead   = "head"   // the branch HEAD is on, or HEAD itself when detached
	RefBranch = "branch" // a local branch
	RefRemote = "remote" // a remote-tracking branch
	RefTag    = "tag"
)

// Ref is a branch or tag a commit of the graph carries.
type Ref struct {
	Name string // "main", "origin/main", "v1.0"; "HEAD" when detached
	Kind string // RefHead, RefBranch, RefRemote, RefTag
}

// GraphRow is one row of the history graph: a commit with the lanes git
// drew left of it, or a row of lanes only (where branches fork or join).
type GraphRow struct {
	Lanes   string // in box-drawing glyphs: "●", "│ ●", "│╲"
	Hash    string // short; "" on a row of lanes only
	Full    string
	Subject string
	Author  string
	When    time.Time
	Refs    []Ref
	Out     bool // on HEAD, not on its upstream: not pushed
	In      bool // on the upstream, not on HEAD: incoming
}

// graphCache keeps each folder's last graph and the refs it was read for.
var graphCache = struct {
	sync.Mutex
	m map[string]graphEntry
}{m: map[string]graphEntry{}}

type graphEntry struct {
	key  [32]byte
	rows []GraphRow
}

// Graph reads the history of HEAD (and of its upstream when upstream is
// set) as a graph, n commits. It runs `git log --graph` again only when a
// ref moved: `git show-ref --head` keys the folder's last graph.
func Graph(ctx context.Context, dir string, upstream bool, n int) ([]GraphRow, error) {
	refs, _ := git(ctx, dir, "show-ref", "--head") // exit 1 without refs: still a key
	key := sha256.Sum256([]byte(refs + "\x00" + strconv.Itoa(n) + strconv.FormatBool(upstream)))
	graphCache.Lock()
	if e, ok := graphCache.m[dir]; ok && e.key == key {
		graphCache.Unlock()
		return e.rows, nil
	}
	graphCache.Unlock()
	args := []string{
		"log", "--graph", "--topo-order", "--decorate=full",
		"--format=%x00%H%x1f%h%x1f%s%x1f%an%x1f%ct%x1f%D", "-n", strconv.Itoa(n), "HEAD",
	}
	if upstream {
		args = append(args, "@{u}")
	}
	out, err := git(ctx, dir, args...)
	if err != nil {
		return nil, err
	}
	rows := ParseGraph(out)
	if upstream {
		if lr, err := git(ctx, dir, "rev-list", "--left-right", "HEAD...@{u}"); err == nil {
			MarkInOut(rows, lr)
		}
	}
	graphCache.Lock()
	graphCache.m[dir] = graphEntry{key: key, rows: rows}
	graphCache.Unlock()
	return rows, nil
}

// ParseGraph reads `git log --graph --decorate=full
// --format=%x00%H%x1f%h%x1f%s%x1f%an%x1f%ct%x1f%D`: the lanes are what
// comes before the NUL, a row without one has lanes only.
func ParseGraph(out string) []GraphRow {
	var rows []GraphRow
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if l == "" {
			continue
		}
		lanes, rest, commit := strings.Cut(l, "\x00")
		row := GraphRow{Lanes: drawLanes(lanes)}
		if commit {
			f := strings.Split(rest, "\x1f")
			if len(f) != 6 {
				continue
			}
			ts, _ := strconv.ParseInt(f[4], 10, 64)
			row.Full, row.Hash, row.Subject, row.Author, row.When = f[0], f[1], f[2], f[3], time.Unix(ts, 0)
			row.Refs = parseRefs(f[5])
		} else if row.Lanes == "" {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

// drawLanes turns git's graph characters into box-drawing glyphs.
func drawLanes(s string) string {
	r := strings.NewReplacer("*", "●", "|", "│", "/", "╱", "\\", "╲", "-", "─", "_", "─", ".", "·")
	return strings.TrimRight(r.Replace(s), " ")
}

// parseRefs reads %D with --decorate=full: "HEAD -> refs/heads/main,
// refs/remotes/origin/main, tag: refs/tags/v1". The remotes' HEAD and the
// stash are left out.
func parseRefs(d string) []Ref {
	var refs []Ref
	for _, p := range strings.Split(d, ", ") {
		p = strings.TrimSpace(p)
		switch {
		case p == "":
		case p == "HEAD":
			refs = append(refs, Ref{Name: "HEAD", Kind: RefHead})
		case strings.HasPrefix(p, "HEAD -> "):
			refs = append(refs, Ref{Name: strings.TrimPrefix(strings.TrimPrefix(p, "HEAD -> "), "refs/heads/"), Kind: RefHead})
		case strings.HasPrefix(p, "tag: "):
			refs = append(refs, Ref{Name: strings.TrimPrefix(strings.TrimPrefix(p, "tag: "), "refs/tags/"), Kind: RefTag})
		case strings.HasPrefix(p, "refs/heads/"):
			refs = append(refs, Ref{Name: strings.TrimPrefix(p, "refs/heads/"), Kind: RefBranch})
		case strings.HasPrefix(p, "refs/remotes/"):
			if name := strings.TrimPrefix(p, "refs/remotes/"); !strings.HasSuffix(name, "/HEAD") {
				refs = append(refs, Ref{Name: name, Kind: RefRemote})
			}
		}
	}
	return refs
}

// MarkInOut marks the rows `git rev-list --left-right HEAD...@{u}` lists:
// "<" commits are only on HEAD (not pushed), ">" only on the upstream.
func MarkInOut(rows []GraphRow, leftRight string) {
	side := map[string]byte{}
	for _, l := range strings.Split(leftRight, "\n") {
		if len(l) > 1 {
			side[l[1:]] = l[0]
		}
	}
	for i := range rows {
		switch side[rows[i].Full] {
		case '<':
			rows[i].Out = true
		case '>':
			rows[i].In = true
		}
	}
}
