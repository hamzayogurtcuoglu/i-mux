package workspace

import (
	"context"
	"fmt"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/quickpick"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// KeyName is a tmux key as the help writes it: "C-\" is "Ctrl+\", "C-p"
// "Ctrl+P", "M-v" "Alt+V", "C-M-Left" "Ctrl+Alt+←".
func KeyName(k string) string {
	var mods []string
	for {
		switch {
		case strings.HasPrefix(k, "C-") && len(k) > 2:
			mods, k = append(mods, "Ctrl"), k[2:]
			continue
		case strings.HasPrefix(k, "M-") && len(k) > 2:
			mods, k = append(mods, "Alt"), k[2:]
			continue
		case strings.HasPrefix(k, "S-") && len(k) > 2:
			mods, k = append(mods, "Shift"), k[2:]
			continue
		}
		break
	}
	switch k {
	case "Left":
		k = "←"
	case "Right":
		k = "→"
	case "Up":
		k = "↑"
	case "Down":
		k = "↓"
	case "Space":
		k = "Space"
	default:
		if len(mods) > 0 && len(k) == 1 {
			k = strings.ToUpper(k)
		}
	}
	return strings.Join(append(mods, k), "+")
}

// prefixed is a prefix key as the help writes it: "prefix v".
func prefixed(k string) string { return "prefix " + KeyName(k) }

// keysOr joins the keys bound ("" ones are not), "" when none is.
func keysOr(keys ...string) string {
	var out []string
	for _, k := range keys {
		if k != "" {
			out = append(out, k)
		}
	}
	return strings.Join(out, "  ·  ")
}

// KeysPick is the help (D67): the actions with the keys the config binds,
// in groups, as a Quick Pick that filters them as one types and runs the
// chosen one for pane p on client. prefix is tmux's prefix key ("C-b").
func (w *WS) KeysPick(p *Pane, client, prefix string) *quickpick.Pick {
	t := w.Cfg.Tmux
	tgt := uiTarget(p.ID, client)
	argv := func(args ...string) []string { return append(args, tgt...) }
	pick := &quickpick.Pick{
		Title:       i18n.T("help.title", KeyName(prefix)),
		Placeholder: i18n.T("help.placeholder"),
		Empty:       i18n.T("help.empty"),
		Client:      client,
		Pane:        p.ID,
	}
	group := ""
	add := func(label, keys string, args []string) {
		if keys == "" {
			return // not bound
		}
		pick.Items = append(pick.Items, quickpick.Item{Label: label, Description: keys, Group: group, Argv: args})
	}
	menuKeys := keysOr(KeyName(t.MenuKeyRoot), prefixedOr(t.MenuKeyPrefix))

	group = i18n.T("help.group_agents")
	add(i18n.T("help.menu"), menuKeys, argv("__menu"))
	if menuKeys != "" {
		first := t.MenuKeyRoot
		if first == "" {
			first = "prefix " + t.MenuKeyPrefix
		}
		for _, a := range agent.Ordered(w.Cfg) {
			add(i18n.T("help.continue", a.Short), KeyName(first)+" "+i18n.T("help.then")+" "+a.Key, argv("switch", string(a.ID)))
		}
	}
	add(i18n.T("help.sessions"), prefixedOr(t.PickerKeyPrefix), argv("__menu", "--sessions"))
	add(i18n.T("help.goto"), KeyName(t.QuickOpenKeyRoot), argv("__goto"))
	if w.Cfg.Paste.Images {
		add(i18n.T("help.paste"), "Ctrl+V  ·  Alt+V", nil)
	}

	group = i18n.T("help.group_panes")
	add(i18n.T("help.split_right"), prefixedOr(t.SplitRightKey), argv("__menu", "--split", "h"))
	add(i18n.T("help.split_down"), prefixedOr(t.SplitDownKey), argv("__menu", "--split", "v"))
	move := "prefix ←↑→↓"
	if t.DirectPaneKeys {
		move = keysOr(move, "Ctrl+←↑→↓")
		add(i18n.T("help.move"), move, nil)
		add(i18n.T("help.split_dir"), "Ctrl+Alt+←↑→↓", nil)
	} else {
		add(i18n.T("help.move"), move, nil)
	}
	if t.SwapPaneKeys {
		for _, sw := range tmux.SwapKeys {
			add(i18n.T(swapLabels[sw.Dir]), prefixed(sw.Key), argv("__pane", "swap", sw.Dir))
		}
	}
	add(i18n.T("help.resize"), prefixedOr(t.ResizeKey), argv("__pane", "resize"))
	add(i18n.T("help.zoom"), "prefix Z", nil)

	if t.Tabs {
		group = i18n.T("help.group_tabs")
		add(i18n.T("help.new_tab"), prefixedOr(t.NewTabKey), argv("__menu", "--tab"))
		add(i18n.T("help.name_tab"), prefixedOr(t.RenameTabKey), nil)
		add(i18n.T("help.close_tab"), "×", nil)
	}

	group = i18n.T("help.group_sidebar")
	add(i18n.T("help.sidebar"), prefixedOr(t.SidebarKey), argv("sidebar", "toggle"))
	add(i18n.T("help.views"), "1  2  3  4  5", nil)
	add(i18n.T("help.search"), "3  ·  /", nil)
	add(i18n.T("help.rename"), i18n.T("help.right_click"), nil)
	add(i18n.T("help.help"), keysOr(prefixedOr(t.HelpKey), "?"), nil)
	add(i18n.T("help.prefix"), i18n.T("help.prefix_keys", KeyName(prefix)), nil)
	return pick
}

// swapLabels are the help's rows of prefix H/J/K/L.
var swapLabels = map[string]string{
	DirLeft: "help.swap_left", DirDown: "help.swap_down", DirUp: "help.swap_up", DirRight: "help.swap_right",
}

// prefixedOr is prefixed(k), "" for no key.
func prefixedOr(k string) string {
	if k == "" {
		return ""
	}
	return prefixed(k)
}

// ShowKeys opens the help on client (prefix ?, the sidebar's ?, the rail's
// Help).
func (w *WS) ShowKeys(ctx context.Context, p *Pane, client string) error {
	prefix := "C-b"
	if out, err := w.T.Run(ctx, tmux.Cmd{"show-options", "-gv", "prefix"}); err == nil && strings.TrimSpace(out) != "" {
		prefix = strings.TrimSpace(out)
	}
	cw, ch := 0, 0
	if client != "" {
		if out, err := w.T.Run(ctx, tmux.Cmd{"display-message", "-p", "-c", client, "#{client_width}\t#{client_height}"}); err == nil {
			_, _ = fmt.Sscanf(strings.TrimSpace(out), "%d\t%d", &cw, &ch)
		}
	}
	return quickpick.Show(ctx, w.T, w.Self, w.KeysPick(p, client, prefix), cw, ch)
}

// ResizeMode puts client in the resize mode (the help's Resize panes; the
// key is tmux.ResizeMode).
func (w *WS) ResizeMode(ctx context.Context, client string) error {
	sw := tmux.Cmd{"switch-client", "-T", tmux.ResizeTable}
	msg := tmux.Cmd{"display-message", "-d", "0", tmux.EscapeMessage(i18n.T("resize.mode"))}
	if client != "" {
		sw = tmux.Cmd{"switch-client", "-c", client, "-T", tmux.ResizeTable}
		msg = tmux.Cmd{"display-message", "-c", client, "-d", "0", tmux.EscapeMessage(i18n.T("resize.mode"))}
	}
	_, err := w.T.Run(ctx, sw, msg)
	return err
}
