package cli

import (
	"context"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

func init() {
	register(&command{
		Name:   "__name-session",
		Args:   "<agent:id> [name...] | --from-option",
		Hidden: true,
		Run:    runNameSession,
	})
}

// runNameSession names a conversation (History → Rename…; §9.3 "Named
// conversations"): an empty name removes it. The open agent running it
// takes the name too; from the prompt, History opens again.
func runNameSession(e *Env, args []string) error {
	fs := newFlags(e, "__name-session")
	fromOpt := fs.Bool("from-option", false, "take the conversation, pane, client and name from the prompt's options")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	usage := &UsageError{Msg: i18n.T("cli.usage", "__name-session <agent:id> [name...]")}
	w, _, err := newWorkspace(e, false)
	if err != nil {
		return err
	}
	ctx := context.Background()
	var key, name, pane, client string
	if *fromOpt {
		out, _ := w.T.Run(ctx,
			tmux.Cmd{"show-options", "-gqv", workspace.OptNameKey},
			tmux.Cmd{"show-options", "-gqv", workspace.OptNamePane},
			tmux.Cmd{"show-options", "-gqv", workspace.OptNameClient},
			tmux.Cmd{"show-options", "-gqv", workspace.OptNameValue})
		_, _ = w.T.Run(ctx,
			tmux.Cmd{"set-option", "-gu", workspace.OptNameKey},
			tmux.Cmd{"set-option", "-gu", workspace.OptNamePane},
			tmux.Cmd{"set-option", "-gu", workspace.OptNameClient},
			tmux.Cmd{"set-option", "-gu", workspace.OptNameValue})
		lines := strings.SplitN(out, "\n", 5)
		for len(lines) < 4 {
			lines = append(lines, "")
		}
		key, pane, client, name = strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1]), strings.TrimSpace(lines[2]), strings.TrimSpace(lines[3])
	} else {
		if len(pos) < 1 {
			return usage
		}
		key, name = pos[0], strings.TrimSpace(strings.Join(pos[1:], " "))
	}
	if a, id, ok := strings.Cut(key, ":"); !ok || id == "" || !session.Known(a) {
		return usage
	}
	if err := w.NameConversation(ctx, sources.New(w.Cfg), key, name); err != nil {
		return err
	}
	if pane == "" {
		return nil
	}
	msg := i18n.T("picker.named", name)
	if name == "" {
		msg = i18n.T("picker.unnamed")
	}
	w.Message(ctx, client, pane, msg)
	menu := []string{"--sessions", "--pane", pane}
	if client != "" {
		menu = append(menu, "--client", client)
	}
	return runMenu(e, menu)
}
