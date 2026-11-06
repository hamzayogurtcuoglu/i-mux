package antigravity

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func writeTask(t *testing.T, home, dir, id, body string, mtime time.Time) {
	t.Helper()
	p := filepath.Join(home, dir, id, "task.md")
	testutil.WriteFile(t, p, []byte(body))
	if err := os.Chtimes(p, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestParseTaskList(t *testing.T) {
	h := testutil.Isolate(t)
	p := filepath.Join(h.Home, "task.md")
	testutil.WriteFile(t, p, []byte("# Task: fix the login redirect\n\n"+
		"- [x] Read src/auth.ts\n"+
		"- [/] Fix the redirect after login\n"+
		"  * [ ] Add a test for it\n"+
		"1. [-] Upgrade the router (blocked)\n"+
		"- not a checkbox\n"+
		"- [ ]\n"+
		"Some prose with [x] inside.\n"))
	want := []session.Todo{
		{Text: "Read src/auth.ts", Status: session.TodoDone},
		{Text: "Fix the redirect after login", Status: session.TodoInProgress},
		{Text: "Add a test for it", Status: session.TodoPending},
		{Text: "Upgrade the router (blocked)", Status: session.TodoBlocked},
	}
	if got := ParseTaskList(p); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
	if ParseTaskList(filepath.Join(h.Home, "missing.md")) != nil {
		t.Fatal("a missing file gives nil")
	}
}

// Todos picks the newest task.md changed during the run, from the CLI's and
// the app's brain directories.
func TestTodosNewestSinceRunStart(t *testing.T) {
	h := testutil.Isolate(t)
	start := testutil.FrozenTime
	writeTask(t, h.Home, BrainDirs[1], "11111111-old", "- [ ] stale plan\n", start.Add(-time.Hour))
	writeTask(t, h.Home, BrainDirs[1], "22222222-app", "- [x] app plan\n", start.Add(time.Minute))
	writeTask(t, h.Home, BrainDirs[0], "33333333-cli", "- [/] cli plan\n", start.Add(2*time.Minute))
	got := Todos(h.Home, start)
	if len(got) != 1 || got[0].Text != "cli plan" || got[0].Status != session.TodoInProgress {
		t.Fatalf("got %+v", got)
	}
	if got := Todos(h.Home, start.Add(time.Hour)); got != nil {
		t.Fatalf("nothing changed after since: %+v", got)
	}
}
