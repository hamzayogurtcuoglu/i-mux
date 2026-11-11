//go:build windows

package cli

import (
	"context"
	"os/exec"

	"github.com/hamzayogurtcuoglu/i-mux/internal/winx"
)

// userStatusline runs the user's own Claude status line command with Git
// Bash, as Claude Code does on Windows (sh on the PATH when Git is not
// found), without a console window.
func userStatusline(ctx context.Context, cmd string) *exec.Cmd {
	sh := winx.GitBash()
	if sh == "" {
		sh = "sh"
	}
	return winx.Hidden(exec.CommandContext(ctx, sh, "-c", cmd))
}
