package sidebar

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/gitinfo"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// The row of an agent whose prompts i-mux answered shows them (D112): "✓ N"
// instead of the spinner while it works, and while it idles until the pane is
// looked at; "Needs you" comes first.
func TestApprovedBadge(t *testing.T) {
	b, w := badge(PaneRow{State: StateWorking, Approved: 3}, 0, false)
	if !strings.Contains(b, "✓ 3") || w != 5 {
		t.Errorf("working: %q %d", b, w)
	}
	b, _ = badge(PaneRow{State: StateIdle, Approved: 12}, 0, false)
	if !strings.Contains(b, "✓ 12") {
		t.Errorf("idle: %q", b)
	}
	if b, _ := badge(PaneRow{State: StateWorking, Approved: 3}, 0, true); b != approvedPill+"✓"+reset {
		t.Errorf("compact: %q", b)
	}
	if b, _ := badge(PaneRow{State: StateBlocked, Approved: 3}, 0, false); !strings.Contains(b, "Needs you") {
		t.Errorf("blocked: %q", b)
	}
	if b, _ := badge(PaneRow{State: StateExited, Approved: 3}, 0, false); strings.Contains(b, "✓") {
		t.Errorf("exited: %q", b)
	}
	if b, _ := badge(PaneRow{State: StateWorking, Approved: 0}, 0, false); strings.Contains(b, "✓") {
		t.Errorf("none: %q", b)
	}
}

// The count comes from the pane's option, which the watcher writes (D112).
func TestApprovedReadFromOption(t *testing.T) {
	h := testutil.Isolate(t)
	c := realEngine(t)
	ctx := context.Background()
	out, err := c.Run(ctx, tmux.Cmd{"new-session", "-d", "-x", "120", "-y", "40", "-c", h.Project, "-P", "-F", "#{pane_id}", "sh"})
	if err != nil {
		t.Fatal(err)
	}
	pane := strings.TrimSpace(out)
	out, _ = c.Run(ctx, tmux.Cmd{"split-window", "-h", "-b", "-f", "-d", "-l", "30", "-t", pane, "-P", "-F", "#{pane_id}", "sh"})
	bar := strings.TrimSpace(out)
	now := testutil.FrozenTime
	stamp := strconv.FormatInt(now.Unix(), 10)
	_, err = c.Run(ctx,
		tmux.Cmd{"set-option", "-p", "-t", bar, OptRole, RoleSidebar},
		tmux.Cmd{"set-option", "-p", "-t", pane, "@imux_agent", "claude"},
		tmux.Cmd{"set-option", "-p", "-t", pane, "@imux_label", "Claude"},
		tmux.Cmd{"set-option", "-p", "-t", pane, OptState, StateWorking},
		tmux.Cmd{"set-option", "-p", "-t", pane, OptStateAt, stamp},
		tmux.Cmd{"set-option", "-p", "-t", pane, OptApproved, "7"},
		tmux.Cmd{"select-pane", "-t", pane})
	if err != nil {
		t.Fatal(err)
	}
	src := &Source{
		T: c, Cfg: config.Default(), Now: func() time.Time { return now },
		Resolve: func(*config.Config, *state.State) map[session.AgentID]agent.Resolved {
			return map[session.AgentID]agent.Resolved{"claude": {Installed: true}}
		},
		Git: func(context.Context, string) gitinfo.Info {
			return gitinfo.Info{State: gitinfo.OK, Branch: "main"}
		},
	}
	m, _, err := src.Gather(ctx, bar)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Panes) != 1 || m.Panes[0].Approved != 7 {
		t.Fatalf("panes %+v", m.Panes)
	}
	_, _ = c.Run(ctx, tmux.Cmd{"set-option", "-p", "-t", pane, OptApproved, "0"})
	if m, _, _ = src.Gather(ctx, bar); m.Panes[0].Approved != 0 {
		t.Fatalf("after a reset: %+v", m.Panes[0])
	}
	_, _ = c.Run(ctx, tmux.Cmd{"kill-session"})
}
