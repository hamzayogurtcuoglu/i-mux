package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/hamzayogurtcuoglu/i-mux/internal/completion"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func init() {
	register(&command{
		Name:    "settings",
		Summary: "cmd.settings",
		Args:    "[--pane P]",
		Run:     runSettingsOpen,
	})
	register(&command{
		Name:   "__settings",
		Args:   "set <key> <value> | menu [commit-agent] | edit  [--pane P] [--client C]",
		Hidden: true,
		Run:    runSettings,
	})
	register(&command{
		Name:   "__completion",
		Args:   "setup [--yes] | signin",
		Hidden: true,
		Run:    runCompletion,
	})
}

// settingKey is where a Settings row is kept in config.toml and which
// values it takes.
type settingKey struct {
	section, key string
	allowed      []string
	quote        bool
}

var settingKeys = map[string]settingKey{
	"icons":        {"tmux", "icons", []string{"auto", "nerd", "emoji"}, true},
	"commit_agent": {"git", "commit_agent", append([]string{"auto"}, config.AgentIDs...), true},
	"on_arrival":   {"handoff", "on_arrival", []string{"wait", "continue"}, true},
	"on_limit":     {"", "on_limit", []string{"ask", "auto", "off"}, true},
	"completion":   {"completion", "enabled", []string{"on", "off"}, false},
	"notify":       {"notify", "enabled", []string{"on", "off"}, false},
	"notify_sound": {"notify", "sound", []string{"on", "off"}, false},
	"paste_images": {"paste", "images", []string{"on", "off"}, false},
}

// runSettingsOpen opens the Settings menu on the client showing this pane
// (`imux settings`, ⚙ on the rail).
func runSettingsOpen(e *Env, args []string) error {
	fs := newFlags(e, "settings")
	paneF := fs.String("pane", "", "pane whose client shows the menu (default: this pane)")
	if _, err := parseFlags(fs, args); err != nil {
		return err
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
	p, err := w.PaneTarget(ctx, pane)
	if err != nil {
		return err
	}
	return w.Show(ctx, w.SettingsMenu(p.ID, w.ClientFor(ctx, p)), w.ClientFor(ctx, p), p.ID)
}

// runSettings changes one setting and opens the menu again, shows a menu,
// or edits the config file (in a popup).
func runSettings(e *Env, args []string) error {
	fs := newFlags(e, "__settings")
	paneF := fs.String("pane", "", "pane the menu is shown for")
	clientF := fs.String("client", "", "client that shows it")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	usage := &UsageError{Msg: i18n.T("cli.usage", "__settings set <key> <value> | menu [<row>] | edit")}
	if len(pos) == 0 {
		return usage
	}
	ctx := context.Background()
	switch pos[0] {
	case "set":
		if len(pos) != 3 {
			return usage
		}
		if err := setSetting(pos[1], pos[2]); err != nil {
			w, _, werr := newWorkspace(e, false)
			if werr == nil {
				w.Message(ctx, *clientF, *paneF, i18n.T("settings.failed", err.Error()))
			}
			return err
		}
		sidebar.RefreshAll(ctx, tmux.New(paths.TmuxConf()))
		return showSettings(e, *paneF, *clientF, "")
	case "menu":
		which := ""
		if len(pos) > 1 {
			which = pos[1]
		}
		return showSettings(e, *paneF, *clientF, which)
	case "edit":
		if _, err := os.Stat(paths.Config()); errors.Is(err, os.ErrNotExist) {
			if err := paths.Ensure(); err != nil {
				return err
			}
			if err := config.WriteTemplate(paths.Config(), false); err != nil {
				return err
			}
		}
		return runEdit(e, []string{paths.Config()})
	}
	return usage
}

// setSetting writes one Settings row's value into config.toml.
func setSetting(name, value string) error {
	if name == "editor" {
		return setEditor(value)
	}
	k, ok := settingKeys[name]
	if !ok {
		return fmt.Errorf("unknown setting %q", name)
	}
	allowed := false
	for _, a := range k.allowed {
		allowed = allowed || a == value
	}
	if !allowed {
		return fmt.Errorf("%s: invalid value %q", name, value)
	}
	v := config.Str(value)
	if !k.quote {
		v = config.Bool(value == "on")
	}
	if err := paths.Ensure(); err != nil {
		return err
	}
	return config.Set(paths.Config(), k.section, k.key, v)
}

// setEditor saves the Editor row's choice (D50): copilot turns AI
// completion on (its setup ran first); any other sets `editor` ("" for
// auto) and turns AI completion off.
func setEditor(value string) error {
	switch value {
	case "auto", "micro", "nano", "nvim", "copilot":
	default:
		return fmt.Errorf("editor: invalid value %q", value)
	}
	if err := paths.Ensure(); err != nil {
		return err
	}
	if value == "copilot" {
		return config.Set(paths.Config(), "completion", "enabled", config.Bool(true))
	}
	ed := value
	if value == "auto" {
		ed = ""
	}
	if err := config.Set(paths.Config(), "", "editor", config.Str(ed)); err != nil {
		return err
	}
	return config.Set(paths.Config(), "completion", "enabled", config.Bool(false))
}

// showSettings shows the Settings menu (or a submenu) with a fresh config.
func showSettings(e *Env, pane, client, which string) error {
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	m := w.SettingsMenu(pane, client)
	if sub := w.SettingMenu(which, pane, client); sub != nil {
		m = sub
	}
	return w.Show(context.Background(), m, client, pane)
}

// runCompletion sets up AI completion in the editor or signs in to GitHub
// Copilot (D30). Both run in a popup: they talk to the user.
func runCompletion(e *Env, args []string) error {
	fs := newFlags(e, "__completion")
	yes := fs.Bool("yes", false, "answer yes to every question")
	_ = fs.String("pane", "", "pane the Settings menu was shown for")
	_ = fs.String("client", "", "client that showed it")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "__completion setup|signin")}
	}
	in := bufio.NewReader(os.Stdin)
	done := func() {
		if !*yes {
			fmt.Fprint(e.Stdout, "\n"+i18n.T("completion.press_enter"))
			_, _ = in.ReadString('\n')
		}
	}
	run := func(name string, a ...string) error {
		cmd := exec.Command(name, a...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, e.Stdout, e.Stderr
		return cmd.Run()
	}
	switch pos[0] {
	case "setup":
		fmt.Fprintln(e.Stdout, i18n.T("completion.setting_up")+"\n")
		brew, _ := exec.LookPath("brew")
		s := &completion.Setup{Out: e.Stdout, In: in, Yes: *yes, Run: run, Brew: brew}
		if err := s.Do(); err != nil {
			fmt.Fprintln(e.Stdout, "\n"+i18n.T("completion.failed", err.Error()))
			done()
			return err
		}
		if err := paths.Ensure(); err != nil {
			return err
		}
		if err := config.Set(paths.Config(), "completion", "enabled", config.Bool(true)); err != nil {
			fmt.Fprintln(e.Stdout, i18n.T("settings.failed", err.Error()))
			done()
			return err
		}
		sidebar.RefreshAll(context.Background(), tmux.New(paths.TmuxConf()))
		fmt.Fprintln(e.Stdout, "\n"+i18n.T("completion.ready"))
		if !*yes && s.Ask("\n"+i18n.T("completion.signin_now")) {
			return signIn(e, run, done)
		}
		done()
		return nil
	case "signin":
		return signIn(e, run, done)
	}
	return &UsageError{Msg: i18n.T("cli.usage", "__completion setup|signin")}
}

// signIn opens Neovim in the profile on `:Copilot setup` (a GitHub device
// code; one time).
func signIn(e *Env, run func(string, ...string) error, done func()) error {
	nvim := completion.Nvim()
	if nvim == "" || !completion.Ready() {
		fmt.Fprintln(e.Stdout, i18n.T("completion.not_ready"))
		done()
		return completion.ErrNotReady
	}
	fmt.Fprintln(e.Stdout, i18n.T("completion.signin_help"))
	if err := os.Setenv("NVIM_APPNAME", completion.AppName); err != nil {
		return err
	}
	err := run(nvim, "+Copilot setup")
	done()
	return err
}
