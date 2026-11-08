package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/update"
	"github.com/hamzayogurtcuoglu/i-mux/internal/version"
)

func init() {
	register(&command{
		Name:    "update",
		Summary: "cmd.update",
		Args:    "[--check]",
		Run:     runUpdate,
	})
}

// runUpdate installs the newest release over this imux (Settings → Update).
// The watcher and the sidebars restart with it on their own.
func runUpdate(e *Env, args []string) error {
	fs := newFlags(e, "update")
	check := fs.Bool("check", false, "only tell whether a newer release is out")
	popup := fs.Bool("popup", false, "wait for Enter at the end (run in a popup)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if _, err := loadConfig(e); err != nil {
		return err
	}
	done := func() {
		if *popup {
			fmt.Fprint(e.Stdout, "\n"+i18n.T("completion.press_enter"))
			_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		}
	}
	defer done()
	cur := version.Version
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	latest, err := update.Latest(ctx)
	cancel()
	if err != nil {
		fmt.Fprintln(e.Stdout, i18n.T("update.unreachable", err.Error()))
		return err
	}
	remember(latest)
	if !update.Newer(latest, cur) {
		fmt.Fprintln(e.Stdout, i18n.T("update.latest", cur))
		return nil
	}
	if *check {
		fmt.Fprintln(e.Stdout, i18n.T("update.out", latest, cur))
		return nil
	}
	dir, err := update.Dir()
	if err != nil {
		return err
	}
	if update.Homebrew(dir) {
		fmt.Fprintln(e.Stdout, i18n.T("update.brew"))
		return nil
	}
	fmt.Fprintln(e.Stdout, i18n.T("update.updating", cur, latest)+"\n")
	argv := update.InstallCmd(latest, dir)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, e.Stdout, e.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(e.Stdout, "\n"+i18n.T("update.failed", err.Error()))
		return err
	}
	fmt.Fprintln(e.Stdout, "\n"+i18n.T("update.done", latest))
	sidebar.RefreshAll(context.Background(), tmux.New(paths.TmuxConf()))
	return nil
}

// remember keeps the newest release in state.json for the sidebars and
// Settings.
func remember(latest string) {
	_ = state.Update(func(s *state.State) error {
		s.Latest, s.LatestChecked = latest, time.Now().Unix()
		return nil
	})
}

// checkUpdates asks GitHub once a day (the watcher runs it) and makes the
// sidebars redraw when a newer release is out.
func checkUpdates(ctx context.Context) {
	for {
		st, _ := state.Read()
		if st == nil || time.Since(time.Unix(st.LatestChecked, 0)) >= update.Every {
			c, cancel := context.WithTimeout(ctx, 30*time.Second)
			if latest, err := update.Latest(c); err == nil {
				remember(latest)
				if update.Newer(latest, version.Version) {
					sidebar.RefreshAll(ctx, tmux.New(paths.TmuxConf()))
				}
			}
			cancel()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Hour):
		}
	}
}
