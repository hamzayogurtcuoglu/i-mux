package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/editor"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/installview"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
)

func init() {
	register(&command{
		Name:    "editor",
		Summary: "cmd.editor",
		Args:    "[install [--yes]]",
		Run:     runEditor,
	})
}

// noticeMicroDeclined records "Use nano" in the dialog that offers micro
// (D45): it is not offered again on its own.
const noticeMicroDeclined = "micro_declined"

// runEditor says which editor files open in; `install` gets micro.
func runEditor(e *Env, args []string) error {
	fs := newFlags(e, "editor")
	yes := fs.Bool("yes", false, "get micro without asking")
	popup := fs.Bool("popup", false, "wait for Enter before closing (Settings)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	install := len(pos) == 1 && pos[0] == "install"
	if len(pos) > 1 || len(pos) == 1 && !install {
		return &UsageError{Msg: i18n.T("cli.usage", "editor [install [--yes]]")}
	}
	cfg, err := loadConfig(e)
	if err != nil {
		return err
	}
	if in, ok := e.Stdin.(*os.File); *popup && ok {
		defer func() {
			fmt.Fprint(e.Stdout, "\n"+i18n.T("font.close"))
			_, _ = bufio.NewReader(in).ReadString('\n')
		}()
	}
	if install {
		return getMicro(e, *yes)
	}
	argv, _ := editorCmd(cfg, "", 0)
	if argv == nil {
		fmt.Fprintln(e.Stdout, i18n.T("editor.none"))
	} else {
		fmt.Fprintln(e.Stdout, i18n.T("editor.status", strings.Join(argv[:len(argv)-1], " ")))
	}
	if !editor.Chosen(cfg.Editor) && !editor.HasMicro() && editor.CanGetMicro() {
		fmt.Fprintln(e.Stdout, i18n.T("editor.micro_hint", editor.MicroMB))
	}
	return nil
}

// getMicro gets micro into ~/.imux/tools: it says what and where, asks
// unless yes, then downloads it with its progress.
func getMicro(e *Env, yes bool) error {
	if m := editor.OurMicro(); m != "" {
		fmt.Fprintln(e.Stdout, i18n.T("editor.already", paths.Shorten(m)))
		return nil
	}
	fmt.Fprintln(e.Stdout, i18n.T("editor.intro", editor.MicroVersion))
	fmt.Fprintln(e.Stdout, i18n.T("editor.plan", editor.MicroMB, paths.Shorten(editor.MicroDir())))
	if !yes {
		in, _ := e.Stdin.(*os.File)
		if in == nil || !isTerminal(in) {
			fmt.Fprintln(e.Stdout, i18n.T("editor.no_terminal"))
			return nil
		}
		if !askYes(e, in, i18n.T("editor.ask_install")) {
			fmt.Fprintln(e.Stdout, i18n.T("font.skipped"))
			return nil
		}
	}
	fmt.Fprintln(e.Stdout)
	out, _ := e.Stdout.(*os.File)
	if err := editor.InstallMicro(context.Background(), e.Stdout, out != nil && isTerminal(out)); err != nil {
		return fmt.Errorf("%s: %w", i18n.T("editor.failed"), err)
	}
	_ = state.Update(func(s *state.State) error { delete(s.Notices, noticeMicroDeclined); return nil })
	fmt.Fprintln(e.Stdout, "\n"+i18n.T("editor.installed", editor.MicroVersion, paths.Shorten(editor.MicroDir())))
	return nil
}

// offerMicro reports whether opening a file with argv asks first whether
// to get micro (D45): nano or vi because nothing else is chosen or
// installed, micro exists for this machine, the user did not say no, and
// there is a terminal to ask on.
func offerMicro(cfg *config.Config, argv []string) bool {
	if len(argv) == 0 || editor.Chosen(cfg.Editor) || editor.HasMicro() || !editor.CanGetMicro() {
		return false
	}
	switch filepath.Base(argv[0]) {
	case "nano", "vim", "vi":
	default:
		return false
	}
	if st, err := state.Read(); err == nil && st.Notices[noticeMicroDeclined] {
		return false
	}
	return isTerminal(os.Stdin) && isTerminal(os.Stdout)
}

// askMicro is the dialog that offers micro before the file opens in name
// (nano): Get micro gets it here; Use nano (or Esc) is remembered.
func askMicro(e *Env, cfg *config.Config, name string) {
	icon := "📝"
	if sidebar.IconTheme(cfg) == sidebar.IconsNerd {
		icon = "\x1b[38;5;214m\uea73\x1b[0m" // codicon "edit"
	}
	m := &installview.Model{
		Icon:  icon,
		Title: i18n.T("editor.ask_title", name),
		Text:  []string{i18n.T("editor.ask_why"), "\t" + i18n.T("editor.ask_size", editor.MicroMB)},
		Choices: []installview.Choice{
			{Label: i18n.T("editor.btn_get"), Key: "g", R: installview.Install},
			{Label: i18n.T("editor.btn_keep", name), Key: "n", R: installview.Close},
		},
		Hint: i18n.T("editor.ask_hint", name),
	}
	r, err := installview.Run(m, os.Stdin, os.Stdout)
	if err != nil {
		return
	}
	if r != installview.Install {
		_ = state.Update(func(s *state.State) error { s.SetNotice(noticeMicroDeclined); return nil })
		return
	}
	fmt.Fprint(e.Stdout, "\x1b[H\x1b[2J"+i18n.T("editor.getting", editor.MicroVersion)+"\n\n")
	if err := editor.InstallMicro(context.Background(), e.Stdout, true); err != nil {
		fmt.Fprintln(e.Stdout, "\n"+i18n.T("editor.failed")+": "+err.Error())
		fmt.Fprint(e.Stdout, "\n"+i18n.T("editor.fallback", name))
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	}
}
