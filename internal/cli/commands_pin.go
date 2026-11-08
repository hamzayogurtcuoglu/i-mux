package cli

import (
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
)

func init() {
	register(&command{
		Name:   "__pin",
		Args:   "<agent:id> [--unpin] [--pane P] [--client C]",
		Hidden: true,
		Run:    runPin,
	})
}

// runPin pins a conversation in History (or unpins it) and, from a menu,
// opens the list again (§9.3 "Pinned conversations").
func runPin(e *Env, args []string) error {
	fs := newFlags(e, "__pin")
	unpin := fs.Bool("unpin", false, "unpin instead")
	paneF := fs.String("pane", "", "reopen the list for this pane")
	clientF := fs.String("client", "", "on this client")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return &UsageError{Msg: i18n.T("cli.usage", "__pin <agent:id> [--unpin]")}
	}
	key := pos[0]
	if a, id, ok := strings.Cut(key, ":"); !ok || id == "" || !session.Known(a) {
		return &UsageError{Msg: i18n.T("cli.usage", "__pin <agent:id> [--unpin]")}
	}
	if err := state.Update(func(s *state.State) error {
		if *unpin {
			s.Unpin(key)
		} else {
			s.Pin(key)
		}
		return nil
	}); err != nil {
		return err
	}
	if *paneF == "" {
		return nil
	}
	menu := []string{"--sessions", "--pane", *paneF}
	if *clientF != "" {
		menu = append(menu, "--client", *clientF)
	}
	return runMenu(e, menu)
}
