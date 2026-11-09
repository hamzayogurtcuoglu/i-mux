package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/version"
	"github.com/hamzayogurtcuoglu/i-mux/internal/versions"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:    "versions",
		Summary: "cmd.versions",
		Args:    "[--popup]",
		Run:     runVersions,
	})
}

// runVersions lists i-mux, tmux and the agent CLIs with their versions and
// update commands (Settings → Versions and updates). In the popup a letter
// runs that row's command right there, and the list is read again.
func runVersions(e *Env, args []string) error {
	fs := newFlags(e, "versions")
	popup := fs.Bool("popup", false, "ask for a letter and run its update (Settings)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	cfg, err := loadConfig(e)
	if err != nil {
		return err
	}
	self, err := workspace.Self()
	if err != nil {
		return err
	}
	ctx := context.Background()
	show := func() []versions.Row {
		rows := versions.Collect(ctx, versionsEnv(ctx, cfg, self))
		rememberVersions(cfg, rows)
		fmt.Fprintln(e.Stdout, i18n.T("versions.title")+"\n")
		fmt.Fprint(e.Stdout, versions.Format(rows))
		return rows
	}
	rows := show()
	if !*popup {
		return nil
	}
	in := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprint(e.Stdout, "\n"+i18n.T("versions.prompt"))
		line, err := in.ReadString('\n')
		key := strings.TrimSpace(line)
		if key == "" || err != nil {
			return nil
		}
		var row *versions.Row
		for i := range rows {
			if rows[i].Key == key && rows[i].Fix != nil {
				row = &rows[i]
			}
		}
		if row == nil {
			fmt.Fprintln(e.Stdout, i18n.T("versions.nothing", key))
			continue
		}
		fmt.Fprintln(e.Stdout, "\n$ "+row.Show)
		cmd := exec.Command(row.Fix[0], row.Fix[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, e.Stdout, e.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintln(e.Stdout, "\n"+i18n.T("versions.failed", err.Error()))
		} else {
			fmt.Fprintln(e.Stdout, "\n"+i18n.T("versions.done"))
			if row.Tmux {
				fmt.Fprintln(e.Stdout, i18n.T("versions.tmux_restart"))
			}
		}
		fmt.Fprintln(e.Stdout)
		rows = show()
	}
}

// versionsEnv reads the real system: PATH, the agents' binaries and the
// version of the imux binary on disk (an update replaces it while this
// process runs).
func versionsEnv(ctx context.Context, cfg *config.Config, self string) versions.Env {
	st, _ := state.Read()
	if st == nil {
		st = &state.State{}
	}
	cur := version.Version
	if f := strings.Fields(commandOutput(ctx, self, "version")); len(f) >= 2 {
		cur = f[1]
	}
	return versions.Env{
		Cfg: cfg, Self: self, Current: cur, Latest: st.Latest, Root: os.Geteuid() == 0,
		LookPath: exec.LookPath,
		Output:   commandOutput,
		Resolve:  func(id session.AgentID) agent.Resolved { return agent.Resolve(cfg, id, st) },
		Probe:    func(r agent.Resolved) string { return agent.ProbeVersion(r.Path, r.PathEnv) },
	}
}

// commandOutput runs a short command (5 s at most) and returns its stdout.
func commandOutput(ctx context.Context, argv ...string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).Output()
	if err != nil && len(out) == 0 {
		return ""
	}
	return string(out)
}

// rememberVersions puts the versions just read into state.json's bin cache,
// so doctor and the menus show them (they re-read a version once a day).
func rememberVersions(cfg *config.Config, rows []versions.Row) {
	byName := map[string]string{}
	for _, r := range rows {
		if r.Version != "" {
			byName[r.Name] = r.Version
		}
	}
	now := time.Now()
	_ = state.Update(func(s *state.State) error {
		for _, a := range agent.Ordered(cfg) {
			b := s.Bins[string(a.ID)]
			if v := byName[a.Name]; v != "" && b != nil && b.Version != v {
				b.Version, b.CheckedAt = v, now
			}
		}
		return nil
	})
}
