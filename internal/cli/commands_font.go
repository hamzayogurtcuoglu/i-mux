package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/nerdfont"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/termfont"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func init() {
	register(&command{
		Name:    "font",
		Summary: "cmd.font",
		Args:    "[install] [--yes] [--check]",
		Run:     runFont,
	})
}

// runFont shows whether the sidebar's icons have their Nerd Font (D34);
// `install` puts it in place: the font where the terminal draws text (on
// WSL, Windows), the terminals i-mux can set, and icons = "nerd".
func runFont(e *Env, args []string) error {
	fs := newFlags(e, "font")
	yes := fs.Bool("yes", false, "install and set the terminals without asking")
	checkOnly := fs.Bool("check", false, "print nothing; exit 1 when the font is not in place")
	popup := fs.Bool("popup", false, "wait for Enter before closing (Settings)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	install := len(pos) == 1 && pos[0] == "install"
	if len(pos) > 1 || len(pos) == 1 && !install {
		return &UsageError{Msg: i18n.T("cli.usage", "font [install] [--yes] [--check]")}
	}
	in, _ := e.Stdin.(*os.File)
	if *popup && in != nil {
		defer func() {
			fmt.Fprint(e.Stdout, "\n"+i18n.T("font.close"))
			_, _ = bufio.NewReader(in).ReadString('\n')
		}()
	}
	ctx := context.Background()
	sys, err := nerdfont.Detect(ctx)
	if err != nil {
		if *checkOnly {
			return &ExitCodeError{Code: ExitError}
		}
		return fmt.Errorf("%s: %w", i18n.T("font.no_system"), err)
	}
	if *checkOnly {
		if sys.Ready() {
			return nil
		}
		return &ExitCodeError{Code: ExitError}
	}
	icons := "auto"
	if cfg, err := loadConfig(e); err == nil && cfg.Tmux.Icons != "" {
		icons = cfg.Tmux.Icons
	}
	present := sys.Present()
	fontStatus(e, sys, icons, present)
	if !install {
		if !sys.Ready() || icons == "emoji" {
			fmt.Fprintln(e.Stdout, "\n"+i18n.T("font.hint"))
		}
		return nil
	}

	var change []nerdfont.Terminal
	for _, t := range sys.Terminals {
		if !termfont.IsNerdName(t.Font()) {
			change = append(change, t)
		}
	}
	needFont := !sys.Mine() && (len(change) > 0 || len(present) == 0)
	if !needFont && len(change) == 0 && icons == "nerd" {
		fmt.Fprintln(e.Stdout, "\n"+i18n.T("font.ready"))
		return nil
	}
	fmt.Fprintln(e.Stdout, "\n"+i18n.T("font.plan"))
	if needFont {
		where := i18n.T("font.for_you", sys.FontDir)
		if sys.Kind == nerdfont.WSL {
			where = i18n.T("font.for_windows")
		}
		fmt.Fprintln(e.Stdout, "  • "+i18n.T("font.plan_font", nerdfont.Release, nerdfont.SizeMB, where))
	}
	for _, t := range change {
		fmt.Fprintln(e.Stdout, "  • "+i18n.T("font.plan_term", t.Name, filepath.Base(t.File)+".imux-backup"))
	}
	fmt.Fprintln(e.Stdout, "  • "+i18n.T("font.plan_icons"))
	if !*yes {
		if in == nil || !isTerminal(in) {
			fmt.Fprintln(e.Stdout, i18n.T("font.no_terminal"))
			return nil
		}
		fmt.Fprint(e.Stdout, i18n.T("font.ask"))
		// Only an answer counts: no input at all (EOF, /dev/null) is a no.
		line, err := bufio.NewReader(in).ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		if err != nil && answer == "" {
			fmt.Fprintln(e.Stdout, "\n"+i18n.T("font.skipped"))
			return nil
		}
		switch answer {
		case "", "y", "yes", "e", "evet":
		default:
			fmt.Fprintln(e.Stdout, i18n.T("font.skipped"))
			return nil
		}
	}

	fmt.Fprintln(e.Stdout)
	if needFont {
		fmt.Fprint(e.Stdout, i18n.T("font.downloading", nerdfont.Archive, nerdfont.SizeMB))
		if err := sys.Install(ctx); err != nil {
			fmt.Fprintln(e.Stdout)
			return fmt.Errorf("%s: %w", i18n.T("font.failed"), err)
		}
		dir := sys.FontDir
		if sys.WinDir != "" {
			dir = sys.WinDir
		}
		fmt.Fprintln(e.Stdout, i18n.T("font.installed_in", dir))
	}
	var changed []string
	for _, t := range change {
		if _, err := t.Use(sys.Family); err != nil {
			fmt.Fprintln(e.Stdout, i18n.T("font.term_failed", t.Name, err.Error()))
			continue
		}
		changed = append(changed, t.Name)
		fmt.Fprintln(e.Stdout, i18n.T("font.term_set", t.Name, sys.Family))
	}
	// The editor's icons when a terminal i-mux knows has the font (on
	// Linux fontconfig finds it for any terminal); else auto, which follows
	// the terminal once the user picks the font there.
	nerdTerm := false
	for _, t := range sys.Terminals {
		nerdTerm = nerdTerm || termfont.IsNerdName(t.Font())
	}
	want := icons
	switch {
	case nerdTerm || sys.Kind == nerdfont.Linux:
		want = "nerd"
	case icons == "emoji":
		want = "auto"
	}
	if want != icons {
		if err := setSetting("icons", want); err != nil {
			fmt.Fprintln(e.Stdout, i18n.T("settings.failed", err.Error()))
		} else {
			sidebar.RefreshAll(ctx, tmux.New(paths.TmuxConf()))
		}
	}
	fmt.Fprintln(e.Stdout, i18n.T("font.icons", want))
	fmt.Fprintln(e.Stdout)
	switch {
	case len(changed) > 0:
		fmt.Fprintln(e.Stdout, i18n.T("font.restart", joinAnd(changed)))
	case sys.Kind == nerdfont.MacOS && !nerdTerm:
		fmt.Fprintln(e.Stdout, i18n.T("font.choose_mac", sys.Family))
	case !nerdTerm:
		fmt.Fprintln(e.Stdout, i18n.T("font.choose", sys.Family))
	default:
		fmt.Fprintln(e.Stdout, i18n.T("font.ready"))
	}
	return nil
}

// fontStatus prints the icons setting, the font and each terminal's font.
func fontStatus(e *Env, sys *nerdfont.System, icons string, present []string) {
	fmt.Fprintln(e.Stdout, i18n.T("font.title", icons))
	fmt.Fprintln(e.Stdout)
	tw := tabwriter.NewWriter(e.Stdout, 0, 0, 3, ' ', 0)
	state := i18n.T("font.missing")
	switch {
	case sys.Mine():
		dir := sys.FontDir
		if sys.WinDir != "" {
			dir = sys.WinDir
		}
		state = i18n.T("font.installed", dir)
	case len(present) > 0:
		state = i18n.T("font.other", filepath.Base(present[0]))
	}
	fmt.Fprintf(tw, "  %s\t%s\n", nerdfont.Family, state)
	for _, t := range sys.Terminals {
		f := t.Font()
		switch {
		case termfont.IsNerdName(f):
			f += " ✓"
		case f == "":
			f = i18n.T("font.term_default")
		default:
			f = i18n.T("font.term_other", f)
		}
		fmt.Fprintf(tw, "  %s\t%s\n", t.Name, f)
	}
	_ = tw.Flush()
}

// joinAnd lists names: "A", "A and B", "A, B and C".
func joinAnd(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}
	return strings.Join(names[:len(names)-1], ", ") + i18n.T("font.and") + names[len(names)-1]
}
