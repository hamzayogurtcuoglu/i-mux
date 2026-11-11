//go:build windows

package cli

import (
	"os/exec"

	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// installCommand runs an agent's install command line with PowerShell (the
// Windows commands are PowerShell's: irm ... | iex, npm).
func installCommand(line string) *exec.Cmd {
	return exec.Command(tmux.PowerShell(), "-NoLogo", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", line)
}
