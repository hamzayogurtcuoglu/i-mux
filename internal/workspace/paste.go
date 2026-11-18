package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"hash/fnv"
	"os"
	"path/filepath"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Paste timing (§9.5): the agent's screen must be unchanged for StableFor
// before the handoff is pasted; after StableMax it is pasted anyway.
var (
	StableFor  = 1500 * time.Millisecond
	StableMax  = 20 * time.Second
	stablePoll = 250 * time.Millisecond
	enterDelay = 150 * time.Millisecond
)

// pasteMode reports whether run l's prompt is pasted instead of passed in
// argv: inject = "paste", or a template without a {prompt} slot.
func (w *WS) pasteMode(l Launch) bool {
	if l.Prompt == "" {
		return false
	}
	ac := w.Cfg.Agent(string(l.Agent))
	tpl := ac.NewArgs
	if l.ResumeID != "" {
		tpl = ac.ResumeArgs
	}
	return w.Cfg.Handoff.Inject == "paste" || !agent.HasPrompt(tpl)
}

// WaitStable waits until pane's screen did not change for StableFor (or
// StableMax passed). It returns an error if the pane is gone or dead.
func (w *WS) WaitStable(ctx context.Context, pane string) error {
	deadline := time.Now().Add(StableMax)
	var last uint64
	since := time.Now()
	for {
		out, err := w.T.Run(ctx,
			tmux.Cmd{"display-message", "-p", "-t", pane, "#{pane_dead}"},
			tmux.Cmd{"capture-pane", "-p", "-t", pane})
		if err != nil {
			return err
		}
		if len(out) >= 2 && out[:2] == "1\n" {
			return errors.New("pane exited")
		}
		h := fnv.New64a()
		_, _ = h.Write([]byte(out))
		if sum := h.Sum64(); sum != last {
			last, since = sum, time.Now()
		} else if time.Since(since) >= StableFor {
			return nil
		}
		if time.Now().After(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(stablePoll):
		}
	}
}

// Paste puts text into pane as if typed: a private temp file, load-buffer,
// paste-buffer -p (bracketed only if the app asked for it) -d (delete the
// buffer), then Enter when enter is set.
func (w *WS) Paste(ctx context.Context, pane, text string, enter bool) error {
	var r [6]byte
	if _, err := rand.Read(r[:]); err != nil {
		return err
	}
	name := "imux-" + hex.EncodeToString(r[:])
	dir := paths.Launch()
	if err := paths.EnsureDir(dir); err != nil {
		return err
	}
	file := filepath.Join(dir, name+".txt")
	if err := paths.WriteFile(file, []byte(text)); err != nil {
		return err
	}
	defer os.Remove(file)
	if _, err := w.T.Run(ctx,
		tmux.Cmd{"load-buffer", "-b", name, file},
		tmux.Cmd{"paste-buffer", "-p", "-d", "-b", name, "-t", pane}); err != nil {
		_, _ = w.T.Run(ctx, tmux.Cmd{"delete-buffer", "-b", name})
		return err
	}
	if !enter {
		return nil
	}
	time.Sleep(enterDelay)
	_, err := w.T.Run(ctx, tmux.Cmd{"send-keys", "-t", pane, "Enter"})
	return err
}
