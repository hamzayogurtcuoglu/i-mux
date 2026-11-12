package cli

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/clipimage"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func init() {
	register(&command{
		Name:   "__paste-image",
		Args:   "--pane P [--client C] [--key K]",
		Hidden: true,
		Run:    runPasteImage,
	})
}

// runPasteImage pastes the clipboard's image (or copied image files) into
// an agent's pane as their paths, the way a dropped file arrives (§9.3
// "Pasting images"). With no image the key (Ctrl+V, Alt+V) goes to the pane.
func runPasteImage(e *Env, args []string) error {
	fs := newFlags(e, "__paste-image")
	paneF := fs.String("pane", "", "the agent's pane")
	clientF := fs.String("client", "", "client for messages")
	keyF := fs.String("key", "", "key to pass on when there is no image (C-v, M-v)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	pane, err := targetPane(*paneF)
	if err != nil {
		return err
	}
	w, cfg, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	p, err := w.PaneTarget(ctx, pane)
	if err != nil {
		return err
	}
	passOn := func() {
		if *keyF != "" {
			_, _ = w.T.Run(ctx, tmux.Cmd{"send-keys", "-t", p.ID, *keyF})
		}
	}
	say := func(msg string) {
		if *keyF == "" { // run by hand, not by a key: say why nothing came
			w.Message(ctx, *clientF, p.ID, msg)
		}
	}
	if !cfg.Paste.Images || p.Agent == "" {
		passOn()
		return nil
	}
	files, err := clipboardImages(ctx, cfg)
	var noTool *clipimage.ErrNoTool
	switch {
	case errors.Is(err, clipimage.ErrNoImage):
		passOn()
		say(i18n.T("paste.none"))
		return nil
	case errors.As(err, &noTool), err != nil:
		passOn()
		w.Message(ctx, *clientF, p.ID, i18n.T("paste.no_tool", err.Error()))
		return err
	}
	text := ImageText(p.Agent, files)
	if _, err := w.T.Run(ctx,
		tmux.Cmd{"set-buffer", "-b", "imux-paste", "--", text},
		tmux.Cmd{"paste-buffer", "-p", "-d", "-b", "imux-paste", "-t", p.ID}); err != nil {
		return err
	}
	if *keyF == "" {
		w.Message(ctx, *clientF, p.ID, i18n.T("paste.done", len(files)))
	}
	return nil
}

// clipboardImages runs [paste] command, or reads the clipboard.
func clipboardImages(ctx context.Context, cfg *config.Config) ([]string, error) {
	if c := strings.Fields(cfg.Paste.Command); len(c) > 0 {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, c[0], c[1:]...).Output()
		if err != nil {
			return nil, err
		}
		var files []string
		sc := bufio.NewScanner(strings.NewReader(string(out)))
		for sc.Scan() {
			if f := strings.TrimSpace(sc.Text()); f != "" {
				if _, err := os.Stat(f); err == nil {
					files = append(files, f)
				}
			}
		}
		if len(files) == 0 {
			return nil, clipimage.ErrNoImage
		}
		return files, nil
	}
	return (&clipimage.Reader{Dir: paths.Images()}).Read(ctx)
}

// ImageText is what the agent gets: the paths as a dropped file arrives
// (spaces escaped), "@path" for Antigravity, and a space after.
func ImageText(agent session.AgentID, files []string) string {
	parts := make([]string, 0, len(files))
	for _, f := range files {
		f = strings.ReplaceAll(f, " ", `\ `)
		if agent == session.Antigravity {
			f = "@" + f
		}
		parts = append(parts, f)
	}
	return strings.Join(parts, " ") + " "
}
