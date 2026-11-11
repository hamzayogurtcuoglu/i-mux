package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/watch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/webview"
)

func init() {
	register(&command{
		Name:    "quit",
		Summary: "cmd.quit",
		Args:    "[--yes]",
		Run:     runQuit,
	})
}

// runQuit closes i-mux (D48): the open projects are recorded, the live
// preview servers and the watcher stop, and the tmux server ends with
// every agent. --menu asks in a tmux menu on --client instead (the tab
// bar's ⏻ Exit).
func runQuit(e *Env, args []string) error {
	fs := newFlags(e, "quit")
	yes := fs.Bool("yes", false, "quit without asking")
	menu := fs.Bool("menu", false, "ask in a menu on --client")
	paneF := fs.String("pane", "", "the pane the menu is for")
	clientF := fs.String("client", "", "the client that shows the menu")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 0 {
		return &UsageError{Msg: i18n.T("cli.usage", "quit [--yes]")}
	}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if w.ServerID(ctx) == "" {
		fmt.Fprintln(e.Stdout, i18n.T("quit.not_running"))
		return nil
	}
	if *menu {
		return w.Show(ctx, w.QuitConfirmMenu(ctx, *paneF, *clientF), *clientF, *paneF)
	}
	if !*yes {
		in, _ := e.Stdin.(*os.File)
		if in == nil || !isTerminal(in) {
			fmt.Fprintln(e.Stdout, i18n.T("quit.no_terminal"))
			return nil
		}
		fmt.Fprint(e.Stdout, i18n.T("quit.ask", w.AgentCount(ctx)))
		line, _ := bufio.NewReader(in).ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes", "e", "evet":
		default:
			fmt.Fprintln(e.Stdout, i18n.T("font.skipped"))
			return nil
		}
	}
	err = w.Quit(ctx, func() {
		for _, i := range webview.All(ctx) {
			i.Stop()
		}
		watch.Stop()
	})
	if err == nil && !*yes {
		fmt.Fprintln(e.Stdout, i18n.T("quit.done"))
	}
	return err
}
