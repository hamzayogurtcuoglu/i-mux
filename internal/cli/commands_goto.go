package cli

import (
	"context"
	"os"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/gotofile"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func init() {
	register(&command{
		Name:   "__goto",
		Args:   "[--pane P --client C | --run]",
		Hidden: true,
		Run:    runGoto,
	})
}

// runGoto is Go to File (§9.3, Ctrl+P). From the key binding it opens the
// popup in the pane's project folder (on the sidebar: its target pane's);
// with --run it is the popup, and the chosen file opens in the editor.
func runGoto(e *Env, args []string) error {
	fs := newFlags(e, "__goto")
	run := fs.Bool("run", false, "the popup itself")
	paneF := fs.String("pane", "", "the pane the key was pressed in")
	clientF := fs.String("client", "", "the client to open the popup on")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	if *run {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		path, line, err := gotofile.Run(gotofile.Options{In: os.Stdin, Out: os.Stdout, Dir: dir})
		if err != nil || path == "" {
			return err
		}
		return runEdit(e, gotofile.LineArgs(path, line))
	}
	pane, err := targetPane(*paneF)
	if err != nil {
		return err
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	dir := paneDir(ctx, w.T, pane)
	c := tmux.Cmd{"display-popup"}
	if *clientF != "" {
		c = append(c, "-c", *clientF)
	}
	c = append(c, "-E", "-w", "80%", "-h", "70%")
	if dir != "" {
		c = append(c, "-d", tmux.EscapeFormat(dir)) // -d is a format
	}
	return w.T.Spawn(append(c, tmux.ShJoin(w.Self, "__goto", "--run")))
}

// paneDir is the folder of a pane's project: its agent's folder, else its
// shell's; on the sidebar, that of the pane it acts on.
func paneDir(ctx context.Context, t *tmux.Client, pane string) string {
	const f = "#{" + sidebar.OptRole + "}\t#{@imux_last}\t#{@imux_cwd}\t#{pane_current_path}"
	out, err := t.Run(ctx, tmux.Cmd{"display-message", "-p", "-t", pane, f})
	if err != nil {
		return ""
	}
	r := tmux.Rows(out, 4)
	if len(r) == 0 {
		return ""
	}
	if r[0][0] == sidebar.RoleSidebar && r[0][1] != "" && r[0][1] != pane {
		return paneDir(ctx, t, r[0][1])
	}
	if d := strings.TrimSpace(r[0][2]); d != "" {
		return d
	}
	return strings.TrimSpace(r[0][3])
}
