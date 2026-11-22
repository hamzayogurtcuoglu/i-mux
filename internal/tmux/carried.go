package tmux

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
)

// CarriesEngine reports that this binary carries i-mux-core (D88): a real
// imux build (cmd/imux sets it) runs as the engine when its file name is
// imux-core. Test binaries do not, so their engine lookups stay as they are.
var CarriesEngine bool

// EngineName is the engine's file name ("imux-core", "imux-core.exe").
const EngineName = "imux-core"

// RunsAsEngine reports whether argv0 names the engine: an imux binary
// linked or copied as imux-core runs as i-mux-core (D88).
func RunsAsEngine(argv0 string) bool {
	base := filepath.Base(argv0)
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:] // a Windows path, whatever the system
	}
	return strings.TrimSuffix(strings.ToLower(base), ".exe") == EngineName
}

// linkFile and removeFile are os.Link and os.Remove; tests replace them
// (a volume without hard links, a file Windows keeps).
var (
	linkFile   = os.Link
	removeFile = os.Remove
)

// carriedDir is where i-mux makes the engine from imux itself.
func carriedDir() string { return filepath.Join(paths.Home(), "engine") }

// carriedEngine is the engine made from the running imux (D88): dir's
// imux-core, a hard link to self or, where a link cannot be made (another
// volume), a copy with self's size and modification time. It is made again
// when it no longer is self (imux was replaced, by go install or an
// update); a stale one a running engine server still holds (Windows) is
// renamed to .old first and removed on a later call.
func carriedEngine(self, dir string) (string, error) {
	target := filepath.Join(dir, EngineName+sysx.ExeSuffix)
	removeOld(dir)
	si, err := os.Stat(self)
	if err != nil {
		return "", err
	}
	if ti, err := os.Stat(target); err == nil {
		if os.SameFile(si, ti) || (ti.Mode().IsRegular() && ti.Size() == si.Size() && ti.ModTime().Equal(si.ModTime())) {
			return target, nil
		}
		if err := removeFile(target); err != nil {
			old := fmt.Sprintf("%s.%d.old", target, time.Now().UnixNano())
			if err := os.Rename(target, old); err != nil {
				return "", err
			}
		}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	// Made under a name of its own and renamed into place, so two imux
	// starting together never see half a file.
	tmp := fmt.Sprintf("%s.%d.tmp", target, os.Getpid())
	_ = os.Remove(tmp)
	if err := linkFile(self, tmp); err != nil {
		if err := copyExe(self, tmp, si); err != nil {
			_ = os.Remove(tmp)
			return "", err
		}
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return target, nil
}

// copyExe copies self to dst as an executable with self's modification
// time, which tells a later call the copy is still self.
func copyExe(self, dst string, si os.FileInfo) error {
	in, err := os.Open(self)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(dst, si.ModTime(), si.ModTime())
}

// removeOld removes the stale engines renamed to .old once nothing holds
// them (Windows keeps a running program's file).
func removeOld(dir string) {
	olds, _ := filepath.Glob(filepath.Join(dir, EngineName+"*.old"))
	for _, o := range olds {
		_ = os.Remove(o)
	}
}
