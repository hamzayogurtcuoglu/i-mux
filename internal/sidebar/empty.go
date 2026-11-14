package sidebar

import (
	"context"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// ShellItem is the "Terminal" choice of the "No agents" pane (imux split's
// name for a plain shell).
const ShellItem = "shell"

// EmptyOptions configure the "No agents" pane (§9.3 "A project without
// agents").
type EmptyOptions struct {
	T       *tmux.Client
	Self    string     // the imux binary
	Pane    string     // this pane ($TMUX_PANE)
	Project string     // the project folder's name
	Agents  []AgentRow // in config order, with Installed
	In      *os.File
	Out     io.Writer
}

// emptyItem is one thing the pane can start.
type emptyItem struct {
	key, name, id string
	ok            bool // installed
}

// EmptyScreen is the drawn pane: its rows and, per row, the item a click on
// it starts (-1 for none).
type EmptyScreen struct {
	Lines []string
	Item  []int
	items []emptyItem
}

// emptyItems are the agents and a terminal.
func emptyItems(agents []AgentRow) []emptyItem {
	var out []emptyItem
	for _, a := range agents {
		out = append(out, emptyItem{key: a.Key, name: a.Name, id: string(a.ID), ok: a.Installed})
	}
	return append(out, emptyItem{key: "t", name: i18n.T("menu.terminal"), id: ShellItem, ok: true})
}

// RenderEmpty draws the "No agents" pane for w×h: the project, the agents
// to start (sel highlighted, -1 for none), and where else to start one; a
// starting item replaces the last line.
func RenderEmpty(project string, agents []AgentRow, w, h, sel int, starting string) EmptyScreen {
	items := emptyItems(agents)
	type row struct {
		text string
		item int
	}
	rows := []row{{bold + i18n.T("empty.title", project) + reset, -1}, {"", -1}, {muted + i18n.T("empty.start") + reset, -1}}
	nameW := 0
	for _, it := range items {
		nameW = max(nameW, DisplayWidth(it.name))
	}
	for i, it := range items {
		text := "  " + cyan + it.key + reset + "  " + fit(it.name, nameW)
		if !it.ok {
			text = "  " + muted + it.key + "  " + fit(it.name, nameW) + " " + i18n.T("menu.not_installed") + reset
		}
		if i == sel {
			text = onBg(text+"  ", selBg)
		}
		rows = append(rows, row{text, i})
	}
	last := muted + i18n.T("empty.or_sidebar") + reset
	if starting != "" {
		last = cyan + i18n.T("empty.starting", starting) + reset
	}
	rows = append(rows, row{"", -1}, row{last, -1})

	// The block centered in the pane.
	bw := 0
	for _, r := range rows {
		bw = max(bw, DisplayWidth(Plain(r.text)))
	}
	left := max((w-bw)/2, 0)
	top := max((h-len(rows))/2, 0)
	sc := EmptyScreen{items: items}
	for i := 0; i < top; i++ {
		sc.Lines, sc.Item = append(sc.Lines, ""), append(sc.Item, -1)
	}
	for _, r := range rows {
		sc.Lines = append(sc.Lines, clip(strings.Repeat(" ", left)+r.text, w))
		sc.Item = append(sc.Item, r.item)
	}
	return sc
}

// pick is the item of key k ("" for none): an installed one.
func (sc EmptyScreen) pick(k string) int {
	for i, it := range sc.items {
		if it.key == k && it.ok {
			return i
		}
	}
	return -1
}

// move steps the selection by d over installed items.
func (sc EmptyScreen) move(sel, d int) int {
	n := len(sc.items)
	if sel < 0 { // nothing selected: down starts at the first, up at the last
		sel = 0
		if d > 0 {
			sel = n - 1
		}
	}
	for i := 1; i <= n; i++ {
		j := ((sel+d*i)%n + n) % n
		if sc.items[j].ok {
			return j
		}
	}
	return -1
}

// RunEmpty draws the "No agents" pane until it is replaced: a key (the
// item's letter, ↑↓ and Enter) or a click starts an agent or a terminal in
// this pane through `imux split`, run by tmux (respawning this pane ends
// this process).
func RunEmpty(ctx context.Context, o EmptyOptions) error {
	restore, err := rawMode(int(o.In.Fd()))
	if err == nil {
		defer restore()
	}
	_, _ = io.WriteString(o.Out, "\x1b[?1049h\x1b[?25l\x1b[?1000h\x1b[?1006h")
	defer func() { _, _ = io.WriteString(o.Out, "\x1b[?1000l\x1b[?1006l\x1b[?25h\x1b[?1049l") }()

	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGWINCH, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT)
	defer signal.Stop(sigs)
	input := make(chan []byte, 8)
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := o.In.Read(buf)
			if n > 0 {
				input <- append([]byte(nil), buf[:n]...)
			}
			if err != nil {
				close(input)
				return
			}
		}
	}()

	sel, starting := -1, ""
	var sc EmptyScreen
	draw := func() {
		w, h := Size(int(o.In.Fd()))
		sc = RenderEmpty(o.Project, o.Agents, w, h, sel, starting)
		// On the theme's background (D68), each row painted.
		rows := make([]string, len(sc.Lines))
		for i, l := range sc.Lines {
			rows[i] = paint(l) + "\x1b[K"
		}
		_, _ = io.WriteString(o.Out, paneBase+"\x1b[H\x1b[2J"+strings.Join(rows, "\r\n")+reset)
	}
	start := func(i int) {
		if i < 0 || i >= len(sc.items) || !sc.items[i].ok || starting != "" {
			return
		}
		it := sc.items[i]
		sel, starting = i, it.name
		draw()
		_, _ = o.T.Run(ctx, tmux.Cmd{"run-shell", "-b", tmux.UICommand(o.Self, "split", it.id, "-h", "--pane", o.Pane)})
	}
	draw()
	var pending []byte
	for {
		select {
		case <-ctx.Done():
			return nil
		case sig := <-sigs:
			if sig != syscall.SIGWINCH {
				return nil
			}
			draw()
		case b, ok := <-input:
			if !ok {
				return nil
			}
			var evs []Event
			evs, pending = Parse(append(pending, b...), true)
			for _, ev := range evs {
				switch {
				case ev.Kind == EvClick && ev.Y >= 1 && ev.Y <= len(sc.Item):
					start(sc.Item[ev.Y-1])
				case ev.Kind == EvKey && (ev.Key == "down" || ev.Key == "tab"):
					sel = sc.move(sel, 1)
					draw()
				case ev.Kind == EvKey && (ev.Key == "up" || ev.Key == "backtab"):
					sel = sc.move(sel, -1)
					draw()
				case ev.Kind == EvKey && ev.Key == "enter":
					if sel < 0 {
						sel = sc.move(-1, 1)
					}
					start(sel)
				case ev.Kind == EvKey:
					start(sc.pick(ev.Key))
				}
			}
		}
	}
}
