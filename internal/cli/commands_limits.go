package cli

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/panel"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

func init() {
	register(&command{
		Name:    "limits",
		Args:    "[--json] [--pager]",
		Summary: "cmd.limits",
		Run:     runLimits,
	})
}

func runLimits(e *Env, args []string) error {
	fs := newFlags(e, "limits")
	asJSON := fs.Bool("json", false, "print JSON (schema: docs/LIMITS.md)")
	pager := fs.Bool("pager", false, "show in a pager (used by the tmux popup)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	w, cfg, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	rep := panel.Collect(context.Background(), cfg, w.State(), func(id session.AgentID) bool { return w.Installed(id) }, now())
	if *asJSON {
		return rep.JSON(e.Stdout)
	}
	if !*pager {
		return rep.Table(e.Stdout)
	}
	var buf bytes.Buffer
	if err := rep.Table(&buf); err != nil {
		return err
	}
	if less, err := exec.LookPath("less"); err == nil {
		cmd := exec.Command(less, "-R", "-X", "-K")
		cmd.Stdin, cmd.Stdout, cmd.Stderr = &buf, os.Stdout, os.Stderr
		return cmd.Run()
	}
	_, _ = e.Stdout.Write(buf.Bytes())
	fmt.Fprint(e.Stdout, i18n.T("limits.press_enter"))
	_, _ = bufio.NewReader(e.Stdin).ReadString('\n')
	return nil
}
