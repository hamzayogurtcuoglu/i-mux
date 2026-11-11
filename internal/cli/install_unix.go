//go:build !windows

package cli

import "os/exec"

// installCommand runs an agent's install command line with sh.
func installCommand(line string) *exec.Cmd { return exec.Command("sh", "-c", line) }
