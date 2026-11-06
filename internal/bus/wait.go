package bus

import (
	"context"
	"errors"
	"hash/fnv"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// ErrTimeout means the pane did not become idle in time.
var ErrTimeout = errors.New("timed out")

// Wait polls pane's screen every second until it did not change for idle,
// and returns that screen. It fails with ErrTimeout after timeout.
func (b *Bus) Wait(ctx context.Context, pane string, idle, timeout time.Duration) (string, error) {
	start := b.W.Now()
	var last uint64
	var screen string
	since := start
	first := true
	for {
		out, err := b.W.T.Run(ctx, tmux.Cmd{"capture-pane", "-p", "-J", "-t", pane})
		if err != nil {
			return "", err
		}
		h := fnv.New64a()
		_, _ = h.Write([]byte(out))
		now := b.W.Now()
		if sum := h.Sum64(); first || sum != last {
			last, since, screen, first = sum, now, out, false
		} else if now.Sub(since) >= idle {
			return screen, nil
		}
		if now.Sub(start) >= timeout {
			return screen, ErrTimeout
		}
		if err := ctx.Err(); err != nil {
			return screen, err
		}
		b.Sleep(time.Second)
	}
}
