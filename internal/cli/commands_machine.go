package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/machine"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/update"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:    "machine",
		Args:    "add [[<name>] <destination>] | ls | rm <name> | reconnect <name> | open <name> --session S [--remote-pane P] | project <name> <folder> | terminal <name>",
		Summary: "cmd.machine",
		Run:     runMachine,
	})
	register(&command{
		Name:    "__remote-view",
		Args:    "<machine> <session> [<pane>]",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runRemoteView,
	})
}

// OptRemote marks a pane that shows a machine's project ("name/session"):
// the keys i-mux binds without the prefix go on to it (D73).
const OptRemote = tmux.OptRemote

// runMachine manages the machines (D73).
func runMachine(e *Env, args []string) error {
	fs := newFlags(e, "machine")
	sessionF := fs.String("session", "", "open: the machine's project (session)")
	paneF := fs.String("pane", "", "the pane (here) it is for")
	remotePane := fs.String("remote-pane", "", "open: the machine's pane to select")
	clientF := fs.String("client", "", "the client to show it on")
	fromOpt := fs.Bool("from-option", false, "project: the folder and machine from the prompt's options")
	yes := fs.Bool("yes", false, "add: install i-mux there without asking")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	usage := &UsageError{Msg: i18n.T("cli.usage", "machine add [[<name>] <destination>] | ls | rm <name> | reconnect <name> | open <name> --session S [--remote-pane P] | project <name> <folder> | terminal <name>")}
	if len(pos) == 0 {
		return usage
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	mc := &machineCmd{e: e, w: w, ctx: context.Background(), client: *clientF, pane: *paneF}
	if *fromOpt {
		opt := func(k string) string {
			out, _ := w.T.Run(mc.ctx, tmux.Cmd{"show-options", "-gqv", k})
			return strings.TrimSpace(out)
		}
		answer := opt(workspace.OptMachineAnswer)
		_, _ = w.T.Run(mc.ctx, tmux.Cmd{"set-option", "-gu", workspace.OptMachineAnswer})
		mc.pane, mc.client = opt(workspace.OptMachinePane), opt(workspace.OptMachineClient)
		if answer == "" {
			return nil // the prompt was left empty
		}
		if pos[0] == "project" {
			pos = []string{"project", opt(workspace.OptMachineName), answer}
		}
	}
	named := func() (machine.Machine, error) {
		if len(pos) < 2 {
			return machine.Machine{}, usage
		}
		m, err := machine.Get(pos[1])
		if err != nil {
			return m, mc.fail(fmt.Errorf("%s: %w", pos[1], err))
		}
		return m, nil
	}
	switch pos[0] {
	case "add":
		switch len(pos) {
		case 1:
			// The Remote view's +: the destination asked here (D74).
			fmt.Print(i18n.T("machine.add_prompt") + " ")
			line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			if line = strings.TrimSpace(line); line == "" {
				return nil
			}
			return mc.add(machine.NameFor(line), line, *yes, true)
		case 2:
			return mc.add(machine.NameFor(pos[1]), pos[1], *yes, true)
		case 3:
			return mc.add(pos[1], pos[2], *yes, false)
		}
		return usage
	case "ls":
		return mc.list()
	case "rm":
		m, err := named()
		if err != nil {
			return err
		}
		return mc.remove(m)
	case "reconnect":
		m, err := named()
		if err != nil {
			return err
		}
		return mc.reconnect(m)
	case "menu", "confirm-remove":
		if len(pos) < 2 {
			return usage
		}
		menu := w.MachineMenu(pos[1], mc.pane, mc.client)
		if pos[0] == "confirm-remove" {
			menu = w.MachineRemoveMenu(pos[1], mc.pane, mc.client)
		}
		return w.Show(mc.ctx, menu, mc.client, mc.pane)
	case "open":
		m, err := named()
		if err != nil {
			return err
		}
		return mc.open(m, *sessionF, *remotePane)
	case "project":
		m, err := named()
		if err != nil || len(pos) < 3 {
			return errors.Join(err, usage)
		}
		return mc.project(m, strings.Join(pos[2:], " "))
	case "terminal":
		m, err := named()
		if err != nil {
			return err
		}
		return mc.terminal(m)
	}
	return usage
}

type machineCmd struct {
	e            *Env
	w            *workspace.WS
	ctx          context.Context
	client, pane string
}

func (c *machineCmd) say(msg string) {
	if c.client != "" {
		c.w.Message(c.ctx, c.client, c.pane, msg)
		return
	}
	fmt.Fprintln(c.e.Stderr, msg)
}

func (c *machineCmd) fail(err error) error {
	c.say(err.Error())
	return err
}

// ssh runs ssh on this terminal (SSH may ask its own questions).
func (c *machineCmd) ssh(args []string) (int, error) {
	cmd := exec.Command(machine.SSHBin(), args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), nil
	}
	return 0, err
}

// add connects to a new machine where the user sees SSH's own questions
// (its host key, a password), checks its i-mux (offering to install it
// there), then saves it. free: the name was made from the destination,
// and another is found when it is taken.
func (c *machineCmd) add(name, target string, yes, free bool) error {
	if err := machine.CheckTarget(target); err != nil {
		return c.done(c.fail(err))
	}
	if free {
		base := name
		for i := 2; ; i++ {
			if _, err := machine.Get(name); err != nil {
				break
			}
			name = base + "-" + strconv.Itoa(i)
		}
	}
	if err := machine.CheckName(name); err != nil {
		return c.done(c.fail(err))
	}
	if _, err := machine.Get(name); err == nil {
		return c.done(c.fail(machine.ErrExists))
	}
	m := machine.Machine{Name: name, Target: target}
	fmt.Println(i18n.T("machine.connecting_to", target))
	if err := os.MkdirAll(machine.ControlDir(), 0o700); err != nil {
		return c.done(err)
	}
	code, err := c.ssh(machine.Args(m, machine.Interactive, "version"))
	if err != nil {
		return c.done(c.fail(err))
	}
	switch code {
	case 0:
	case 127:
		if !yes && !c.ask(i18n.T("machine.install_ask", name)) {
			return c.done(c.fail(errors.New(i18n.T("machine.install_first", name))))
		}
		if code, err = c.ssh(append(machine.Options(machine.Attach), "--", target, installCommand())); err != nil || code != 0 {
			return c.done(c.fail(errors.New(i18n.T("machine.install_failed", name))))
		}
	default:
		return c.done(c.fail(errors.New(i18n.T("machine.unreachable", target))))
	}
	// Its i-mux must be one that reports (v0.61+).
	if _, st := machine.Fetch(c.ctx, m); st.Status != machine.StatusOK {
		msg := i18n.T("machine.old_imux", name)
		if st.Status == machine.StatusAuth || st.Status == machine.StatusOffline {
			msg = i18n.T("machine.unreachable", target) + ": " + st.Error
		}
		return c.done(c.fail(errors.New(msg)))
	}
	if err := machine.Add(m); err != nil {
		return c.done(c.fail(err))
	}
	c.say(i18n.T("machine.added", name))
	return c.done(nil)
}

// installCommand is the release's installer, run on the machine; it checks
// the archive's checksum itself.
func installCommand() string {
	url := "https://github.com/" + update.Repo + "/releases/latest/download/install.sh"
	return "curl -fsSL " + url + " | sh -s -- --yes"
}

// ask asks a yes/no question on this terminal (no is the default).
func (c *machineCmd) ask(q string) bool {
	fmt.Print(q + " [y/N] ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(line))
	return a == "y" || a == "yes" || a == "e" || a == "evet"
}

// done waits for Enter in a popup, so its last words can be read.
func (c *machineCmd) done(err error) error {
	if isTerminal(os.Stdin) && os.Getenv("TMUX") != "" {
		fmt.Print("\n" + i18n.T("limits.press_enter"))
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
	return err
}

func (c *machineCmd) list() error {
	ms, err := machine.Load()
	if err != nil {
		return err
	}
	if len(ms) == 0 {
		fmt.Fprintln(c.e.Stdout, i18n.T("machine.none"))
		return nil
	}
	for _, m := range ms {
		st := machine.ReadState(m.Name)
		line := fmt.Sprintf("%-16s %-32s %s", m.Name, m.Target, workspace.MachineStatus(st))
		if st.Error != "" && st.Status != machine.StatusOK {
			line += " (" + st.Error + ")"
		}
		fmt.Fprintln(c.e.Stdout, line)
	}
	return nil
}

// remove forgets a machine: its connection closed, its tabs here closed.
func (c *machineCmd) remove(m machine.Machine) error {
	_ = exec.Command(machine.SSHBin(), machine.ExitArgs(m)...).Run()
	if err := machine.Remove(m.Name); err != nil {
		return c.fail(err)
	}
	if out, err := c.w.T.Run(c.ctx, tmux.Cmd{"list-panes", "-a", "-F", "#{pane_id}\t#{" + OptRemote + "}"}); err == nil {
		for _, r := range tmux.Rows(out, 2) {
			if strings.HasPrefix(r[1], m.Name+"/") {
				_, _ = c.w.T.Run(c.ctx, tmux.Cmd{"kill-pane", "-t", r[0]})
			}
		}
	}
	sidebar.RefreshAll(c.ctx, c.w.T)
	c.say(i18n.T("machine.removed", m.Name))
	return nil
}

// reconnect closes the connection and connects again here, where SSH may
// ask (a popup): the watcher then rides on the new one.
func (c *machineCmd) reconnect(m machine.Machine) error {
	_ = exec.Command(machine.SSHBin(), machine.ExitArgs(m)...).Run()
	fmt.Println(i18n.T("machine.connecting_to", m.Target))
	code, err := c.ssh(machine.Args(m, machine.Interactive, "version"))
	if err != nil || code != 0 {
		return c.done(c.fail(errors.New(i18n.T("machine.unreachable", m.Target))))
	}
	if _, st := machine.Fetch(c.ctx, m); st.Status == machine.StatusOK {
		st.At = time.Now()
		_ = machine.WriteState(m.Name, st)
		sidebar.RefreshAll(c.ctx, c.w.T)
	}
	c.say(i18n.T("machine.reconnected", m.Name))
	return c.done(nil)
}

// project opens a folder on the machine as a project there.
func (c *machineCmd) project(m machine.Machine, folder string) error {
	folder = strings.TrimSpace(folder)
	if folder == "" || strings.ContainsAny(folder, "\x00\n\r") {
		return c.fail(errors.New(i18n.T("machine.bad_folder")))
	}
	ctx, cancel := context.WithTimeout(c.ctx, machine.Timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, machine.SSHBin(), machine.Args(m, machine.Background, "open", "-d", "--", folder)...).CombinedOutput()
	if err != nil {
		return c.fail(errors.New(i18n.T("machine.project_failed", m.Name, machine.Clean(lastLine(string(out))))))
	}
	c.say(i18n.T("machine.project_opened", machine.Clean(folder), m.Name))
	return nil
}

// open shows one of the machine's projects here: a tab whose pane runs
// imux __remote-view (the one there is, when there is one).
func (c *machineCmd) open(m machine.Machine, s, pane string) error {
	if !machine.ValidSession(s) || pane != "" && !machine.ValidPane(pane) {
		return c.fail(errors.New(i18n.T("machine.no_project", s)))
	}
	key := m.Name + "/" + s
	if out, err := c.w.T.Run(c.ctx, tmux.Cmd{"list-panes", "-a", "-F", "#{pane_id}\t#{" + OptRemote + "}\t#{window_id}\t#{session_name}"}); err == nil {
		for _, r := range tmux.Rows(out, 4) {
			if r[1] == key {
				return c.show(r[3], r[2])
			}
		}
	}
	project := s
	for _, p := range machine.ReadState(m.Name).Projects {
		if p.Session == s {
			project = p.Name
		}
	}
	target, err := c.localSession()
	if err != nil {
		return c.fail(err)
	}
	self := c.w.Self
	argv := []string{"new-window", "-P", "-F", "#{pane_id}\t#{window_id}", "-t", "=" + target + ":", self, "__remote-view", m.Name, s}
	if pane != "" {
		argv = append(argv, pane)
	}
	out, err := c.w.T.Run(c.ctx, tmux.Cmd(argv))
	if err != nil {
		return c.fail(err)
	}
	f := strings.SplitN(strings.TrimSpace(out), "\t", 2)
	if len(f) != 2 {
		return errors.New("new-window: " + out)
	}
	title := "💻 " + m.Name + " · " + project
	_, _ = c.w.T.Run(c.ctx,
		tmux.Cmd{"set-option", "-p", "-t", f[0], OptRemote, key},
		tmux.Cmd{"set-option", "-w", "-t", f[1], "@imux_sidebar_off", "1"}, // the machine's i-mux has its own
		tmux.Cmd{"set-option", "-w", "-t", f[1], "@imux_tab", title},
		tmux.Cmd{"rename-window", "-t", f[1], title})
	return c.show(target, f[1])
}

// localSession is where a machine's tab opens: the client's session, else
// this pane's, else the first one.
func (c *machineCmd) localSession() (string, error) {
	if c.client != "" {
		if out, err := c.w.T.Run(c.ctx, tmux.Cmd{"display-message", "-p", "-c", c.client, "#{session_name}"}); err == nil && strings.TrimSpace(out) != "" {
			return strings.TrimSpace(out), nil
		}
	}
	if p := firstNonEmpty(c.pane, os.Getenv("TMUX_PANE")); p != "" {
		if out, err := c.w.T.Run(c.ctx, tmux.Cmd{"display-message", "-p", "-t", p, "#{session_name}"}); err == nil && strings.TrimSpace(out) != "" {
			return strings.TrimSpace(out), nil
		}
	}
	ss, err := c.w.Sessions(c.ctx)
	if err != nil || len(ss) == 0 {
		return "", errors.New(i18n.T("machine.no_local"))
	}
	return ss[0].Name, nil
}

func (c *machineCmd) show(sessionName, window string) error {
	cmds := []tmux.Cmd{{"select-window", "-t", window}}
	if c.client != "" {
		cmds = append(cmds, tmux.Cmd{"switch-client", "-c", c.client, "-t", "=" + sessionName})
	}
	_, err := c.w.T.Run(c.ctx, cmds...)
	return err
}

// terminal opens a tab with a shell on the machine.
func (c *machineCmd) terminal(m machine.Machine) error {
	target, err := c.localSession()
	if err != nil {
		return c.fail(err)
	}
	argv := append([]string{"new-window", "-P", "-F", "#{window_id}", "-t", "=" + target + ":", "-n", "💻 " + m.Name, machine.SSHBin()}, machine.ShellArgs(m)...)
	out, err := c.w.T.Run(c.ctx, tmux.Cmd(argv))
	if err != nil {
		return c.fail(err)
	}
	win := strings.TrimSpace(out)
	_, _ = c.w.T.Run(c.ctx, tmux.Cmd{"set-option", "-w", "-t", win, "@imux_sidebar_off", "1"},
		tmux.Cmd{"set-option", "-w", "-t", win, "@imux_tab", "💻 " + m.Name})
	return c.show(target, win)
}

// runRemoteView is a machine's project in this pane (D73): ssh -t to its
// i-mux, again every few seconds while the connection is down, until it is
// detached (ssh's 0) or the user presses Ctrl+C.
func runRemoteView(e *Env, args []string) error {
	if len(args) < 2 || len(args) > 3 || !machine.ValidSession(args[1]) || len(args) == 3 && !machine.ValidPane(args[2]) {
		return &UsageError{Msg: i18n.T("cli.usage", "__remote-view <machine> <session> [<pane>]")}
	}
	m, err := machine.Get(args[0])
	if err != nil {
		return err
	}
	words := append([]string{"__remote-attach"}, args[1:]...)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(stop)
	for {
		cmd := exec.Command(machine.SSHBin(), machine.Args(m, machine.Attach, words...)...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		err := cmd.Run()
		var ee *exec.ExitError
		if err == nil || errors.As(err, &ee) && ee.ExitCode() != 255 {
			return nil // detached, or the project ended there
		}
		fmt.Fprint(e.Stdout, "\r\n"+i18n.T("machine.lost", m.Name)+"\r\n")
		select {
		case <-stop:
			return nil
		case <-time.After(3 * time.Second):
		}
	}
}
