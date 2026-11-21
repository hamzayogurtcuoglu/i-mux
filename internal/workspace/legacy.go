package workspace

import (
	"context"
	"os/exec"

	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// LegacyTmux is the tmux an earlier i-mux ran on (D87): a client for its
// server on imux's socket, or nil without tmux. Tests replace it.
var LegacyTmux = func() *tmux.Client {
	bin, err := exec.LookPath("tmux")
	if err != nil {
		return nil
	}
	return &tmux.Client{Bin: bin, Socket: tmux.Socket(), Runner: tmux.ExecRunner{}}
}

// MoveFromTmux moves the projects an earlier i-mux left open on tmux to the
// engine (D87, T92.3): they are recorded as interrupted snapshots, so
// Restore last session brings them back on i-mux-core with their agents
// resuming their conversations, and that one tmux server is closed. tmux
// itself and its other servers are never touched. It returns how many
// projects moved: none without tmux or without that server.
func (w *WS) MoveFromTmux(ctx context.Context) int {
	old := LegacyTmux()
	if old == nil {
		return 0
	}
	lw := *w
	lw.T = old
	if lw.ServerID(ctx) == "" {
		return 0 // no server of an earlier i-mux
	}
	open, err := lw.Capture(ctx)
	if err != nil {
		return 0
	}
	if err := lw.SaveSnapshot(ctx); err != nil {
		return 0 // its projects could not be kept: leave the server running
	}
	_, _ = old.Run(ctx, tmux.Cmd{"kill-server"})
	return len(open)
}
