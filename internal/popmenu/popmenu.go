// Package popmenu shows i-mux's menus in a popup where tmux's own menus take
// no mouse (tmux before 3.5: a click closed them without choosing). The popup
// is drawn by `imux __menu-popup`, which gets the mouse there (D54, §9.3
// "Menus on tmux before 3.5").
package popmenu

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Item is one row of a menu.
type Item struct {
	Name     string `json:"name,omitempty"` // as shown
	Key      string `json:"key,omitempty"`  // tmux key name ("c", "Enter"); "" for none
	Cmd      string `json:"cmd,omitempty"`  // tmux command, as display-menu got it
	Disabled bool   `json:"disabled,omitempty"`
	Sep      bool   `json:"sep,omitempty"` // a separator line
}

// Menu is a display-menu: what the popup shows and where its choice runs.
type Menu struct {
	Title  string `json:"title,omitempty"`
	Items  []Item `json:"items"`
	Client string `json:"client,omitempty"` // -c
	Pane   string `json:"pane,omitempty"`   // -t
	X      string `json:"x,omitempty"`      // -x
	Y      string `json:"y,omitempty"`      // -y
}

// unescape undoes tmux.EscapeFormat: i-mux's menu names are plain text.
func unescape(s string) string { return strings.ReplaceAll(s, "##", "#") }

// Parse reads a display-menu command as tmux.Client.Menu and i-mux's menus
// build it: flags, "--", then name, key and command for each item, a lone ""
// for a separator.
func Parse(cmd tmux.Cmd) (*Menu, error) {
	if len(cmd) == 0 || cmd[0] != "display-menu" {
		return nil, errors.New("not a display-menu")
	}
	m := &Menu{X: "C", Y: "C"}
	i := 1
flags:
	for i < len(cmd) {
		a := cmd[i]
		switch a {
		case "--":
			i++
			break flags
		case "-M", "-O":
			i++
		case "-c", "-t", "-T", "-x", "-y", "-b", "-C", "-H", "-s", "-S":
			if i+1 >= len(cmd) {
				return nil, errors.New("display-menu: " + a + " needs a value")
			}
			v := cmd[i+1]
			switch a {
			case "-c":
				m.Client = v
			case "-t":
				m.Pane = v
			case "-T":
				m.Title = unescape(v)
			case "-x":
				m.X = v
			case "-y":
				m.Y = v
			}
			i += 2
		default:
			if strings.HasPrefix(a, "-") && len(a) > 1 {
				return nil, errors.New("display-menu: unknown flag " + a)
			}
			break flags
		}
	}
	rest := cmd[i:]
	for len(rest) > 0 {
		if rest[0] == "" {
			m.Items = append(m.Items, Item{Sep: true})
			rest = rest[1:]
			continue
		}
		if len(rest) < 3 {
			return nil, errors.New("display-menu: an item without its key and command")
		}
		it := Item{Name: unescape(rest[0]), Key: rest[1], Cmd: rest[2]}
		if strings.HasPrefix(it.Name, "-") {
			it.Name, it.Disabled = it.Name[1:], true
		}
		m.Items = append(m.Items, it)
		rest = rest[3:]
	}
	return m, nil
}

// Size is the popup's size for m: the longest row and every item, with the
// title row and tmux's border (tmux keeps a popup within the client).
func Size(m *Menu) (w, h int) {
	inner := sidebar.DisplayWidth(m.Title) + 5 // " title  ✕ "
	for _, it := range m.Items {
		if it.Sep {
			continue
		}
		n := 2 + sidebar.DisplayWidth(it.Name) + 1
		if k := keyLabel(it); k != "" {
			n += 2 + sidebar.DisplayWidth(k)
		}
		inner = max(inner, n)
	}
	return max(inner, 16) + 2, len(m.Items) + 1 + 2
}

// Show is tmux.MenuPopup for the imux binary self: the menu is written to
// paths.Menus()/<id>.json, then one tmux client shows the popup and, once it
// has closed, sources <id>.tmux, which the popup writes with the chosen
// command (nothing when it was closed). That client starts from the menu's
// pane (TMUX_PANE), as display-menu runs a choice there.
func Show(self string) func(ctx context.Context, c *tmux.Client, cmd tmux.Cmd) error {
	return func(ctx context.Context, c *tmux.Client, cmd tmux.Cmd) error {
		m, err := Parse(cmd)
		if err != nil {
			return c.Spawn(cmd) // tmux's menu, keys only
		}
		dir := paths.Menus()
		if err := paths.EnsureDir(dir); err != nil {
			return c.Spawn(cmd)
		}
		prune(dir)
		id, err := newID()
		if err != nil {
			return c.Spawn(cmd)
		}
		b, err := json.Marshal(m)
		if err != nil {
			return c.Spawn(cmd)
		}
		if err := os.WriteFile(filepath.Join(dir, id+".json"), b, 0o600); err != nil {
			return c.Spawn(cmd)
		}
		out := filepath.Join(dir, id+".tmux")
		w, h := Size(m)
		return Popup(ctx, c, m.Client, m.Pane, w, h, m.X, m.Y, tmux.ShJoin(self, "__menu-popup", id),
			tmux.Cmd{"source-file", "-q", out},
			tmux.Cmd{"run-shell", "-b", tmux.EscapeFormat("rm -f " + tmux.ShQuote(out))})
	}
}

// Popup shows program (a shell command) in a w×h popup at x, y on client,
// from pane, and does not wait for it; after runs once the popup closed (a
// popup cannot open over another). The tmux client that shows it starts
// from pane (TMUX_PANE), as display-menu runs a choice there.
func Popup(ctx context.Context, c *tmux.Client, client, pane string, w, h int, x, y, program string, after ...tmux.Cmd) error {
	pop := tmux.Cmd{"display-popup"}
	if client != "" {
		pop = append(pop, "-c", client)
	}
	if pane != "" {
		pop = append(pop, "-t", pane)
	}
	pop = append(pop, "-E", "-w", strconv.Itoa(w), "-h", strconv.Itoa(h), "-x", x, "-y", y, program)
	var cmds []tmux.Cmd
	if c.MessageTakesClient(ctx) {
		// A status-line message freezes the client's terminal, and tmux
		// 3.3 and 3.4 then draw none of the popup's output until it goes:
		// one shown with -d 0 ("✨ Writing…") until a key. An empty
		// message that lasts 1 ms replaces it. (3.2 draws a popup through
		// the freeze, which its popups need.)
		clear := tmux.Cmd{"display-message", "-d", "1"}
		if client != "" {
			clear = append(clear, "-c", client)
		}
		cmds = append(cmds, append(clear, ""))
	}
	var env []string
	if pane != "" {
		env = []string{"TMUX_PANE=" + pane}
	}
	return c.SpawnEnv(env, append(append(cmds, pop), after...)...)
}

// NewID is a random file name for a popup's data under paths.Menus(); it
// also removes the files older ones left.
func NewID() (string, error) {
	if err := paths.EnsureDir(paths.Menus()); err != nil {
		return "", err
	}
	prune(paths.Menus())
	return newID()
}

// newID is a random file name.
func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ValidID reports whether id is one newID made (the popup reads only those).
func ValidID(id string) bool {
	if len(id) != 16 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// Files are the menu's file and the file of its choice.
func Files(id string) (menu, choice string) {
	return filepath.Join(paths.Menus(), id+".json"), filepath.Join(paths.Menus(), id+".tmux")
}

// pruneAfter is how old a left-over file is (a popup that never started, a
// choice whose command failed) before it goes.
const pruneAfter = 10 * time.Minute

func prune(dir string) {
	es, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range es {
		if fi, err := e.Info(); err == nil && time.Since(fi.ModTime()) > pruneAfter {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
