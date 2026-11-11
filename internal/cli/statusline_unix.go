//go:build !windows

package cli

import (
	"context"
	"os/exec"
)

// userStatusline runs the user's own Claude status line command with sh, as
// Claude Code does.
func userStatusline(ctx context.Context, cmd string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", cmd)
}
