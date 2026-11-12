// Package quickpick is VS Code's Quick Pick in a popup (§9.3 "Source Control
// like VS Code", D58): a title, an input that filters the items as one
// types, the items in groups with a description each. The chosen item runs
// imux with its argv: in the popup, where its output shows, or once the
// popup has closed (a popup, a menu or a prompt cannot open over it).
package quickpick

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/popmenu"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Item is one row to choose.
type Item struct {
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"` // dim, after the label
	Group       string   `json:"group,omitempty"`       // a new group starts at a new name
	Argv        []string `json:"argv,omitempty"`        // imux's arguments, run when chosen
	InPopup     bool     `json:"in_popup,omitempty"`    // run in the popup: its output shows
	Always      bool     `json:"always,omitempty"`      // listed whatever is typed
	Query       bool     `json:"query,omitempty"`       // what was typed goes along: --initial T
}

// Pick is a Quick Pick: its title, the input's placeholder, its items, and
// where it shows.
type Pick struct {
	Title       string `json:"title"`
	Placeholder string `json:"placeholder,omitempty"`
	Empty       string `json:"empty,omitempty"` // shown when nothing matches
	Items       []Item `json:"items"`
	Client      string `json:"client,omitempty"`
	Pane        string `json:"pane,omitempty"`
}

// Size is the popup's size for p on a client of cw×ch cells (0: unknown):
// as wide as its longest row, as tall as its rows, within the client.
func Size(p *Pick, cw, ch int) (w, h int) {
	w = max(sidebar.DisplayWidth(p.Title)+6, 48)
	rows, group := 0, ""
	for _, it := range p.Items {
		if it.Group != group && it.Group != "" {
			rows++
		}
		group = it.Group
		rows++
		lw := sidebar.DisplayWidth(it.Label) + 6
		if it.Description != "" {
			lw += sidebar.DisplayWidth(it.Description) + 2
		}
		w = max(w, lw)
	}
	h = max(rows, 1) + 3 + 2 // title, input, line; the border
	if cw <= 0 {
		cw = 80
	}
	if ch <= 0 {
		ch = 24
	}
	w = min(w, 100, max(cw-4, 20))
	h = min(max(h, 8), max(ch*3/4, 8), ch)
	return w, h
}

// Show writes p for the popup and shows it on p.Client, without waiting.
func Show(ctx context.Context, c *tmux.Client, self string, p *Pick, cw, ch int) error {
	id, err := popmenu.NewID()
	if err != nil {
		return err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if err := os.WriteFile(file(id), b, 0o600); err != nil {
		return err
	}
	w, h := Size(p, cw, ch)
	return popmenu.Popup(ctx, c, p.Client, p.Pane, w, h, "C", "C", tmux.ShJoin(self, "__quickpick", id),
		tmux.Cmd{"run-shell", "-b", tmux.EscapeFormat(tmux.ShJoin(self, "__quickpick", "--run", id))})
}

func file(id string) string { return filepath.Join(paths.Menus(), id+".pick.json") }

func laterFile(id string) string { return filepath.Join(paths.Menus(), id+".run.json") }

// Load reads (and removes) the Quick Pick Show wrote as id.
func Load(id string) (*Pick, error) {
	if !popmenu.ValidID(id) {
		return nil, errors.New("quickpick: bad id")
	}
	b, err := os.ReadFile(file(id))
	_ = os.Remove(file(id))
	if err != nil {
		return nil, err
	}
	var p Pick
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Argv is the chosen item's argv with what was typed (an item with Query:
// --initial before its data).
func Argv(it Item, query string) []string {
	a := append([]string{}, it.Argv...)
	if !it.Query || query == "" {
		return a
	}
	i := len(a)
	for j, s := range a {
		if s == "--" {
			i = j
			break
		}
	}
	return append(append(a[:i:i], "--initial", query), a[i:]...)
}

// Exec runs imux self with argv on the popup's terminal (in, out).
func Exec(self string, argv []string, in, out *os.File) error {
	if len(argv) == 0 {
		return nil
	}
	cmd := exec.Command(self, argv...)
	if in != nil && out != nil {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = in, out, out
	}
	return cmd.Run()
}

// Later keeps argv for RunLater, which runs once the popup id has closed.
func Later(id string, argv []string) error {
	if !popmenu.ValidID(id) {
		return errors.New("quickpick: bad id")
	}
	b, err := json.Marshal(argv)
	if err != nil {
		return err
	}
	return os.WriteFile(laterFile(id), b, 0o600)
}

// RunLater runs imux self with what Later kept for the popup id (nothing
// when it closed without a choice, or ran its choice itself).
func RunLater(self, id string) error {
	if !popmenu.ValidID(id) {
		return errors.New("quickpick: bad id")
	}
	b, err := os.ReadFile(laterFile(id))
	_ = os.Remove(laterFile(id))
	if err != nil {
		return nil
	}
	var argv []string
	if err := json.Unmarshal(b, &argv); err != nil || len(argv) == 0 {
		return err
	}
	return exec.Command(self, argv...).Run()
}
