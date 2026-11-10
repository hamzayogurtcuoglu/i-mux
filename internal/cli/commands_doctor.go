package cli

import (
	"context"
	"fmt"

	"github.com/hamzayogurtcuoglu/i-mux/internal/doctor"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/nerdfont"
)

func init() {
	register(&command{
		Name:    "doctor",
		Args:    "[--ci]",
		Summary: "cmd.doctor",
		Run:     runDoctor,
	})
}

// watcherStatus reports the limit watcher (M3 sets it).
var watcherStatus = func() (bool, int) { return false, 0 }

func runDoctor(e *Env, args []string) error {
	fs := newFlags(e, "doctor")
	ci := fs.Bool("ci", false, "skip the agent checks (release smoke tests)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	w, cfg, err := newWorkspace(e, !*ci)
	if err != nil {
		return err
	}
	rep := doctor.Run(context.Background(), doctor.Options{
		Cfg: cfg, Tmux: w.T, Self: w.Self, CI: *ci, Watcher: watcherStatus, Resolved: w.Bins(),
		Font: func() bool {
			s, err := nerdfont.Detect(context.Background())
			return err == nil && s.Ready()
		},
	})
	fmt.Fprintln(e.Stdout, i18n.T("doctor.header"))
	for _, r := range rep.Results {
		fmt.Fprintf(e.Stdout, "  %s %s\n", r.Level.Symbol(), r.Msg)
	}
	if rep.Failed() {
		return &ExitCodeError{Code: ExitError, Msg: i18n.T("doctor.failed")}
	}
	return nil
}
