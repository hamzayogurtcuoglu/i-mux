package folders

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mk(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestScan(t *testing.T) {
	home := t.TempDir()
	mk(t, home,
		"code/acme-api/.git", "code/acme-api/sub/.git", // nested repo: not searched
		"code/blog/.git", "Documents/notes/.git",
		"code/node_modules/x/.git", "Library/y/.git", ".hidden/z/.git", // skipped
		"a/b/c/d/deep/.git", // depth 5
		"plain/folder")
	got := Scan(home, ScanOptions{})
	want := []string{filepath.Join(home, "Documents/notes"), filepath.Join(home, "code/acme-api"), filepath.Join(home, "code/blog")}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Scan\n%v\nwant\n%v", got, want)
	}
	if got := Scan(home, ScanOptions{Depth: 5}); len(got) != 4 {
		t.Fatalf("depth 5: %v", got)
	}
	if got := Scan(home, ScanOptions{MaxFound: 1}); len(got) != 1 {
		t.Fatalf("MaxFound: %v", got)
	}

	cache := filepath.Join(home, "folders.json")
	now := time.Unix(1_800_000_000, 0)
	if err := Store(cache, want, now); err != nil {
		t.Fatal(err)
	}
	if r, fresh := Cached(cache, now.Add(time.Minute)); !fresh || len(r) != 3 {
		t.Fatalf("cache %v %v", r, fresh)
	}
	if _, fresh := Cached(cache, now.Add(time.Hour)); fresh {
		t.Fatal("old cache is not fresh")
	}
}

func TestMergeAndRank(t *testing.T) {
	home := t.TempDir()
	mk(t, home, "code/acme-api", "code/blog", "work/acme-mobile", "Documents/notes-acme", "x/backend")
	p := func(s string) string { return filepath.Join(home, s) }
	cands := Merge([]string{p("code/blog")}, []string{p("code/acme-api"), p("code/blog"), p("gone")},
		[]string{p("work/acme-mobile"), p("Documents/notes-acme"), p("x/backend"), p("code/acme-api")})
	var kinds []string
	for _, c := range cands {
		kinds = append(kinds, filepath.Base(c.Path)+":"+string(rune('0'+c.Kind)))
	}
	if strings.Join(kinds, " ") != "blog:0 acme-api:1 acme-mobile:2 notes-acme:2 backend:2" {
		t.Fatalf("merge %v", kinds)
	}
	short := func(s string) string { return "~" + strings.TrimPrefix(s, home) }
	names := func(cs []Cand) string {
		var n []string
		for _, c := range cs {
			n = append(n, filepath.Base(c.Path))
		}
		return strings.Join(n, " ")
	}
	for q, want := range map[string]string{
		"":     "blog acme-api acme-mobile notes-acme backend",
		"acm":  "acme-api acme-mobile notes-acme", // prefix first, then substring
		"ACME": "acme-api acme-mobile notes-acme",
		"wrk":  "acme-mobile",   // subsequence of ~/work/acme-mobile
		"code": "blog acme-api", // path substring
		"zzz":  "",
	} {
		if got := names(Rank(q, cands, short)); got != want {
			t.Errorf("Rank(%q) = %q, want %q", q, got, want)
		}
	}
}

func TestBrowse(t *testing.T) {
	home := t.TempDir()
	mk(t, home, "code/acme-api", "code/Acme-web", "code/blog", "code/.cache")
	if err := os.WriteFile(filepath.Join(home, "code/acme.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	names := func(cs []Cand) string {
		var n []string
		for _, c := range cs {
			n = append(n, filepath.Base(c.Path))
		}
		return strings.Join(n, " ")
	}
	for q, want := range map[string]string{
		"~/code/":   "acme-api Acme-web blog", // no files, no hidden
		"~/code/ac": "acme-api Acme-web",
		"~/code/.":  ".cache",
		"~":         "code",
		home + "/c": "code",
		"~/nope/":   "",
	} {
		if got := names(Browse(q, home)); got != want {
			t.Errorf("Browse(%q) = %q, want %q", q, got, want)
		}
	}
	c := Browse("~/code/ac", home)[0]
	if got := Complete("~/code/ac", home, c); got != "~/code/acme-api/" {
		t.Errorf("Complete %q", got)
	}
	if got := Complete(home+"/code/ac", home, c); got != home+"/code/acme-api/" {
		t.Errorf("Complete abs %q", got)
	}
	if !IsPath("~") || !IsPath("/tmp") || !IsPath("~/x") || IsPath("acme") || IsPath("~acme") {
		t.Error("IsPath")
	}
	if Clean("a\x1bb\tc") != "abc" {
		t.Error("Clean")
	}
}
