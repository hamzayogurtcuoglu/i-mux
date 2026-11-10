package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/fileops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/installview"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/webview"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:    "web",
		Summary: "cmd.web",
		Args:    "[install [--yes] | remove | open <file|url>]",
		Run:     runWeb,
	})
	register(&command{
		Name:    "__web-pane",
		Args:    "<url>",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runWebPane,
	})
	register(&command{
		Name:    "__web-ask",
		Args:    "<url> [--pane %N] [--client C]",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runWebAsk,
	})
}

// runWeb is the web preview (D44): whether it is on and installed;
// install and remove its browser; open a page beside a pane.
func runWeb(e *Env, args []string) error {
	fs := newFlags(e, "web")
	yes := fs.Bool("yes", false, "install without asking")
	popup := fs.Bool("popup", false, "wait for Enter before closing (Settings)")
	paneF := fs.String("pane", "", "open: the pane the page opens beside (default: this pane)")
	clientF := fs.String("client", "", "the client for messages and popups")
	as := fs.String("as", "preview", "open: preview, browser (the system's) or edit")
	fromOpt := fs.Bool("from-option", false, "open: the page, pane and client a prompt left in options")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	usage := &UsageError{Msg: i18n.T("cli.usage", "web [install [--yes] | remove | open <file|url>]")}
	cfg, err := loadConfig(e)
	if err != nil {
		return err
	}
	if in, ok := e.Stdin.(*os.File); *popup && ok {
		defer func() {
			fmt.Fprint(e.Stdout, "\n"+i18n.T("web.press_enter"))
			_, _ = bufio.NewReader(in).ReadString('\n')
		}()
	}
	switch {
	case len(pos) == 0:
		webStatus(e, cfg)
		return nil
	case pos[0] == "install" && len(pos) == 1:
		return webInstall(e, cfg, *yes)
	case pos[0] == "remove" && len(pos) == 1:
		return webRemove(e, *clientF)
	case pos[0] == "open" && len(pos) == 2 && !*fromOpt, pos[0] == "open" && len(pos) == 1 && *fromOpt:
		w, _, err := newWorkspace(e, false)
		if err != nil {
			return err
		}
		return webOpen(w, strings.Join(pos[1:], ""), *paneF, *clientF, *as, *fromOpt)
	}
	return usage
}

func webStatus(e *Env, cfg *config.Config) {
	state := i18n.T("settings.off")
	if cfg.Web.Enabled {
		state = i18n.T("settings.on_plain")
	}
	fmt.Fprintln(e.Stdout, i18n.T("web.status", state))
	b := webview.Browser(cfg)
	switch {
	case cfg.Web.Browser != "" && b == nil:
		fmt.Fprintln(e.Stdout, i18n.T("web.status_missing", cfg.Web.Browser))
	case cfg.Web.Browser != "":
		fmt.Fprintln(e.Stdout, i18n.T("web.status_browser", strings.Join(b, " ")))
	case b == nil:
		fmt.Fprintln(e.Stdout, i18n.T("web.status_none", webview.DownloadMB))
	default:
		fmt.Fprintln(e.Stdout, i18n.T("web.status_browser", "Carbonyl "+webview.Version+" ("+paths.Shorten(webview.Dir())+")"))
	}
}

// webInstall installs Carbonyl: it says what and where, asks unless yes,
// downloads, checks, unpacks, then finds missing libraries and runs it once.
func webInstall(e *Env, cfg *config.Config, yes bool) error {
	ctx := context.Background()
	if cfg.Web.Browser != "" {
		fmt.Fprintln(e.Stdout, i18n.T("web.custom", cfg.Web.Browser))
		return nil
	}
	in, _ := e.Stdin.(*os.File)
	if !webview.Installed(cfg) {
		fmt.Fprintln(e.Stdout, i18n.T("web.intro", webview.Version))
		fmt.Fprintln(e.Stdout, i18n.T("web.plan", webview.DownloadMB, webview.DiskMB, paths.Shorten(webview.Dir())))
		if !yes {
			if in == nil || !isTerminal(in) {
				fmt.Fprintln(e.Stdout, i18n.T("web.no_terminal"))
				return nil
			}
			if !askYes(e, in, i18n.T("web.ask")) {
				fmt.Fprintln(e.Stdout, i18n.T("font.skipped"))
				return nil
			}
		}
		fmt.Fprintln(e.Stdout)
		out, _ := e.Stdout.(*os.File)
		inst := &webview.Installer{Out: e.Stdout, Bar: out != nil && isTerminal(out)}
		if err := inst.Install(ctx); err != nil {
			return fmt.Errorf("%s: %w", i18n.T("web.failed"), err)
		}
	}
	if !webLibs(ctx, e, in) {
		return errors.New(i18n.T("web.libs_missing_after"))
	}
	argv := webview.Browser(cfg)
	if os.Geteuid() == 0 {
		argv = append(argv, "--no-sandbox")
	}
	v, err := webview.CheckVersion(ctx, argv)
	if err != nil {
		if v != "" {
			fmt.Fprintln(e.Stdout, v)
		}
		return fmt.Errorf("%s: %w", i18n.T("web.failed"), err)
	}
	fmt.Fprintln(e.Stdout, "\n"+i18n.T("web.installed", v, paths.Shorten(webview.Dir())))
	return nil
}

// webLibs names the libraries Carbonyl needs that this machine lacks and
// offers to install them with apt; it reports whether none are missing.
func webLibs(ctx context.Context, e *Env, in *os.File) bool {
	libs := webview.Missing(ctx, webview.Bin())
	if len(libs) == 0 {
		return true
	}
	fmt.Fprintln(e.Stdout, "\n"+i18n.T("web.libs", strings.Join(libs, ", ")))
	pkgs := webview.AptPackages(ctx, libs)
	if len(pkgs) == 0 {
		fmt.Fprintln(e.Stdout, i18n.T("web.libs_by_hand"))
		return false
	}
	argv := append([]string{"sudo", "apt-get", "install", "-y"}, pkgs...)
	if os.Geteuid() == 0 {
		argv = argv[1:]
	}
	fmt.Fprintln(e.Stdout, i18n.T("web.libs_cmd", strings.Join(argv, " ")))
	if in == nil || !isTerminal(in) || !askYes(e, in, i18n.T("web.libs_ask")) {
		return false
	}
	c := exec.Command(argv[0], argv[1:]...)
	c.Stdin, c.Stdout, c.Stderr = in, e.Stdout, e.Stderr
	if err := c.Run(); err != nil {
		return false
	}
	return len(webview.Missing(ctx, webview.Bin())) == 0
}

// askYes asks q; Enter or y is a yes, no input at all a no.
func askYes(e *Env, in io.Reader, q string) bool {
	fmt.Fprint(e.Stdout, q)
	line, err := bufio.NewReader(in).ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(line))
	if err != nil && a == "" {
		fmt.Fprintln(e.Stdout)
		return false
	}
	switch a {
	case "", "y", "yes", "e", "evet":
		return true
	}
	return false
}

func webRemove(e *Env, client string) error {
	_, err := os.Stat(webview.Dir())
	had := err == nil
	if err := webview.Remove(); err != nil {
		return err
	}
	msg := i18n.T("web.removed", webview.DiskMB)
	if !had {
		msg = i18n.T("web.not_there")
	}
	fmt.Fprintln(e.Stdout, msg)
	if client != "" {
		if w, _, err := newWorkspace(e, false); err == nil {
			w.Message(context.Background(), client, "", msg)
		}
	}
	return nil
}

// webOpen opens a page (a file, or a URL) as asked: in the preview beside
// pane, in the system's browser, or a file in the editor.
func webOpen(w *workspace.WS, what, pane, client, as string, fromOpt bool) error {
	ctx := context.Background()
	if fromOpt {
		out, err := w.T.Run(ctx,
			tmux.DelimCmd("url"), tmux.Cmd{"show-options", "-gqv", workspace.OptWebURL},
			tmux.DelimCmd("pane"), tmux.Cmd{"show-options", "-gqv", workspace.OptWebPane},
			tmux.DelimCmd("client"), tmux.Cmd{"show-options", "-gqv", workspace.OptWebClient})
		if err != nil {
			return err
		}
		sec := tmux.Sections(out)
		val := func(k string) string { return strings.TrimSpace(sec[k]) }
		what, pane, client = val("url"), val("pane"), val("client")
	}
	if pane == "" && client == "" {
		if p, err := targetPane(""); err == nil {
			pane = p
		}
	}
	dir := ""
	if pane != "" {
		if out, err := w.T.Run(ctx, tmux.Cmd{"display-message", "-p", "-t", pane, "#{pane_current_path}"}); err == nil {
			dir = strings.TrimSpace(out)
		}
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	u := webview.Target(what, dir)
	if u == "" {
		return &UsageError{Msg: i18n.T("cli.usage", "web open <file|url>")}
	}
	file := webview.FilePath(u)
	if file != "" {
		dir = filepath.Dir(file)
	}
	switch as {
	case "browser":
		if file != "" {
			return (&fileops.System{}).Browse(ctx, file)
		}
		return (&fileops.System{}).Browse(ctx, u)
	case "edit":
		if file == "" {
			return errors.New(i18n.T("web.not_a_file", what))
		}
		c := tmux.Cmd{"display-popup"}
		if client != "" {
			c = append(c, "-c", client)
		}
		return w.T.Spawn(append(c, "-E", "-w", "90%", "-h", "90%", "-d", tmux.EscapeFormat(dir), tmux.ShJoin(w.Self, "__edit", file)))
	case "preview":
	default:
		return &UsageError{Msg: i18n.T("cli.usage", "web open --as preview|browser|edit <file|url>")}
	}
	if pane == "" {
		return &UsageError{Msg: i18n.T("cli.no_pane")}
	}
	if !webview.Installed(w.Cfg) {
		if client == "" {
			return errors.New(i18n.T("web.not_installed"))
		}
		// The popup that offers to install it (§9.3 "Web preview").
		args := []string{w.Self, "__web-ask", u, "--pane", pane, "--client", client}
		return w.T.Spawn(tmux.Cmd{"display-popup", "-c", client, "-E", "-w", "76", "-h", "15", "-d", tmux.EscapeFormat(dir), tmux.ShJoin(args...)})
	}
	return webPane(ctx, w, pane, u, dir)
}

// webPane shows u in the web pane of pane's window, which then loads it
// (a second click on a file reloads it), else in a new pane right of pane.
func webPane(ctx context.Context, w *workspace.WS, pane, u, dir string) error {
	run := tmux.ShJoin(w.Self, "__web-pane", u)
	name := webview.Label(u)
	out, err := w.T.Run(ctx, tmux.Cmd{"list-panes", "-t", pane, "-F", "#{pane_id}\t#{" + sidebar.OptWeb + "}"})
	if err != nil {
		return err
	}
	for _, r := range tmux.Rows(out, 2) {
		if r[1] != "" {
			_, err := w.T.Run(ctx,
				tmux.Cmd{"respawn-pane", "-k", "-t", r[0], "-c", tmux.EscapeFormat(dir), run},
				tmux.Cmd{"set-option", "-p", "-t", r[0], sidebar.OptWeb, u},
				tmux.Cmd{"set-option", "-p", "-t", r[0], sidebar.OptName, name},
				tmux.Cmd{"select-pane", "-t", r[0]})
			return err
		}
	}
	// No -d: the options that follow go to the new pane, the current one.
	_, err = w.T.Run(ctx,
		tmux.Cmd{"split-window", "-h", "-t", pane, "-c", tmux.EscapeFormat(dir), run},
		tmux.Cmd{"set-option", "-p", sidebar.OptWeb, u},
		tmux.Cmd{"set-option", "-p", sidebar.OptName, name})
	return err
}

// runWebPane is a web pane's program: the browser on the page, until it
// exits; when it fails, why, until Enter.
func runWebPane(e *Env, args []string) error {
	fs := newFlags(e, "__web-pane")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "__web-pane <url>")}
	}
	cfg, err := loadConfig(e)
	if err != nil {
		return err
	}
	wait := func() {
		fmt.Fprint(e.Stdout, "\n"+i18n.T("web.press_enter"))
		_, _ = bufio.NewReader(e.Stdin).ReadString('\n')
	}
	argv := webview.Command(cfg, pos[0], os.Geteuid() == 0)
	if argv == nil {
		fmt.Fprintln(e.Stdout, i18n.T("web.not_installed"))
		wait()
		return nil
	}
	c := exec.Command(argv[0], argv[1:]...)
	c.Env = append(os.Environ(), "COLORTERM=truecolor")
	c.Stdin, c.Stdout = e.Stdin, e.Stdout
	tail := &tailBuffer{max: 8 << 10}
	c.Stderr = tail // Chromium's logs would draw over the page
	// Ctrl+C is the browser's: this process stays to say why it failed.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGQUIT)
	err = c.Run()
	signal.Stop(sig)
	if err == nil || closed(err) {
		return nil
	}
	fmt.Fprint(e.Stdout, "\x1b[0m\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1049l\x1b[?25h\x1b[H\x1b[2J")
	fmt.Fprintln(e.Stdout, i18n.T("web.pane_failed", err.Error()))
	if t := strings.TrimSpace(tail.String()); t != "" {
		lines := strings.Split(t, "\n")
		fmt.Fprintln(e.Stdout, "\x1b[38;5;245m"+strings.Join(lines[max(len(lines)-12, 0):], "\n")+"\x1b[0m")
	}
	if libs := webview.Missing(context.Background(), argv[0]); len(libs) > 0 {
		fmt.Fprintln(e.Stdout, "\n"+i18n.T("web.libs", strings.Join(libs, ", ")))
		fmt.Fprintln(e.Stdout, i18n.T("web.libs_settings"))
	}
	wait()
	return nil
}

// closed reports a browser that was closed with its pane (or killed).
func closed(err error) bool {
	var ee *exec.ExitError
	if !errors.As(err, &ee) {
		return false
	}
	ws, ok := ee.Sys().(syscall.WaitStatus)
	if !ok || !ws.Signaled() {
		return false
	}
	switch ws.Signal() {
	case syscall.SIGHUP, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGINT:
		return true
	}
	return false
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	b   []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if len(t.b) > t.max {
		t.b = t.b[len(t.b)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.b)
}

// runWebAsk is the popup for a page while the preview has no browser: it
// installs it (then opens the page), opens the page in the system's
// browser, or the file in the editor.
func runWebAsk(e *Env, args []string) error {
	fs := newFlags(e, "__web-ask")
	paneF := fs.String("pane", "", "the pane the page opens beside")
	clientF := fs.String("client", "", "the client the popup is on")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "__web-ask <url>")}
	}
	cfg, err := loadConfig(e)
	if err != nil {
		return err
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	u, sys := pos[0], &fileops.System{}
	file := webview.FilePath(u)
	choices := []installview.Choice{{Label: i18n.T("web.btn_install"), Key: "i", R: installview.Install}}
	if sys.CanBrowse() {
		choices = append(choices, installview.Choice{Label: i18n.T("web.btn_browser"), Key: "b", R: installview.Browser})
	}
	if file != "" {
		choices = append(choices, installview.Choice{Label: i18n.T("web.btn_edit"), Key: "e", R: installview.Edit})
	}
	choices = append(choices, installview.Choice{Label: i18n.T("install.close"), R: installview.Close})
	m := &installview.Model{
		Icon:  "\x1b[38;5;39m⊕\x1b[0m",
		Title: i18n.T("web.ask_title"),
		Text: []string{
			i18n.T("web.ask_why", webview.Label(u)),
			"\t" + i18n.T("web.ask_size", webview.DownloadMB, webview.DiskMB),
		},
		Choices: choices,
		Hint:    i18n.T("web.ask_hint"),
	}
	r, err := installview.Run(m, os.Stdin, os.Stdout)
	if err != nil {
		return err
	}
	ctx := context.Background()
	switch r {
	case installview.Browser:
		if file != "" {
			return sys.Browse(ctx, file)
		}
		return sys.Browse(ctx, u)
	case installview.Edit:
		// The editor's popup once this one has gone (one at a time).
		edit := []string{w.Self, "web", "open", "--as", "edit", file}
		if *clientF != "" {
			edit = append(edit, "--client", *clientF)
		}
		_, err := w.T.Run(ctx, tmux.Cmd{"run-shell", "-b", "-d", "0.15", tmux.EscapeFormat(tmux.ShJoin(edit...) + tmux.Silent)})
		return err
	case installview.Install:
		fmt.Fprint(e.Stdout, "\x1b[H\x1b[2J")
		if err := webInstall(e, cfg, true); err != nil {
			fmt.Fprintln(e.Stdout, "\n"+err.Error())
			fmt.Fprint(e.Stdout, "\n"+i18n.T("web.press_enter"))
			_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
			return err
		}
		fmt.Fprint(e.Stdout, "\n"+i18n.T("web.open_after", webview.Label(u)))
		if key() != "enter" || *paneF == "" {
			return nil
		}
		dir := filepath.Dir(file)
		if file == "" {
			dir, _ = os.Getwd()
		}
		return webPane(ctx, w, *paneF, u, dir)
	}
	return nil
}
