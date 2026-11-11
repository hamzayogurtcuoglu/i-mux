// Package selfexec lets imux's long-running processes (the watcher, the
// sidebars) notice that the imux binary was replaced (an upgrade) and
// restart themselves with it, so no old version keeps running unseen.
package selfexec

import (
	"os"
	"path/filepath"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
)

// Stamp identifies the executable file a process started from.
type Stamp struct {
	Path string
	Size int64
	Mod  time.Time
}

// Take records the running executable ("" Path when it cannot be found).
func Take() Stamp {
	p, err := os.Executable()
	if err != nil {
		return Stamp{}
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	fi, err := os.Stat(p)
	if err != nil {
		return Stamp{}
	}
	return Stamp{Path: p, Size: fi.Size(), Mod: fi.ModTime()}
}

// Changed reports whether another executable file now sits at the path: a
// new binary, complete (non-empty and executable).
func (s Stamp) Changed() bool {
	if s.Path == "" {
		return false
	}
	fi, err := os.Stat(s.Path)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() == 0 || !sysx.Executable(fi) {
		return false
	}
	if time.Since(fi.ModTime()) < time.Second {
		return false // still being written
	}
	return fi.Size() != s.Size || !fi.ModTime().Equal(s.Mod)
}

// Exec replaces the process with the executable at the path, with args
// (os.Args) and the same environment. It returns only on failure.
func (s Stamp) Exec(args []string) error {
	return sysx.Exec(s.Path, args, os.Environ())
}
