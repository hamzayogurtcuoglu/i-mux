//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux-core/engine"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/phone"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// TestPhoneStream (D105, T109.6): the phone's view is one more client of the
// workspace: it gets i-mux's screen (the sidebar with it) at the phone's
// size, its keys reach the agent, and closing it leaves the workspace.
func TestPhoneStream(t *testing.T) {
	h := newH(t, sidebarConfig)
	agent, _ := h.startWithSidebar()
	h.run("claude", 1)

	old := phone.AttachFunc
	phone.AttachFunc = func(ctx context.Context, args []string, name string, cols, rows int, out io.Writer) (phone.Stream, error) {
		return engine.Attach(ctx, args, engine.Terminal{Name: name, Cols: cols, Rows: rows}, out)
	}
	defer func() { phone.AttachFunc = old }()
	eng := &phone.TmuxEngine{T: tmux.New(paths.TmuxConf())}
	var out lockedBuf
	st, err := eng.Attach(context.Background(), "phone-test", 120, 36, &out)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	defer st.Close()
	h.waitFor("the sidebar on the phone", 10*time.Second, func() bool {
		return strings.Contains(stripANSI(out.String()), "AGENTS")
	})
	clients := h.mustTmux("list-clients", "-F", "#{client_name} #{client_width}x#{client_height}")
	if !strings.Contains(clients, "phone-test 120x36") {
		t.Fatalf("clients:\n%s", clients)
	}

	// Keys typed on the phone reach the agent's pane.
	if err := st.Input([]byte("from the phone\r")); err != nil {
		t.Fatal(err)
	}
	h.waitFor("the phone's line in the agent", 10*time.Second, func() bool {
		return strings.Contains(h.stdinOf("claude", 1), "from the phone")
	})
	_ = agent

	// Closed: the client is gone, the workspace stays.
	_ = st.Close()
	h.waitFor("the phone's client gone", 10*time.Second, func() bool {
		return !strings.Contains(h.mustTmux("list-clients", "-F", "#{client_name}"), "phone-test")
	})
	if !strings.Contains(h.mustTmux("list-panes", "-a", "-F", "#{pane_id}"), agent) {
		t.Fatal("the agent's pane went with the phone")
	}
}
