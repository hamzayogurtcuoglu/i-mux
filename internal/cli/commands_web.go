package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/fileops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/webview"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:    "web",
		Summary: "cmd.web",
		Args:    "[open <file|url> | stop]",
		Run:     runWeb,
	})
	register(&command{
		Name:    "__web-serve",
		Args:    "<folder>",
		Summary: "cmd.internal",
		Hidden:  true,
		Run:     runWebServe,
	})
}

// runWeb is the live preview (D47): the folders served and their pages;
// open a page (a file, served with live reload, or an address) in the
// browser; stop the servers.
func runWeb(e *Env, args []string) error {
	fs := newFlags(e, "web")
	paneF := fs.String("pane", "", "open: the pane whose project serves the file (default: this pane)")
	clientF := fs.String("client", "", "the client for messages")
	as := fs.String("as", "", "open: window or tab (default: [web] open)")
	fromOpt := fs.Bool("from-option", false, "open: the page, pane and client a prompt left in options")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	usage := &UsageError{Msg: i18n.T("cli.usage", "web [open <file|url> | stop]")}
	ctx := context.Background()
	switch {
	case len(pos) == 0:
		cfg, err := loadConfig(e)
		if err != nil {
			return err
		}
		webStatus(ctx, e, cfg)
		return nil
	case pos[0] == "stop" && len(pos) == 1:
		all := webview.All(ctx)
		for _, i := range all {
			i.Stop()
		}
		fmt.Fprintln(e.Stdout, i18n.T("web.stopped", len(all)))
		return nil
	case pos[0] == "open" && (len(pos) == 2 && !*fromOpt || len(pos) == 1 && *fromOpt):
		if *as != "" && *as != "window" && *as != "tab" {
			return usage
		}
		w, _, err := newWorkspace(e, false)
		if err != nil {
			return err
		}
		return webOpen(ctx, w, strings.Join(pos[1:], ""), *paneF, *clientF, *as, *fromOpt)
	}
	return usage
}

func webStatus(ctx context.Context, e *Env, cfg *config.Config) {
	switch {
	case !cfg.Web.Enabled:
		fmt.Fprintln(e.Stdout, i18n.T("web.status_off"))
	case cfg.Web.Open == "tab":
		fmt.Fprintln(e.Stdout, i18n.T("web.status_tab"))
	default:
		fmt.Fprintln(e.Stdout, i18n.T("web.status_window"))
	}
	all := webview.All(ctx)
	if len(all) == 0 {
		fmt.Fprintln(e.Stdout, i18n.T("web.none"))
	}
	for _, i := range all {
		st, _ := i.Ping(ctx)
		fmt.Fprintln(e.Stdout, i18n.T("web.serving", paths.Shorten(i.Root), i.URL("/"), st.Pages))
	}
}

// webOpen opens a page: a file (or folder) is served from its project with
// live reload, and an open page of that server shows it; else, as an
// address, the browser opens it.
func webOpen(ctx context.Context, w *workspace.WS, what, pane, client, as string, fromOpt bool) error {
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
	dir, project := "", ""
	if pane != "" {
		if out, err := w.T.Run(ctx, tmux.Cmd{"display-message", "-p", "-t", pane, "#{pane_current_path}\t#{session_path}"}); err == nil {
			if f := strings.SplitN(strings.TrimSpace(out), "\t", 2); len(f) == 2 {
				dir, project = f[0], f[1]
			}
		}
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	file, u := webview.Target(what, dir)
	if file == "" && u == "" {
		return &UsageError{Msg: i18n.T("cli.usage", "web open <file|url>")}
	}
	say := func(msg string) {
		if client != "" || pane != "" {
			w.Message(ctx, client, pane, msg)
		} else {
			fmt.Println(msg)
		}
	}
	label := webview.Label(u)
	if file != "" {
		label = filepath.Base(file)
		root := webview.RootFor(file, project)
		info, err := webview.Ensure(ctx, w.Self, root)
		if err != nil {
			say(i18n.T("web.serve_failed", err.Error()))
			return err
		}
		p := webview.URLPath(root, file)
		if st, err := info.Show(ctx, p); err == nil && st.Shown {
			// A page of this folder is open: it shows this one.
			(&fileops.System{}).Raise(st.Title)
			say(i18n.T("web.shown", label))
			return nil
		}
		u = info.URL(p)
	}
	if as == "" {
		as = w.Cfg.Web.Open
	}
	name, err := openPage(ctx, w, u, as != "tab")
	switch {
	case errors.Is(err, fileops.ErrNoBrowser):
		// Nowhere to show it here (SSH): the address, to open elsewhere.
		_, _ = w.T.Run(ctx, tmux.Cmd{"set-buffer", "-w", "--", u})
		say(i18n.T("web.no_browser", u))
		return nil
	case err != nil:
		say(i18n.T("web.open_failed", err.Error()))
		return err
	}
	if name == "" {
		name = i18n.T("web.your_browser")
	}
	if file != "" {
		say(i18n.T("web.opened_live", label, name))
	} else {
		say(i18n.T("web.opened", label, name))
	}
	return nil
}

// openPage opens u: with [web] browser, else VS Code's helper when the
// client is VS Code's terminal, else the system's browser (window: an app
// window). It returns the browser's name ("" when unknown).
func openPage(ctx context.Context, w *workspace.WS, u string, window bool) (string, error) {
	if f := strings.Fields(w.Cfg.Web.Browser); len(f) > 0 {
		cmd := exec.Command(f[0], append(f[1:], u)...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			return "", err
		}
		go func() { _ = cmd.Wait() }()
		return filepath.Base(f[0]), nil
	}
	out, _ := w.T.Run(ctx,
		tmux.DelimCmd("b"), tmux.Cmd{"show-options", "-gqv", workspace.OptBrowser},
		tmux.DelimCmd("i"), tmux.Cmd{"show-options", "-gqv", workspace.OptVSCodeIPC})
	sec := tmux.Sections(out)
	helper, ipc := strings.TrimSpace(sec["b"]), strings.TrimSpace(sec["i"])
	if helper != "" && ipc != "" && exists(helper) && exists(ipc) {
		cmd := exec.CommandContext(ctx, helper, u)
		cmd.Env = append(os.Environ(), "VSCODE_IPC_HOOK_CLI="+ipc)
		if err := cmd.Run(); err == nil {
			return "VS Code", nil
		}
	}
	return (&fileops.System{}).OpenPage(ctx, u, window)
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// runWebServe is a folder's server (`imux __web-serve <folder>`, started
// detached by Ensure): it runs until idle or stopped.
func runWebServe(e *Env, args []string) error {
	fs := newFlags(e, "__web-serve")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 || !filepath.IsAbs(pos[0]) {
		return &UsageError{Msg: i18n.T("cli.usage", "__web-serve <folder>")}
	}
	return webview.Serve(context.Background(), pos[0])
}
