package cli

import (
	"fmt"
	"log"
	"time"

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
	actor := &watch.Actor{W: w, Log: logger}
	return &watch.Detector{T: tc, Patterns: set, Cfg: cfg.Limits, Now: time.Now, Log: logger, OnEvent: actor.OnEvent}, tc, nil
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
