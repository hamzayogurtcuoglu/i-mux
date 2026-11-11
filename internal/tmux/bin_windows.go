//go:build windows

package tmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// Name is how messages and `imux versions` call the multiplexer: on Windows
// imux runs on psmux, a native tmux for Windows (ConPTY) that speaks tmux's
// commands, formats and config.
const Name = "psmux"

var defaultBin = sync.OnceValue(findPsmux)

// DefaultBin is the multiplexer imux runs when IMUX_TMUX_BIN is not set:
// psmux from PATH, else where imux's installer or winget put it. Another
// program called tmux on PATH is not used: tmux ports for Windows cannot run
// the shell commands imux's bindings need.
func DefaultBin() string { return defaultBin() }

func findPsmux() string {
	if p, err := exec.LookPath("psmux"); err == nil {
		return p
	}
	for _, p := range PsmuxCandidates() {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return "psmux"
}

// PsmuxCandidates lists where psmux is installed without being on PATH:
// imux's own copy (install.ps1), winget, scoop and cargo.
func PsmuxCandidates() []string {
	var out []string
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		out = append(out, filepath.Join(d, "Programs", "imux", "psmux.exe"))
		m, _ := filepath.Glob(filepath.Join(d, "Microsoft", "WinGet", "Packages", "marlocarlo.psmux_*", "psmux.exe"))
		out = append(out, m...)
		out = append(out, filepath.Join(d, "Microsoft", "WinGet", "Links", "psmux.exe"))
	}
	if h, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(h, "scoop", "shims", "psmux.exe"), filepath.Join(h, ".cargo", "bin", "psmux.exe"))
	}
	return out
}
