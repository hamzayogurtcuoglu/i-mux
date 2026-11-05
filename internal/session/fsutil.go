package session

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileInfo is a file found while listing a store.
type FileInfo struct {
	Path    string
	ModTime time.Time
	Size    int64
}

// ListFiles returns regular files in dir whose names match the predicate,
// newest first. A missing dir yields nil.
func ListFiles(dir string, match func(name string) bool) []FileInfo {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []FileInfo
	for _, e := range ents {
		if e.IsDir() || !match(e.Name()) {
			continue
		}
		fi, err := e.Info()
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		out = append(out, FileInfo{Path: filepath.Join(dir, e.Name()), ModTime: fi.ModTime(), Size: fi.Size()})
	}
	SortNewest(out)
	return out
}

// SortNewest sorts files by modification time, newest first (path breaks ties).
func SortNewest(fs []FileInfo) {
	sort.Slice(fs, func(i, j int) bool {
		if !fs[i].ModTime.Equal(fs[j].ModTime) {
			return fs[i].ModTime.After(fs[j].ModTime)
		}
		return fs[i].Path > fs[j].Path
	})
}

// SortRefs sorts refs newest first.
func SortRefs(rs []Ref) {
	sort.SliceStable(rs, func(i, j int) bool {
		if !rs[i].UpdatedAt.Equal(rs[j].UpdatedAt) {
			return rs[i].UpdatedAt.After(rs[j].UpdatedAt)
		}
		return rs[i].Path > rs[j].Path
	})
}

// HasSuffix returns a predicate matching names with the given suffix.
func HasSuffix(suffix string) func(string) bool {
	return func(n string) bool { return strings.HasSuffix(n, suffix) && !strings.HasPrefix(n, ".") }
}

// SameDir reports whether two directory paths refer to the same place,
// tolerating trailing slashes and symlinks (macOS /var → /private/var).
func SameDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b {
		return true
	}
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

// ParseTime parses an RFC 3339 timestamp, returning the zero time on error.
func ParseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Counter collects warning counts by kind and renders them.
type Counter map[string]int

// Add counts one occurrence.
func (c Counter) Add(kind string) { c[kind]++ }

// Warnings renders "N <kind>" strings in a stable order.
func (c Counter) Warnings() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, itoa(c[k])+" "+k)
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
