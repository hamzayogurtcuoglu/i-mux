package search

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	files := Files(context.Background(), dir) // no git: a walk
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

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
