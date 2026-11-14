package sidebar

import (
	"context"
	"strings"
	"testing"
)

// TestNestWorktrees (T77.2): a worktree's project goes right under its
// repository's, sorted; the current one takes its group to the top; a
// worktree whose repository is not open stays where it was.
func TestNestWorktrees(t *testing.T) {
	s := &Source{MainWorktree: func(_ context.Context, dir string) string {
		switch dir {
		case "/r.worktrees/b", "/r.worktrees/a":
			return "/r"
		case "/gone.worktrees/x":
			return "/gone"
		}
		return ""
	}}
	rows := map[string]*ProjectRow{
		"other": {Name: "other"}, "r": {Name: "r"}, "wa": {Name: "a"}, "wb": {Name: "b", Current: true}, "x": {Name: "x"},
	}
	cwd := map[string]string{"other": "/other", "r": "/r", "wa": "/r.worktrees/a", "wb": "/r.worktrees/b", "x": "/gone.worktrees/x"}
	got := s.nestWorktrees(context.Background(), []string{"wb", "other", "r", "wa", "x"}, rows, cwd)
	if strings.Join(got, " ") != "r wa wb other x" {
		t.Errorf("order %v", got)
	}
	if !rows["wa"].Worktree || !rows["wb"].Worktree || rows["x"].Worktree || rows["r"].Worktree {
		t.Errorf("marks %+v", rows)
	}
	// Indented with 🌿, as wide as the sidebar.
	l := projectRow(ProjectRow{Name: "feature-login", Branch: "feature-login", Worktree: true}, 40, 0)
	if p := Plain(l.Text); !strings.HasPrefix(p, "   🌿 feature") || DisplayWidth(p) != 40 {
		t.Errorf("row %q (%d)", p, DisplayWidth(p))
	}
}
