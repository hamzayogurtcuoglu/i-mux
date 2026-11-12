package selfexec

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestChanged(t *testing.T) {
	p := filepath.Join(t.TempDir(), "imux")
	if err := os.WriteFile(p, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute)
	_ = os.Chtimes(p, old, old)
	fi, _ := os.Stat(p)
	s := Stamp{Path: p, Size: fi.Size(), Mod: fi.ModTime()}
	if s.Changed() {
		t.Fatal("unchanged file reported as changed")
	}
	// A new binary (written over a minute ago) is a change.
	if err := os.WriteFile(p, []byte("new binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(p, old.Add(time.Second), old.Add(time.Second))
	if !s.Changed() {
		t.Fatal("new binary not seen")
	}
	// Being written right now, empty, not executable or gone: not yet.
	_ = os.Chtimes(p, time.Now(), time.Now())
	if s.Changed() {
		t.Fatal("a file being written is not a change yet")
	}
	_ = os.Chtimes(p, old, old)
	_ = os.Chmod(p, 0o644)
	if s.Changed() {
		t.Fatal("not executable")
	}
	_ = os.Remove(p)
	if s.Changed() || (Stamp{}).Changed() {
		t.Fatal("gone or unknown")
	}
	if Take().Path == "" {
		t.Fatal("Take finds the test binary")
	}
}
