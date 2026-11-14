package workspace

import (
	"context"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// PaneBox is a pane's place in its window: its cells, inclusive, and its
// role (a sidebar's is sidebar.RoleSidebar).
type PaneBox struct {
	ID                       string
	Left, Top, Right, Bottom int
	Role                     string
}

// Directions of the pane keys (D67).
const (
	DirLeft  = "left"
	DirDown  = "down"
	DirUp    = "up"
	DirRight = "right"
)

// ValidDir reports one of the four directions.
func ValidDir(d string) bool {
	return d == DirLeft || d == DirDown || d == DirUp || d == DirRight
}

// Neighbor is the pane next to cur on side dir: the one across the border
// on that side sharing the most rows (or columns) with it, the upper (or
// left) one of two that share as many. "" past the window's edge, where
// tmux's {left-of} would go around to the other side.
func Neighbor(panes []PaneBox, cur, dir string) string {
	var c *PaneBox
	for i := range panes {
		if panes[i].ID == cur {
			c = &panes[i]
		}
	}
	if c == nil {
		return ""
	}
	best, bestShare, bestAt := "", 0, 0
	for _, p := range panes {
		if p.ID == cur {
			continue
		}
		// Across the border: the panes are a border's cell apart.
		var across bool
		var share, at int
		switch dir {
		case DirLeft:
			across = p.Right < c.Left && p.Right >= c.Left-2
		case DirRight:
			across = p.Left > c.Right && p.Left <= c.Right+2
		case DirUp:
			across = p.Bottom < c.Top && p.Bottom >= c.Top-2
		case DirDown:
			across = p.Top > c.Bottom && p.Top <= c.Bottom+2
		}
		if !across {
			continue
		}
		if dir == DirLeft || dir == DirRight {
			share, at = min(p.Bottom, c.Bottom)-max(p.Top, c.Top)+1, p.Top
		} else {
			share, at = min(p.Right, c.Right)-max(p.Left, c.Left)+1, p.Left
		}
		if share > bestShare || share == bestShare && share > 0 && at < bestAt {
			best, bestShare, bestAt = p.ID, share, at
		}
	}
	return best
}

// paneBoxes lists the panes of pane's window with their places, and
// whether the window is zoomed.
func (w *WS) paneBoxes(ctx context.Context, pane string) ([]PaneBox, bool, error) {
	out, err := w.T.Run(ctx, tmux.Cmd{
		"list-panes", "-t", pane, "-F",
		"#{pane_id}\t#{pane_left}\t#{pane_top}\t#{pane_right}\t#{pane_bottom}\t#{" + sidebar.OptRole + "}\t#{window_zoomed_flag}",
	})
	if err != nil {
		return nil, false, err
	}
	var boxes []PaneBox
	zoomed := false
	for _, r := range tmux.Rows(out, 7) {
		n := make([]int, 4)
		for i := range n {
			n[i], _ = strconv.Atoi(r[1+i])
		}
		boxes = append(boxes, PaneBox{ID: r[0], Left: n[0], Top: n[1], Right: n[2], Bottom: n[3], Role: r[5]})
		zoomed = zoomed || strings.TrimSpace(r[6]) == "1"
	}
	return boxes, zoomed, nil
}

// SwapPane swaps pane with its neighbor on side dir (prefix H/J/K/L,
// D67): the focus stays on pane, now in the neighbor's place. A sidebar
// stays where it is, and nothing moves past the window's edge or while the
// window is zoomed. It reports whether the panes moved.
func (w *WS) SwapPane(ctx context.Context, pane, dir string) (bool, error) {
	boxes, zoomed, err := w.paneBoxes(ctx, pane)
	if err != nil || zoomed {
		return false, err
	}
	nb := Neighbor(boxes, pane, dir)
	if nb == "" {
		return false, nil
	}
	for _, b := range boxes {
		if (b.ID == pane || b.ID == nb) && b.Role == sidebar.RoleSidebar {
			return false, nil
		}
	}
	if _, err := w.T.Run(ctx, tmux.Cmd{"swap-pane", "-d", "-s", pane, "-t", nb}); err != nil {
		return false, err
	}
	w.RefreshSidebars(ctx)
	return true, nil
}
