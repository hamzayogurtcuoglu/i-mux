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
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/installview"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

func init() {
	register(&command{
		Name:    "__install",
		Args:    "<agent> [--pane %N] [--client C]",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runInstall,
	})
}

// runInstall is the popup for an agent that is not installed (§9.3 "Not
// installed"): the command that installs it, run on request; then the chat
// can continue in it.
func runInstall(e *Env, args []string) error {
	fs := newFlags(e, "__install")
	paneF := fs.String("pane", "", "the pane the chat would continue from")
	clientF := fs.String("client", "", "the client the popup is on")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "__install <agent>")}
	}
	id, err := parseAgentFlag("agent", pos[0])
	if err != nil {
		return err
	}
	cfg, err := loadConfig(e)
	if err != nil {
		return err
	}
	a := agent.MustGet(id)
	cmd := agent.Install(cfg, id)
	sym, color := sidebar.AgentIcon(id)
	m := &installview.Model{Icon: color + sym + "\x1b[0m", Name: a.Short, Cmd: cmd, Missing: needs(cmd)}
	r, err := installview.Run(m, os.Stdin, os.Stdout)
	if err != nil || r != installview.Install {
		return err
	}
	fmt.Fprintf(e.Stdout, "\x1b[H\x1b[2J\x1b[1m$ %s\x1b[0m\n\n", cmd)
	c := exec.Command("sh", "-c", cmd)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, e.Stdout, e.Stderr
	runErr := c.Run()
	ok := runErr == nil && agent.Resolve(cfg, id, nil).Installed
	switch {
	case ok && *paneF != "":
		fmt.Fprintln(e.Stdout, "\n"+i18n.T("install.done", a.Short))
		fmt.Fprint(e.Stdout, i18n.T("install.continue", a.Short))
		if key() == "enter" {
			continueIn(e, id, *paneF, *clientF)
		}
		return nil
	case ok:
		fmt.Fprintln(e.Stdout, "\n"+i18n.T("install.done", a.Short))
	case runErr == nil:
		fmt.Fprintln(e.Stdout, "\n"+i18n.T("install.not_on_path", a.Short, a.Bin))
	default:
		fmt.Fprintln(e.Stdout, "\n"+i18n.T("install.failed", runErr.Error()))
	}
	fmt.Fprint(e.Stdout, i18n.T("completion.press_enter"))
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	return runErr
}

// key waits for one key on the terminal and returns its name ("enter",
// "esc", a character).
func key() string {
	fd := int(os.Stdin.Fd())
	restore, err := sidebar.RawMode(fd)
	if err != nil {
		return ""
	}
	defer restore()
	var pending []byte
	for {
		b, err := sidebar.ReadInput(fd, time.Second)
		if err != nil {
			return ""
		}
		var evs []sidebar.Event
		evs, pending = sidebar.Parse(append(pending, b...), true)
		for _, ev := range evs {
			if ev.Kind == sidebar.EvKey {
				return ev.Key
			}
		}
	}
}

// needs returns the program cmd starts with when it is not on the PATH
// ("npm" without Node.js), else "".
func needs(cmd string) string {
	f := strings.Fields(cmd)
	if len(f) == 0 {
		return ""
	}
	switch f[0] {
	case "npm", "curl", "brew":
		if _, err := exec.LookPath(f[0]); err != nil {
			return f[0]
		}
	}
	return ""
}

// continueIn moves the chat in pane to agent id, as a click on it in
// "Continue this chat in" does; the popup closes after it.
func continueIn(e *Env, id session.AgentID, pane, client string) {
	args := []string{"__click", "imux:" + string(id), "--pane", pane}
	if client != "" {
		args = append(args, "--client", client)
	}
	self, err := os.Executable()
	if err != nil {
		return
	}
	if out, err := exec.CommandContext(context.Background(), self, args...).CombinedOutput(); err != nil {
		fmt.Fprintf(e.Stderr, "%v %s\n", err, out)
	}
}
