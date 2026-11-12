// Package search finds text in a project's files, the way an editor's search
// view does (ARCHITECTURE D26, §9.3 "Sidebar v6").
package search

import (
	"bufio"
	"bytes"
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/folders"
)

// Limits.
const (
	MaxFileBytes = 2 << 20
	MaxMatches   = 2000
	MaxFiles     = 50000
)

// Match is one matching line.
type Match struct {
	Path string // relative to the project folder
	Line int
	Col  int // rune offset of the match in Text
	Len  int // match length in runes
	Text string
}

// Files lists the project's files, relative to dir: git's tracked and
// untracked (not ignored) files in a repository, else a walk that skips
// hidden folders and the explorer's skip list.
func Files(ctx context.Context, dir string) []string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "ls-files", "-co", "--exclude-standard", "-z")
	if out, err := cmd.Output(); err == nil {
		var files []string
		for _, f := range strings.Split(string(out), "\x00") {
			if f != "" && len(files) < MaxFiles {
				files = append(files, f)
			}
		}
		sort.Strings(files)
		return files
	}
	var files []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || len(files) >= MaxFiles {
			return filepath.SkipDir
		}
		n := d.Name()
		if d.IsDir() {
			if p != dir && (strings.HasPrefix(n, ".") || folders.Skip[n]) {
				return filepath.SkipDir
			}
			return nil
		}
		if rel, err := filepath.Rel(dir, p); err == nil {
			files = append(files, rel)
		}
		return nil
	})
	sort.Strings(files)
	return files
}

// smartCase reports whether a query is case-sensitive: it has an uppercase
// letter.
func smartCase(q string) bool {
	for _, r := range q {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}

// Search finds query in files (relative to dir). The search stops at
// MaxMatches or when ctx ends; it reports whether it was cut short.
func Search(ctx context.Context, dir string, files []string, query string) ([]Match, bool) {
	if query == "" {
		return nil, false
	}
	sensitive := smartCase(query)
	q := query
	if !sensitive {
		q = strings.ToLower(query)
	}
	qlen := utf8.RuneCountInString(query)
	var out []Match
	for _, f := range files {
		if ctx.Err() != nil {
			return out, true
		}
		p := filepath.Join(dir, f)
		fi, err := os.Stat(p)
		if err != nil || !fi.Mode().IsRegular() || fi.Size() > MaxFileBytes {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil || bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0 {
			continue // binary
		}
		sc := bufio.NewScanner(bytes.NewReader(b))
		sc.Buffer(make([]byte, 64*1024), MaxFileBytes)
		for n := 1; sc.Scan(); n++ {
			line := sc.Text()
			hay := line
			if !sensitive {
				hay = strings.ToLower(line)
			}
			i := strings.Index(hay, q)
			if i < 0 {
				continue
			}
			out = append(out, Match{Path: f, Line: n, Col: utf8.RuneCountInString(hay[:i]), Len: qlen, Text: line})
			if len(out) >= MaxMatches {
				return out, true
			}
		}
	}
	return out, false
}
