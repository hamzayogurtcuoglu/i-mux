// Command imux is a lightweight tmux workspace that moves a conversation
// between AI coding CLIs (Claude Code, Antigravity CLI, Copilot CLI, Codex CLI, OpenCode).
//
// It carries its engine, i-mux-core (D88): run under the name imux-core (a
// link or copy of it), it is the engine, so `go install …/cmd/imux` alone
// gives a working i-mux.
package main

import (
	"context"
	"io"
	"os"

	"github.com/hamzayogurtcuoglu/i-mux-core/engine"
	"github.com/hamzayogurtcuoglu/i-mux/internal/cli"
	"github.com/hamzayogurtcuoglu/i-mux/internal/phone"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func main() {
	if tmux.RunsAsEngine(os.Args[0]) {
		os.Exit(engine.Main(os.Args[1:], os.Stdout, os.Stderr))
	}
	tmux.CarriesEngine = true
	tmux.EngineCommand = engine.Command // engine queries inside this process (D92)
	phone.AttachFunc = attachStream     // the phone's view of the workspace (D105)
	os.Exit(cli.Main(os.Args))
}

// attachStream is a stream client of the engine for the phone (D105).
func attachStream(ctx context.Context, args []string, name string, cols, rows int, out io.Writer) (phone.Stream, error) {
	c, err := engine.Attach(ctx, args, engine.Terminal{Name: name, Cols: cols, Rows: rows}, out)
	if err != nil {
		return nil, err
	}
	return c, nil
}
