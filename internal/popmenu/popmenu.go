// Package popmenu opens i-mux's popups (the quick pick, the Scheduler's
// views, the environment dialog) and names the files they keep their data
// in. (It once also drew menus in a popup for tmux before 3.5, D54; i-mux
// runs on i-mux-core only now, D87, whose menus take the mouse.)
package popmenu

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Popup shows program (argv: the program and its arguments) in a w×h popup
// at x, y on client, from pane, and does not wait for it; after runs once
// the popup closed (a popup cannot open over another). The tmux client that
// shows it starts from pane (TMUX_PANE), as display-menu runs a choice
// there.
func Popup(ctx context.Context, c *tmux.Client, client, pane string, w, h int, x, y string, program []string, after ...tmux.Cmd) error {
	pop := tmux.Cmd{"display-popup"}
	if client != "" {
		pop = append(pop, "-c", client)
	}
	if pane != "" {
		pop = append(pop, "-t", pane)
	}
	pop = append(append(pop, "-E", "-w", strconv.Itoa(w), "-h", strconv.Itoa(h), "-x", x, "-y", y), tmux.Program(program...)...)
	// A status-line message freezes the client's terminal, which then draws
	// none of the popup's output until it goes: one shown with -d 0
	// ("✨ Writing…") until a key. An empty message that lasts 1 ms
	// replaces it.
	clear := tmux.Cmd{"display-message", "-d", "1"}
	if client != "" {
		clear = append(clear, "-c", client)
	}
	cmds := []tmux.Cmd{append(clear, "")}
	var env []string
	if pane != "" {
		env = []string{"TMUX_PANE=" + pane}
	}
	return c.SpawnEnv(env, append(append(cmds, pop), after...)...)
}

// NewID is a random file name for a popup's data under paths.Menus(); it
// also removes the files older ones left.
func NewID() (string, error) {
	if err := paths.EnsureDir(paths.Menus()); err != nil {
		return "", err
	}
	prune(paths.Menus())
	return newID()
}

// newID is a random file name.
func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ValidID reports whether id is one newID made (the popup reads only those).
func ValidID(id string) bool {
	if len(id) != 16 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// pruneAfter is how old a left-over file is (a popup that never started, a
// choice whose command failed) before it goes.
const pruneAfter = 10 * time.Minute

func prune(dir string) {
	es, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range es {
		if fi, err := e.Info(); err == nil && time.Since(fi.ModTime()) > pruneAfter {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
