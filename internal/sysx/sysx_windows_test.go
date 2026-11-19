//go:build windows

package sysx

import (
	"os"
	"path/filepath"
	"testing"
)

// A file runs on Windows when PATHEXT names its extension: file modes have
// no execute bits there.
func TestExecutableByExtension(t *testing.T) {
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	dir := t.TempDir()
	for name, want := range map[string]bool{"imux.exe": true, "claude.CMD": true, "notes.txt": false, "imux": false} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := Executable(fi); got != want {
			t.Errorf("Executable(%s) = %v, want %v", name, got, want)
		}
	}
}
