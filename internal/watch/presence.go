package watch

import (
	"context"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Attached reports whether list-clients output (#{client_control_mode} per
// line) has a client that can show prompts (not in control mode).
func Attached(out string) bool {
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) == "0" {
			return true
		}
	}
	return false
}

// PresenceTicker only checks that the server lives and whether a client is
// attached.
type PresenceTicker struct{ T *tmux.Client }

// Tick implements Ticker.
func (p PresenceTicker) Tick(ctx context.Context) (bool, error) {
	out, err := p.T.Run(ctx, tmux.Cmd{"list-clients", "-F", "#{client_control_mode}"})
	if err != nil {
		return false, err
	}
	return Attached(out), nil
}
