package search

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func tree(t *testing.T) string {
	dir := t.TempDir()
	w := func(p, s string) {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("a.go", "package a\nfunc Foo() {}\n// foo bar\n")
	w("sub/b.txt", "nothing\nFOO upper\n")
	w("node_modules/x.js", "foo\n")
	w(".hidden/h.txt", "foo\n")
	w("bin.dat", "foo\x00bar")
	return dir
}

func TestSearch(t *testing.T) {
	dir := tree(t)
	files := Files(context.Background(), dir, false) // no git: a walk
	if strings.Join(files, ",") != "a.go,bin.dat,sub/b.txt" {
		t.Fatalf("files %v", files)
	}
	got, cut := Search(context.Background(), dir, files, "foo")
	var s []string
	for _, m := range got {
		s = append(s, m.Path+":"+itoa(m.Line)+":"+itoa(m.Col))
	}
	// Smart case: lowercase matches every case; binary files are skipped.
	if strings.Join(s, " ") != "a.go:2:5 a.go:3:3 sub/b.txt:2:0" || cut {
		t.Fatalf("matches %v", s)
	}
	if got, _ := Search(context.Background(), dir, files, "FOO"); len(got) != 1 || got[0].Path != "sub/b.txt" || got[0].Len != 3 {
		t.Fatalf("case-sensitive %+v", got)
	}
	if got, _ := Search(context.Background(), dir, files, ""); got != nil {
		t.Fatal("empty query")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, cut := Search(ctx, dir, files, "foo"); !cut {
		t.Fatal("cancelled search is cut short")
	}
}

// TestFilesHidden (T68.1): search and Go to File find what they found
// before with hidden files not shown (git's list, dot files included; the
// walk's hidden files, not its hidden folders) and look into hidden folders
// outside git while they are shown; .git, the system's files and (outside
// git) the skip list never; in a repository git's list keeps what the
// project tracks in a skip-list folder.
func TestFilesHidden(t *testing.T) {
	testutil.Isolate(t) // no global gitignore
	ctx := context.Background()
	dir := t.TempDir()
	for _, p := range []string{
		"a.go", ".env", ".github/workflows/ci.yml", ".DS_Store", "CVS/Entries", "Thumbs.db",
		"vendor/v.go", ".gitignore", "ignored.log", "sub/.hidden.txt",
	} {
		testutil.WriteFile(t, filepath.Join(dir, p), []byte(p+"\n"))
	}
	testutil.WriteFile(t, filepath.Join(dir, ".gitignore"), []byte("*.log\n"))
	join := func(show bool) string { return strings.Join(Files(ctx, dir, show), ",") }
	// No git: a walk.
	if got := join(false); got != ".env,.gitignore,a.go,ignored.log,sub/.hidden.txt" {
		t.Errorf("walk: %s", got)
	}
	if got := join(true); got != ".env,.github/workflows/ci.yml,.gitignore,a.go,ignored.log,sub/.hidden.txt" {
		t.Errorf("walk, hidden files shown: %s", got)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "a.go", ".github", "vendor"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if got := join(false); got != ".env,.github/workflows/ci.yml,.gitignore,a.go,sub/.hidden.txt,vendor/v.go" {
		t.Errorf("git: %s", got)
	}
	if got := join(true); got != ".env,.github/workflows/ci.yml,.gitignore,a.go,sub/.hidden.txt,vendor/v.go" {
		t.Errorf("git, hidden files shown: %s", got)
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
