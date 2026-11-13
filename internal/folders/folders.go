// Package folders finds the project folders the open-a-project picker offers
// (ARCHITECTURE D22, §9.3 "Open and close projects"): recent ones, open
// ones, and git repositories under the home folder; it ranks them against
// what the user typed and lists subfolders for typed paths.
package folders

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Kind says where a candidate comes from.
type Kind int

// Kinds, in the order they are listed when nothing is typed.
const (
	KindOpen    Kind = iota // a project open now
	KindRecent              // started before
	KindGit                 // a git repository found under home
	KindDir                 // a folder listed while browsing a path
	KindRestore             // "Restore last session" (Path empty)
	KindHeader              // a section title (not selectable)
)

// Cand is one folder to offer.
type Cand struct {
	Path   string
	Kind   Kind
	Detail string // shown on the right ("3 tabs · 4 agents · 2h"); "" = the kind
	Label  string // the text instead of the path (headers, the restore row)
}

// Selectable reports whether the row can be chosen.
func (c Cand) Selectable() bool { return c.Kind != KindHeader }

// Skip are folder names never scanned into.
var Skip = map[string]bool{
	"Library": true, "Applications": true, "Pictures": true, "Music": true, "Movies": true,
	"node_modules": true, "vendor": true, "target": true, "dist": true, "build": true,
	"venv": true, "__pycache__": true, "Pods": true, "DerivedData": true,
}

// Excluded are the names the explorer, search and Go to File never show,
// as VS Code's files.exclude leaves them out by default: version control
// folders and the files the system writes (§9.3 "Explorer title and hidden
// files", D63).
var Excluded = map[string]bool{
	".git": true, ".svn": true, ".hg": true, "CVS": true, ".DS_Store": true, "Thumbs.db": true,
}

// Hidden reports whether the explorer leaves out the entry called name
// (dir: a folder): the Excluded names, the Skip folders and, unless
// showHidden, a name starting with a dot.
func Hidden(name string, dir, showHidden bool) bool {
	return Excluded[name] || dir && Skip[name] || !showHidden && strings.HasPrefix(name, ".")
}

// ExcludedPath reports whether search and Go to File leave out the file at
// the slash-separated path rel from git's list: one of its names is
// Excluded. Git already left out what the project ignores.
func ExcludedPath(rel string) bool {
	for _, n := range strings.Split(rel, "/") {
		if Excluded[n] {
			return true
		}
	}
	return false
}

// ScanOptions bound a scan.
type ScanOptions struct {
	Depth    int           // levels below root (default 4)
	MaxDirs  int           // folders visited at most (default 40000)
	MaxTime  time.Duration // default 2 s
	MaxFound int           // repositories returned at most (default 2000)
}

// Scan finds git repositories under root (a folder with a .git entry; its
// subfolders are not searched). Hidden folders and Skip are left out.
func Scan(root string, o ScanOptions) []string {
	if o.Depth <= 0 {
		o.Depth = 4
	}
	if o.MaxDirs <= 0 {
		o.MaxDirs = 40000
	}
	if o.MaxTime <= 0 {
		o.MaxTime = 2 * time.Second
	}
	if o.MaxFound <= 0 {
		o.MaxFound = 2000
	}
	deadline := time.Now().Add(o.MaxTime)
	visited := 0
	var found []string
	type item struct {
		dir   string
		depth int
	}
	queue := []item{{root, 0}} // breadth first: shallow repositories first
	for len(queue) > 0 && visited < o.MaxDirs && len(found) < o.MaxFound {
		it := queue[0]
		queue = queue[1:]
		visited++
		if visited%256 == 0 && time.Now().After(deadline) {
			break
		}
		ents, err := os.ReadDir(it.dir)
		if err != nil {
			continue
		}
		repo := false
		for _, e := range ents {
			if e.Name() == ".git" {
				repo = true
				break
			}
		}
		if repo && it.dir != root {
			found = append(found, it.dir)
			continue
		}
		if it.depth >= o.Depth {
			continue
		}
		for _, e := range ents {
			n := e.Name()
			if !e.IsDir() || strings.HasPrefix(n, ".") || Skip[n] {
				continue
			}
			queue = append(queue, item{filepath.Join(it.dir, n), it.depth + 1})
		}
	}
	sort.Strings(found)
	return found
}

// cache is ~/.imux/folders.json.
type cache struct {
	At    int64    `json:"at"`
	Repos []string `json:"repos"`
}

// CacheTTL is how long a scan is reused.
const CacheTTL = 10 * time.Minute

// Cached returns the repositories of the cache file and whether they are
// fresh.
func Cached(path string, now time.Time) ([]string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var c cache
	if json.Unmarshal(b, &c) != nil {
		return nil, false
	}
	return c.Repos, now.Sub(time.Unix(c.At, 0)) < CacheTTL
}

// Store writes the cache file.
func Store(path string, repos []string, now time.Time) error {
	b, err := json.Marshal(cache{At: now.Unix(), Repos: repos})
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Merge lists candidates without duplicates in kind order: open, recent,
// git. Folders that no longer exist are dropped.
func Merge(open, recent, git []string) []Cand {
	as := func(ps []string, k Kind) []Cand {
		out := make([]Cand, len(ps))
		for i, p := range ps {
			out[i] = Cand{Path: p, Kind: k}
		}
		return out
	}
	return MergeCands(as(open, KindOpen), as(recent, KindRecent), as(git, KindGit))
}

// MergeCands joins lists in order without duplicate paths (the first one
// wins); folders that no longer exist are dropped.
func MergeCands(lists ...[]Cand) []Cand {
	seen := map[string]bool{}
	var out []Cand
	for _, l := range lists {
		for _, c := range l {
			c.Path = filepath.Clean(c.Path)
			if c.Path == "" || c.Path == "." || seen[c.Path] {
				continue
			}
			if fi, err := os.Stat(c.Path); err != nil || !fi.IsDir() {
				continue
			}
			seen[c.Path] = true
			out = append(out, c)
		}
	}
	return out
}

// subsequence reports whether every rune of q appears in s in order.
func subsequence(q, s string) bool {
	r := []rune(s)
	i := 0
	for _, c := range q {
		for i < len(r) && r[i] != c {
			i++
		}
		if i == len(r) {
			return false
		}
		i++
	}
	return true
}

// Rank filters cands by query (case-insensitive, against shorten(path)) and
// orders them: name prefix, name substring, path substring, subsequence;
// within each, the original order (open, recent, git). An empty query keeps
// all.
func Rank(query string, cands []Cand, shorten func(string) string) []Cand {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return append([]Cand(nil), cands...)
	}
	type scored struct {
		c     Cand
		score int
		i     int
	}
	var out []scored
	for i, c := range cands {
		full := strings.ToLower(shorten(c.Path))
		name := strings.ToLower(filepath.Base(c.Path))
		score := -1
		switch {
		case strings.HasPrefix(name, q):
			score = 0
		case strings.Contains(name, q):
			score = 1
		case strings.Contains(full, q):
			score = 2
		case subsequence(q, full):
			score = 3
		}
		if score >= 0 {
			out = append(out, scored{c, score, i})
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].score != out[b].score {
			return out[a].score < out[b].score
		}
		return out[a].i < out[b].i
	})
	res := make([]Cand, len(out))
	for i, s := range out {
		res[i] = s.c
	}
	return res
}

// IsPath reports whether the query is a path to browse ("/…", "~", "~/…").
func IsPath(query string) bool {
	return strings.HasPrefix(query, "/") || query == "~" || strings.HasPrefix(query, "~/")
}

// Expand turns "~" and "~/x" into home paths.
func Expand(query, home string) string {
	switch {
	case query == "~":
		return home
	case strings.HasPrefix(query, "~/"):
		return home + query[1:] // not Join: "~/x/." must keep its "."
	}
	return query
}

// Browse lists the subfolders for a typed path: the folders in its
// directory part whose name starts with the rest (case-insensitive), hidden
// ones only when the rest starts with ".". "~/code/ac" lists ~/code/ac*.
func Browse(query, home string) []Cand {
	p := Expand(query, home)
	dir, prefix := p, ""
	if !strings.HasSuffix(query, "/") && query != "~" {
		i := strings.LastIndex(p, "/")
		dir, prefix = p[:i+1], p[i+1:]
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	lp := strings.ToLower(prefix)
	var out []Cand
	for _, e := range ents {
		n := e.Name()
		if strings.HasPrefix(n, ".") && !strings.HasPrefix(prefix, ".") {
			continue
		}
		if !strings.HasPrefix(strings.ToLower(n), lp) {
			continue
		}
		full := filepath.Join(dir, n)
		if fi, err := os.Stat(full); err != nil || !fi.IsDir() {
			continue
		}
		out = append(out, Cand{Path: full, Kind: KindDir})
	}
	sort.SliceStable(out, func(a, b int) bool {
		return strings.ToLower(filepath.Base(out[a].Path)) < strings.ToLower(filepath.Base(out[b].Path))
	})
	return out
}

// Complete is the query after Tab on cand while browsing: its path in the
// query's style (~ kept) with a trailing "/".
func Complete(query, home string, c Cand) string {
	p := c.Path
	if strings.HasPrefix(query, "~") && (p == home || strings.HasPrefix(p, home+"/")) {
		p = "~" + strings.TrimPrefix(p, home)
	}
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}

// printable drops control characters from typed or pasted text.
func printable(r rune) bool { return unicode.IsPrint(r) }

// Clean keeps the printable runes of s.
func Clean(s string) string {
	return strings.Map(func(r rune) rune {
		if printable(r) {
			return r
		}
		return -1
	}, s)
}
