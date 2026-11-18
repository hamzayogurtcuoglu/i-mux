package cli

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// engineRunner answers -V with version and list-sessions with running.
type engineRunner struct {
	version string
	running bool
}

func (r engineRunner) Run(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
	if slices.Equal(args, []string{"-V"}) {
		return []byte(r.version + "\n"), nil, nil
	}
	if slices.Contains(args, "list-sessions") && !r.running {
		return nil, []byte("no server running on /tmp/imux-core-501/imux\n"), errors.New("exit status 1")
	}
	return []byte("proj\n"), nil, nil
}

// TestCoreKeepsOld (T88.4): the update says the engine keeps its old version
// only when sessions run on i-mux-core; never for tmux, whose server is not
// i-mux's.
func TestCoreKeepsOld(t *testing.T) {
	for _, c := range []struct {
		version string
		running bool
		want    bool
	}{
		{"tmux 3.8 (imux-core 0.1.0)", true, true},
		{"tmux 3.8 (imux-core 0.1.0)", false, false},
		{"tmux 3.8", true, false},
		{"tmux 3.5a", false, false},
	} {
		cl := &tmux.Client{Bin: "engine", Socket: "imux", Runner: engineRunner{c.version, c.running}}
		if got := coreKeepsOld(context.Background(), cl); got != c.want {
			t.Errorf("%q running=%v: %v, want %v", c.version, c.running, got, c.want)
		}
	}
}
