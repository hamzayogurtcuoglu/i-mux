package popmenu

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
)

const (
	reset = "\x1b[0m"
	bold  = "\x1b[1m"
	faint = "\x1b[38;5;244m"
	sepFg = "\x1b[38;5;240m"
	selBg = "\x1b[48;5;24m\x1b[38;5;255m"
	xMark = "✕" // closes the menu
)

// View is a menu on the popup's screen: what is selected and scrolled to.
type View struct {
	m   *Menu
	sel int // the selected item; -1 for none
	top int // the first item shown
	w   int // the screen's width and height at the last Render
	h   int
}

// NewView shows m with nothing selected, as tmux's menus open.
func NewView(m *Menu) *View { return &View{m: m, sel: -1} }

// Selected is the selected item (-1 for none).
func (v *View) Selected() int { return v.sel }

func (v *View) ok(i int) bool {
	return i >= 0 && i < len(v.m.Items) && !v.m.Items[i].Sep && !v.m.Items[i].Disabled
}

// rows is how many item rows fit below the title row.
func (v *View) rows() int { return max(v.h-1, 1) }

func (v *View) keepVisible() {
	if v.sel < 0 {
		return
	}
	if v.sel < v.top {
		v.top = v.sel
	}
	if v.sel >= v.top+v.rows() {
		v.top = v.sel - v.rows() + 1
	}
}

// keyLabel is an item's key as shown ("" for none or a disabled item, as
// tmux shows them).
func keyLabel(it Item) string {
	if it.Disabled || it.Sep {
		return ""
	}
	return it.Key
}

// Render draws the menu on a w×h screen: the title and ✕, then the items.
func (v *View) Render(w, h int) []string {
	v.w, v.h = w, h
	v.keepVisible()
	title := " " + sidebar.Fit(v.m.Title, max(w-3, 1))
	lines := []string{bold + title + reset + faint + xMark + reset + " "}
	for i := v.top; i < len(v.m.Items) && len(lines) < h; i++ {
		it := v.m.Items[i]
		if it.Sep {
			lines = append(lines, " "+sepFg+strings.Repeat("─", max(w-2, 1))+reset+" ")
			continue
		}
		key := keyLabel(it)
		kw := 0
		if key != "" {
			kw = sidebar.DisplayWidth(key) + 2
		}
		name := "  " + sidebar.Fit(it.Name, max(w-3-kw, 1))
		switch {
		case i == v.sel:
			if key != "" {
				name += "  " + key
			}
			lines = append(lines, selBg+name+" "+reset)
		case it.Disabled:
			lines = append(lines, faint+name+" "+reset)
		case key != "":
			lines = append(lines, name+"  "+faint+key+reset+" ")
		default:
			lines = append(lines, name+" ")
		}
	}
	return lines
}

// at is the item on screen row y (1-based), -1 for none.
func (v *View) at(y int) int {
	i := v.top + y - 2
	if y < 2 || i >= len(v.m.Items) {
		return -1
	}
	return i
}

// move steps the selection by d over the items that can be chosen.
func (v *View) move(d int) {
	n := len(v.m.Items)
	i := v.sel
	if i < 0 && d < 0 {
		i = n
	}
	for k := 0; k < n; k++ {
		i += d
		if i < 0 || i >= n {
			return
		}
		if v.ok(i) {
			v.sel = i
			return
		}
	}
}

// Result is what an event did: Chosen is the item chosen (-1 for none),
// Done that the menu closes.
type Result struct {
	Chosen int
	Done   bool
}

var none = Result{Chosen: -1}

func chosen(i int) Result { return Result{Chosen: i, Done: true} }

// matches reports whether the typed key (sidebar.Parse's name) is an item's
// tmux key.
func matches(tmuxKey, typed string) bool {
	if tmuxKey == "" {
		return false
	}
	if len([]rune(tmuxKey)) == 1 {
		return tmuxKey == typed
	}
	switch tmuxKey {
	case "Enter":
		return typed == "enter"
	case "Space":
		return typed == " "
	case "Tab":
		return typed == "tab"
	case "BSpace":
		return typed == "backspace"
	}
	if strings.HasPrefix(tmuxKey, "C-") && len(tmuxKey) == 3 {
		return typed == "ctrl-"+strings.ToLower(tmuxKey[2:])
	}
	return strings.EqualFold(tmuxKey, typed)
}

// Key handles a key: an item's own key first, as tmux's menus do, then the
// arrows (and k, j), Enter, and the keys that close.
func (v *View) Key(k string) Result {
	for i, it := range v.m.Items {
		if v.ok(i) && matches(it.Key, k) {
			return chosen(i)
		}
	}
	switch k {
	case "up", "k":
		v.move(-1)
	case "down", "j", "tab":
		v.move(1)
	case "backtab":
		v.move(-1)
	case "home", "pgup":
		v.sel = -1
		v.move(1)
	case "end", "pgdown":
		v.sel = -1
		v.move(-1)
	case "enter":
		if v.ok(v.sel) {
			return chosen(v.sel)
		}
		return Result{Chosen: -1, Done: true}
	case "esc", "q", "ctrl-c", "\x07":
		return Result{Chosen: -1, Done: true}
	}
	return none
}

// Click handles a press at (x, y): an item chooses it, ✕ closes.
func (v *View) Click(x, y int) Result {
	if y == 1 {
		if x >= v.w-2 {
			return Result{Chosen: -1, Done: true}
		}
		return none
	}
	if i := v.at(y); v.ok(i) {
		return chosen(i)
	}
	return none
}

// Move selects the item under the pointer.
func (v *View) Move(_, y int) {
	if i := v.at(y); v.ok(i) {
		v.sel = i
	}
}

// Scroll moves the selection with the wheel.
func (v *View) Scroll(up bool) {
	if up {
		v.move(-1)
	} else {
		v.move(1)
	}
}

// Run shows the menu on the popup's terminal until an item is chosen (its
// index) or it is closed (-1).
func Run(m *Menu, in, out *os.File) (int, error) {
	fd := int(in.Fd())
	restore, err := sidebar.RawMode(fd)
	if err != nil {
		return -1, err
	}
	defer restore()
	// No cursor; presses, the pointer's motion (the hover) and the wheel.
	_, _ = io.WriteString(out, "\x1b[?25l\x1b[?1000h\x1b[?1003h\x1b[?1006h")
	defer func() { _, _ = io.WriteString(out, "\x1b[?1003l\x1b[?1000l\x1b[?1006l\x1b[?25h") }()
	winch := make(chan os.Signal, 1)
	sysx.Notify(winch, sysx.SIGWINCH)
	defer sysx.Stop(winch)
	v := NewView(m)
	last := ""
	draw := func() {
		w, h := sidebar.Size(int(out.Fd()))
		var sc sidebar.Screen
		for _, l := range v.Render(w, h) {
			sc.Lines = append(sc.Lines, sidebar.Line{Text: l})
		}
		if f := sidebar.Frame(sc, w, h); f != last {
			_, _ = io.WriteString(out, f)
			last = f
		}
	}
	draw()
	var pending []byte
	for {
		b, err := sidebar.ReadInput(fd, 50*time.Millisecond)
		if err != nil {
			return -1, nil // the popup went away
		}
		select {
		case <-winch:
			last = ""
		default:
		}
		var evs []sidebar.Event
		evs, pending = sidebar.Parse(append(pending, b...), true)
		for _, ev := range evs {
			r := none
			switch ev.Kind {
			case sidebar.EvKey:
				r = v.Key(ev.Key)
			case sidebar.EvClick:
				if !sidebar.EarlyClick() {
					r = v.Click(ev.X, ev.Y)
				}
			case sidebar.EvMove:
				v.Move(ev.X, ev.Y)
			case sidebar.EvScroll:
				v.Scroll(ev.Key == "up")
			}
			if r.Done {
				return r.Chosen, nil
			}
		}
		draw()
	}
}
