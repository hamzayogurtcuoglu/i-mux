package sidebar

import (
	"context"
	"strconv"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// PanesFormat lists the panes of a pane's window (their ids).
const PanesFormat = "#{P:#{pane_id} }"

// Geom is what the sidebar sees of its pane and window.
type Geom struct {
	W, H  int    // the sidebar pane's size
	WinW  int    // the window's width
	Panes string // the window's panes (PanesFormat)
}

// Dragged reports whether the sidebar's width went from prev to cur by the
// user's hand: its edge dragged (or resize-pane), the window keeping its
// width and its panes. A window that changed (a client attached, the
// terminal resized) or lost a pane (closing the agent next to the sidebar
// gives it the room) is not a drag: that width is not the user's.
func Dragged(prev, cur Geom) bool {
	return prev.W > 0 && cur.W > 0 && cur.W != prev.W && cur.WinW == prev.WinW && cur.Panes == prev.Panes
}

// ClampWidth keeps a dragged width within what the config allows.
func ClampWidth(w int) int {
	return min(max(w, config.MinSidebarWidth), config.MaxSidebarWidth)
}

// SaveAfter is how long a dragged width rests before it is saved: a drag
// is a stream of resizes, saved once at its end.
var SaveAfter = 600 * time.Millisecond

// SaveWidth writes the panel's width into the config (tmux.sidebar_width),
// which every sidebar and new windows use; tests replace it.
var SaveWidth = func(path string, w int) error {
	return config.Set(path, "tmux", "sidebar_width", strconv.Itoa(w))
}

// settle keeps the sidebar at its width (§9.3 "Resizing the panel"). A width
// the user dragged it to becomes its width, saved shortly after; any other
// change is undone while the window has room for it. It reports whether it
// took a dragged width (the caller then only draws again).
func (s *sidebar) settle(ctx context.Context, cur Geom, closed bool) bool {
	prev := s.geom
	s.geom = cur
	want := s.width(closed)
	if want <= 0 || cur.W == want {
		return false
	}
	took := false
	if !closed && Dragged(prev, cur) {
		// The rail alone (a closed panel) keeps its width: a click on a
		// view's icon opens it.
		want = ClampWidth(cur.W)
		s.o.Width = want
		s.saveLater()
		took = true
		if want == cur.W {
			return true
		}
	}
	if cur.WinW >= want+20 {
		_, _ = s.t.Run(ctx, tmux.Cmd{"resize-pane", "-t", s.o.Pane, "-x", strconv.Itoa(want)})
	}
	return took
}

// saveLater saves the width once the drag has rested for SaveAfter.
func (s *sidebar) saveLater() {
	if s.saveT != nil {
		s.saveT.Stop()
	}
	s.saveT = time.AfterFunc(SaveAfter, func() {
		select {
		case s.save <- struct{}{}:
		default:
		}
	})
}

// saveWidth writes a dragged width into the config and tells the other
// sidebars, which read it and take it (every tab has the same panel).
func (s *sidebar) saveWidth(ctx context.Context) {
	src := s.o.Source
	if src.CfgPath == "" || src.Cfg != nil && src.Cfg.Tmux.SidebarWidth == s.o.Width {
		return
	}
	if err := SaveWidth(src.CfgPath, s.o.Width); err != nil {
		return // a config that does not load stays as it is; the width holds here
	}
	if src.Cfg != nil {
		src.Cfg.Tmux.SidebarWidth = s.o.Width
	}
	RefreshAll(ctx, s.t)
}
