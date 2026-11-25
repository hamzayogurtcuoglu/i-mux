package phone

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// AttachFunc starts a stream client of the engine in this process:
// i-mux-core's engine.Attach (its E19), which cmd/imux sets (D105). nil, as
// in test binaries: no stream.
var AttachFunc func(ctx context.Context, args []string, name string, cols, rows int, out io.Writer) (Stream, error)

// ErrNoEngine means this imux does not carry the engine (a test binary).
var ErrNoEngine = errors.New("phone: this imux does not carry the engine")

// TmuxEngine is the workspace on i-mux's engine (D87).
type TmuxEngine struct{ T *tmux.Client }

// desktop is the most recently used client that is no phone's: its
// session and size.
func (e *TmuxEngine) desktop(ctx context.Context) (session string, cols, rows int) {
	out, err := e.T.Run(ctx, tmux.Cmd{"list-clients", "-F", "#{client_name}\t#{client_session}\t#{client_width}\t#{client_height}\t#{client_activity}"})
	if err != nil {
		return "", 0, 0
	}
	var best int64 = -1
	for _, f := range tmux.Rows(out, 5) {
		if strings.HasPrefix(f[0], "phone-") {
			continue
		}
		act, _ := strconv.ParseInt(f[4], 10, 64)
		if act > best {
			best = act
			session = f[1]
			cols, _ = strconv.Atoi(f[2])
			rows, _ = strconv.Atoi(f[3])
		}
	}
	return session, cols, rows
}

// DesktopSize is the desktop client's size.
func (e *TmuxEngine) DesktopSize(ctx context.Context) (int, int) {
	_, c, r := e.desktop(ctx)
	return c, r
}

// Attach starts the phone's client on the session the desktop shows, else
// the newest.
func (e *TmuxEngine) Attach(ctx context.Context, name string, cols, rows int, out io.Writer) (Stream, error) {
	if AttachFunc == nil {
		return nil, ErrNoEngine
	}
	args := append(e.T.BaseArgs(), "-u", "attach-session")
	if s, _, _ := e.desktop(ctx); s != "" {
		args = append(args, "-t", "="+s)
	}
	return AttachFunc(ctx, args, name, cols, rows, out)
}
