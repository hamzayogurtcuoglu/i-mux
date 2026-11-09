package workspace

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestConversationNames: an agent named in the sidebar names its
// conversation, which History lists by that name; a name given in History
// renames the open agent running it; continuing a named conversation names
// its pane.
func TestConversationNames(t *testing.T) {
	i18n.SetLocale("en")
	w, ft, h := testWS(t)
	f := sources.New(config.Default())
	ctx := context.Background()
	now := testutil.FrozenTime
	named, other := "44444444-0000-4000-8000-000000000001", "44444444-0000-4000-8000-000000000002"
	writeClaudeSession(t, h, named, "fix the login redirect", now.Add(-time.Hour))
	writeClaudeSession(t, h, other, "write the docs", now.Add(-2*time.Hour))
	key := "claude:" + named
	row := func(id, role, name string, tr Trail) string {
		return strings.Join([]string{id, "claude", "", h.Project, h.Project, "Claude", "0", "proj-1", "@1", "node", "", "", tr.String(), role, name}, "\t")
	}
	trail := Trail{{Agent: "claude", Start: now.Add(-3 * time.Hour).UnixMilli(), SessionID: named}}
	ft.replies["list-panes"] = row("%1", "", "backend fix", trail) + "\n" + row("%2", "sidebar", "", nil) + "\n" +
		row("%3", "", "", Trail{{Agent: "claude", Start: now.Add(-3 * time.Hour).UnixMilli(), SessionID: other}}) + "\n"

	w.RememberNames(ctx, f, nil)
	st, _ := state.Read()
	if st.Names[key] != "backend fix" || len(st.Names) != 1 {
		t.Fatalf("names %v", st.Names)
	}

	// History: the name first, then the preview.
	m := w.PickerMenu(ctx, f, &Pane{ID: "%1", CWD: h.Project}, "c1", 0)
	var rows []string
	for _, it := range m.Items {
		rows = append(rows, it.Name)
	}
	list := strings.Join(rows, "\n")
	if !strings.Contains(list, "backend fix · fix the login redirect") || !strings.Contains(list, "write the docs") || strings.Contains(list, "backend fix · write") {
		t.Fatalf("History:\n%s", list)
	}

	// A name given in History goes to the open agent running it.
	ft.calls = nil
	if err := w.NameConversation(ctx, f, key, "api work"); err != nil {
		t.Fatal(err)
	}
	if w.NameOf(key) != "api work" || !called(ft, "set-option -p -t %1 @imux_name api work") || called(ft, "-t %3 @imux_name") {
		t.Fatalf("rename: %q, calls %q", w.NameOf(key), ft.calls)
	}
	ft.calls = nil
	if err := w.NameConversation(ctx, f, key, ""); err != nil {
		t.Fatal(err)
	}
	if w.NameOf(key) != "" || !called(ft, "set-option -p -u -t %1 @imux_name") {
		t.Fatalf("unname: %q, calls %q", w.NameOf(key), ft.calls)
	}

	// Continuing a named conversation names its pane.
	_ = w.NameConversation(ctx, f, key, "backend fix")
	ft.calls = nil
	w.nameFromConversation(ctx, "%9", key)
	w.nameFromConversation(ctx, "%8", "claude:"+other)
	if !called(ft, "set-option -p -t %9 @imux_name backend fix") || called(ft, "%8") {
		t.Fatalf("continue: calls %q", ft.calls)
	}
}

// called reports whether a tmux call holds want (its arguments joined).
func called(ft *fakeTmux, want string) bool {
	for _, c := range ft.calls {
		if strings.Contains(strings.Join(c, " "), want) {
			return true
		}
	}
	return false
}
