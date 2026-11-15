package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/machine"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/version"
)

func init() {
	register(&command{
		Name:    "__remote-state",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runRemoteState,
	})
	register(&command{
		Name:    "__remote-attach",
		Args:    "<session> [<pane>]",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runRemoteAttach,
	})
}

// runRemoteState is a machine's side of its report (D73): its projects and
// their agent panes' states, as JSON, for the computer that added it.
func runRemoteState(e *Env, _ []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	t := tmux.New(paths.TmuxConf())
	r := machine.Report{Version: version.Version, Projects: []machine.Project{}}
	out, err := t.Run(ctx, tmux.Cmd{
		"list-panes", "-a", "-F",
		"#{session_name}\t#{session_path}\t#{pane_id}\t#{@imux_agent}\t#{@imux_name}\t#{@imux_state}\t#{@imux_state_at}\t#{@imux_role}\t#{pane_dead}",
	})
	if err != nil && !errors.Is(err, tmux.ErrNoServer) {
		return err
	}
	byName := map[string]*machine.Project{}
	var order []string
	for _, f := range tmux.Rows(out, 9) {
		p := byName[f[0]]
		if p == nil {
			p = &machine.Project{Session: f[0], Name: filepath.Base(f[1]), Path: f[1], Panes: []machine.Pane{}}
			byName[f[0]] = p
			order = append(order, f[0])
		}
		if f[7] == sidebar.RoleSidebar || f[3] == "" || f[8] == "1" {
			continue // the UI, a plain shell, or an agent that ended
		}
		since, _ := strconv.ParseInt(f[6], 10, 64)
		p.Panes = append(p.Panes, machine.Pane{ID: f[2], Agent: f[3], Name: f[4], State: f[5], Since: since})
	}
	for _, s := range order {
		p := byName[s]
		p.Branch = gitops.CurrentBranch(ctx, p.Path)
		r.Projects = append(r.Projects, *p)
	}
	return json.NewEncoder(e.Stdout).Encode(r)
}

// runRemoteAttach is a machine's side of opening one of its projects (D73):
// this terminal attaches to its session, pane selected.
func runRemoteAttach(e *Env, args []string) error {
	if len(args) < 1 || len(args) > 2 || !machine.ValidSession(args[0]) || len(args) == 2 && !machine.ValidPane(args[1]) {
		return &UsageError{Msg: i18n.T("cli.usage", "__remote-attach <session> [<pane>]")}
	}
	t := tmux.New(paths.TmuxConf())
	ctx := context.Background()
	if _, err := t.Run(ctx, tmux.Cmd{"has-session", "-t", "=" + args[0]}); err != nil {
		return errors.New(i18n.T("machine.no_project", args[0]))
	}
	if len(args) == 2 {
		_, _ = t.Run(ctx, tmux.Cmd{"select-window", "-t", args[1]}, tmux.Cmd{"select-pane", "-t", args[1]})
	}
	bin, err := exec.LookPath(t.Bin)
	if err != nil {
		return err
	}
	argv := append(append([]string{bin}, t.BaseArgs()...), "-u", "attach-session", "-t", "="+args[0])
	if err := syscall.Exec(bin, argv, os.Environ()); err != nil {
		return fmt.Errorf("tmux: %w", err)
	}
	return nil
}
