package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/historyview"
	"github.com/hamzayogurtcuoglu/i-mux/internal/histsearch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func init() {
	register(&command{
		Name:    "__history",
		Args:    "[--pane %N] [--client C]",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runHistory,
	})
}

// runHistory is History's search popup (§9.3 "Search in History"): the
// folder's conversations, searched by their text as you type; the one
// chosen opens its menu (resume, continue in another agent, ...).
func runHistory(e *Env, args []string) error {
	fs := newFlags(e, "__history")
	paneF := fs.String("pane", "", "the pane History is for")
	clientF := fs.String("client", "", "the client the popup is on")
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
	f := sources.New(cfg)
	m := &historyview.Model{Title: i18n.T("history.title", paths.Shorten(p.CWD)), Now: time.Now()}
	for _, c := range w.History(ctx, f, p.CWD) {
		m.Convs = append(m.Convs, historyview.Conv{Ref: c.Ref, Title: c.Title, Pinned: c.Pinned})
	}
	load := func(ctx context.Context, ref session.Ref) (*session.Session, error) {
		pr := f.Provider(ref.Agent)
		if pr == nil {
			return nil, fmt.Errorf("no parser for %s", ref.Agent)
		}
		return pr.Load(ctx, ref, histsearch.Whole)
	}
	key, err := historyview.Run(m, historyview.Options{In: os.Stdin, Out: os.Stdout, Load: load})
	if err != nil || key == "" {
		return err
	}
	// Its menu, once the popup has gone (a menu would replace it).
	menu := []string{"__menu", "--session", key, "--pane", p.ID}
	if *clientF != "" {
		menu = append(menu, "--client", *clientF)
	}
	_, err = w.T.Run(ctx, tmux.Cmd{"run-shell", "-b", "-d", "0.15", tmux.UICommand(w.Self, menu...)})
	return err
}
