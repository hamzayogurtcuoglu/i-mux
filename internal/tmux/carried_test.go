package tmux

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestRunsAsEngine (D88): an imux binary named imux-core is the engine.
func TestRunsAsEngine(t *testing.T) {
	for argv0, want := range map[string]bool{
		"imux-core": true, "/home/u/.imux/engine/imux-core": true, `C:\x\IMUX-CORE.EXE`: true,
		"imux": false, "/usr/local/bin/imux": false, "imux-core-old": false, "imux.exe": false,
	} {
		if got := RunsAsEngine(argv0); got != want {
			t.Errorf("RunsAsEngine(%q) = %v", argv0, got)
		}
	}
}

// TestCarriedEngineLink (D88): the engine is a hard link to imux, kept while
// it is imux and made again when imux was replaced (go install renames a
// new file into place).
func TestCarriedEngineLink(t *testing.T) {
	dir := t.TempDir()
	self := filepath.Join(dir, "bin", "imux")
	testutil.WriteFile(t, self, []byte("imux v1"))
	home := filepath.Join(dir, "engine")
	p, err := carriedEngine(self, home)
	if err != nil || p != filepath.Join(home, EngineName+sysx.ExeSuffix) {
		t.Fatalf("carriedEngine = %q, %v", p, err)
	}
	same := func() bool {
		a, _ := os.Stat(self)
		b, _ := os.Stat(p)
		return a != nil && b != nil && os.SameFile(a, b)
	}
	if !same() {
		t.Fatal("not a link to imux")
	}
	if _, err := carriedEngine(self, home); err != nil || !same() {
		t.Fatalf("second call: %v", err)
	}
	next := self + ".new"
	testutil.WriteFile(t, next, []byte("imux v2, longer"))
	if err := os.Rename(next, self); err != nil {
		t.Fatal(err)
	}
	if same() {
		t.Fatal("the old link is the new imux already")
	}
	if _, err := carriedEngine(self, home); err != nil || !same() {
		t.Fatalf("not made again after imux was replaced: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "imux v2, longer" {
		t.Fatalf("engine %q", b)
	}
	if left, _ := filepath.Glob(filepath.Join(home, "*.tmp")); len(left) != 0 {
		t.Fatalf("temporary files left: %v", left)
	}
}

// TestCarriedEngineCopy (D88): where no hard link can be made, a copy with
// imux's size and modification time, kept while they match and made again
// when they do not; a stale engine that cannot be removed (a running server
// holds it on Windows) is renamed to .old and removed by a later call.
func TestCarriedEngineCopy(t *testing.T) {
	oldLink, oldRemove := linkFile, removeFile
	t.Cleanup(func() { linkFile, removeFile = oldLink, oldRemove })
	linkFile = func(string, string) error { return errors.New("cross-device link") }

	dir := t.TempDir()
	self := filepath.Join(dir, "imux")
	testutil.WriteFile(t, self, []byte("imux v1"))
	at := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(self, at, at); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "engine")
	p, err := carriedEngine(self, home)
	if err != nil {
		t.Fatal(err)
	}
	ti, _ := os.Stat(p)
	if b, _ := os.ReadFile(p); string(b) != "imux v1" || !ti.ModTime().Equal(at) || !sysx.Executable(ti) {
		t.Fatalf("copy %q %v %v", b, ti.ModTime(), ti.Mode())
	}
	// Kept: nothing is written again.
	if err := os.Chmod(p, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := carriedEngine(self, home); err != nil {
		t.Fatal(err)
	}
	if ti, _ := os.Stat(p); ti.Mode().Perm() != 0o700 {
		t.Fatal("an up-to-date copy was made again")
	}

	// imux replaced, and the stale engine held: renamed, then removed later.
	testutil.WriteFile(t, self, []byte("imux v2"))
	removeFile = func(string) error { return errors.New("in use") }
	if _, err := carriedEngine(self, home); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "imux v2" {
		t.Fatalf("engine %q", b)
	}
	olds, _ := filepath.Glob(filepath.Join(home, "*.old"))
	if len(olds) != 1 {
		t.Fatalf("old engines %v", olds)
	}
	removeFile = oldRemove
	if _, err := carriedEngine(self, home); err != nil {
		t.Fatal(err)
	}
	if olds, _ := filepath.Glob(filepath.Join(home, "*.old")); len(olds) != 0 {
		t.Fatalf("old engines left: %v", olds)
	}
}

// TestEngineBinCarried (D88): with no imux-core beside imux nor on PATH, an
// imux that carries the engine runs it from ~/.imux/engine; one that does
// not (a test binary) names imux-core, as before.
func TestEngineBinCarried(t *testing.T) {
	h := testutil.Isolate(t)
	t.Setenv("PATH", t.TempDir())
	old := CarriesEngine
	t.Cleanup(func() { CarriesEngine = old })

	CarriesEngine = false
	if got := EngineBin(); got != EngineName {
		t.Fatalf("without the engine carried: %q", got)
	}
	CarriesEngine = true
	got := EngineBin()
	if got != filepath.Join(h.Imux, "engine", EngineName+sysx.ExeSuffix) {
		t.Fatalf("carried engine %q (home %s)", got, h.Imux)
	}
	self, _ := os.Executable()
	a, _ := os.Stat(self)
	b, err := os.Stat(got)
	if err != nil || (!os.SameFile(a, b) && a.Size() != b.Size()) {
		t.Fatalf("the engine is not this binary: %v", err)
	}
}
