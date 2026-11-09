package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/bus"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/limits"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/watch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:    "__watch",
		Args:    "run|ensure|stop|status",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runWatch,
	})
	workspace.EnsureWatcher = func(w *workspace.WS) { _ = watch.Ensure(w.Self) }
	watcherStatus = watch.Status
}

// SnapshotEvery is how often the watcher saves the open projects.
var SnapshotEvery = 30 * time.Second

// SidebarCheckEvery is how often the watcher looks for windows whose
// sidebar was killed.
var SidebarCheckEvery = 10 * time.Second

// keepSidebars brings back the sidebars that were killed (§9.3 "Sidebars
// come back"): a window seen without one in two checks in a row gets it
// again; a new tab, whose sidebar comes a moment after it, never gets two.
func keepSidebars(w *workspace.WS, logger *log.Logger) {
	ctx := context.Background()
	seen := map[string]bool{}
	every := SidebarCheckEvery
	if d, err := time.ParseDuration(os.Getenv("IMUX_SIDEBAR_CHECK")); err == nil && d > 0 {
		every = d // tests
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for range t.C {
		missing, err := w.WindowsWithoutSidebar(ctx)
		if err != nil {
			continue
		}
		next := map[string]bool{}
		for win, pane := range missing {
			if !seen[win] {
				next[win] = true
				continue
			}
			if _, err := w.ToggleSidebar(ctx, pane, "on"); err != nil {
				logger.Printf("sidebar for %s: %v", win, err)
				continue
			}
			logger.Printf("sidebar back in %s", win)
		}
		seen = next
	}
}

// newTicker builds the watcher's per-tick work from the config.
var newTicker = func(cfg *config.Config, logger *log.Logger) (watch.Ticker, *tmux.Client, error) {
	tc := tmux.New(paths.TmuxConf())
	set, err := limits.Compile(cfg)
	if err != nil {
		return nil, nil, err
	}
	w, err := workspace.New(cfg)
	if err != nil {
		return nil, nil, err
	}
	w.T = tc
	// A watcher started by a new binary brings the tmux config up to date
	// (key bindings, status bar) for sessions started by an older one.
	if err := w.EnsureConf(context.Background()); err != nil {
		logger.Printf("tmux config: %v", err)
	}
	go keepSidebars(w, logger)
	actor := &watch.Actor{W: w, Log: logger}
	keeper := &watch.StatusKeeper{W: w}
	states := &watch.PaneStates{T: tc, Notify: newNotifier(cfg.Notify, tc).Send}
	var lastSnap time.Time
	return &watch.Detector{
		T: tc, Patterns: set, Cfg: cfg.Limits, Now: time.Now, Log: logger,
		OnEvent: actor.OnEvent,
		OnTick: func(_ context.Context, panes []watch.PaneRow, _ []watch.Client) []tmux.Cmd {
			return keeper.Commands(panes)
		},
		OnTickFailed:  keeper.Forget,
		WatchDetached: cfg.Notify.Enabled,
		OnActivity: func(at time.Time, changed map[string]time.Time, asking map[string]bool) {
			a := &bus.Activity{At: at.UnixMilli(), Changed: map[string]int64{}}
			for p, t := range changed {
				a.Changed[p] = t.UnixMilli()
			}
			_ = bus.WriteActivity(a)
			states.Apply(context.Background(), at, changed, asking)
			// The open projects, for reopening them as they were (D24).
			if at.Sub(lastSnap) >= SnapshotEvery {
				lastSnap = at
				if err := w.SaveSnapshot(context.Background()); err != nil {
					logger.Printf("snapshot: %v", err)
				}
			}
		},
	}, tc, nil
}

func runWatch(e *Env, args []string) error {
	if len(args) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "__watch run|ensure|stop|status")}
	}
	switch args[0] {
	case "run":
		return watch.Run(func(logger *log.Logger) (watch.Ticker, *tmux.Client, time.Duration, time.Duration, error) {
			cfg, err := loadConfig(e)
			if err != nil {
				return nil, nil, 0, 0, err
			}
			if cfg.UpdateCheck {
				go checkUpdates(context.Background())
			}
			tk, tc, err := newTicker(cfg, logger)
			return tk, tc, cfg.Limits.PollInterval.Duration, cfg.Limits.IdleInterval.Duration, err
		})
	case "ensure":
		self, err := workspace.Self()
		if err != nil {
			return err
		}
		return watch.Ensure(self)
	case "stop":
		if !watch.Stop() {
			fmt.Fprintln(e.Stdout, i18n.T("watch.not_running"))
		}
		return nil
	case "status":
		if running, pid := watch.Status(); running {
			fmt.Fprintln(e.Stdout, i18n.T("watch.running", pid))
		} else {
			fmt.Fprintln(e.Stdout, i18n.T("watch.not_running"))
		}
		return nil
	}
	return &UsageError{Msg: i18n.T("cli.usage", "__watch run|ensure|stop|status")}
}
