package workspace

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// legacyRunner is the tmux an earlier i-mux ran on: a server with a project
// (an agent and a sidebar), or none.
type legacyRunner struct {
	running bool
	calls   []string
}

func (r *legacyRunner) Run(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
	cmd := strings.Join(args, " ")
	r.calls = append(r.calls, cmd)
	if !r.running {
		return nil, []byte("no server running on /tmp/tmux-501/imux"), errors.New("exit status 1")
	}
	switch {
	case strings.Contains(cmd, "display-message"):
		return []byte("4242-1700000000\n"), nil, nil
	case strings.Contains(cmd, "list-panes"):
		return []byte("/home/u/api\t0\t\tlayout\t1\tsidebar\t\t\t0\t\t\n" +
			"/home/u/api\t0\t\tlayout\t1\t\tclaude\t\t0\t\t\n"), nil, nil
	}
	return nil, nil, nil
}

// TestMoveFromTmux (D87, T92.3): the projects an earlier i-mux left open on
// tmux become interrupted snapshots, and that server is closed; without tmux
// or without that server nothing happens.
func TestMoveFromTmux(t *testing.T) {
	w, _, _ := testWS(t)
	ctx := context.Background()
	defer func(f func() *tmux.Client) { LegacyTmux = f }(LegacyTmux)

	LegacyTmux = func() *tmux.Client { return nil }
	if n := w.MoveFromTmux(ctx); n != 0 {
		t.Fatalf("without tmux: %d", n)
	}

	r := &legacyRunner{}
	LegacyTmux = func() *tmux.Client { return &tmux.Client{Bin: "tmux", Socket: "imux", Runner: r} }
	if n := w.MoveFromTmux(ctx); n != 0 || strings.Contains(strings.Join(r.calls, "\n"), "kill-server") {
		t.Fatalf("without a server: %d %q", n, r.calls)
	}

	r = &legacyRunner{running: true}
	if n := w.MoveFromTmux(ctx); n != 1 {
		t.Fatalf("moved %d, want 1: %q", n, r.calls)
	}
	if last := r.calls[len(r.calls)-1]; !strings.HasSuffix(last, "kill-server") {
		t.Fatalf("the server was not closed last: %q", r.calls)
	}
	snaps := LoadSnapshots()
	p := snaps.Find("/home/u/api")
	if p == nil || !p.Open || p.Server != "4242-1700000000" || p.Agents() != 1 {
		t.Fatalf("snapshot %+v", p)
	}
	// On the engine's server (another id) it is a project to restore.
	if got := snaps.Interrupted("1-2", nil); len(got) != 1 || got[0].Dir != "/home/u/api" {
		t.Fatalf("interrupted %+v", got)
	}
}

// TestEngineMissing (D87): without imux-core, i-mux says to install i-mux
// again, which brings the engine, and never asks for tmux.
func TestEngineMissing(t *testing.T) {
	w, _, _ := testWS(t)
	w.T = &tmux.Client{Bin: filepath.Join(t.TempDir(), "imux-core"), Socket: "test", Runner: tmux.ExecRunner{}}
	_, err := w.TmuxVersion(context.Background())
	if !errors.Is(err, ErrTmuxMissing) {
		t.Fatalf("err %v", err)
	}
	if msg := err.Error(); !strings.Contains(msg, "imux-core") || strings.Contains(msg, "install tmux") {
		t.Fatalf("message %q", msg)
	}
}
