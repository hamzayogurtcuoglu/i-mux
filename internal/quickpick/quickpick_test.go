package quickpick

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/popmenu"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func pick() *Pick {
	return &Pick{
		Title: "Select a ref to checkout", Placeholder: "type to filter", Empty: "No matching refs",
		Items: []Item{
			{Label: "+ Create new branch…", Always: true, Query: true, Argv: []string{"new"}},
			{Label: "main", Description: "1a2b3c4 · init", Group: "branches", Argv: []string{"main"}},
			{Label: "feature/login", Description: "9f8e7d6 · login", Group: "branches"},
			{Label: "origin/main", Group: "remote branches"},
			{Label: "v1.0", Group: "tags"},
		},
	}
}

func plain(lines []string) string {
	var b []string
	for _, l := range lines {
		b = append(b, strings.TrimRight(sidebar.Plain(l), " "))
	}
	return strings.Join(b, "\n")
}

// TestModel: the items list in their groups; typing filters them, the best
// match first and selected (the Always one stays, after the matches), the
// arrows skip group rows, Enter chooses.
func TestModel(t *testing.T) {
	m := New(pick())
	got := plain(m.Render(40, 12))
	for _, want := range []string{" Select a ref to checkout", " › type to filter", "── branches ─", "   main  1a2b3c4 · init", "── tags ─", "   v1.0"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q:\n%s", want, got)
		}
	}
	if m.Selected() != 0 {
		t.Fatalf("first selected %d", m.Selected())
	}
	m.Key("down")
	if m.Selected() != 1 {
		t.Fatalf("down skips the group row: %d", m.Selected())
	}
	m.Key("up")
	m.Key("up")
	if m.Selected() != 0 {
		t.Fatalf("up stops at the top: %d", m.Selected())
	}
	for _, k := range []string{"l", "o", "g"} {
		m.Key(k)
	}
	got = plain(m.Render(40, 12))
	if !strings.Contains(got, " › log") || strings.Contains(got, "v1.0") || strings.Contains(got, "branches") ||
		!strings.Contains(got, "+ Create new branch…") || strings.Index(got, "feature/login") > strings.Index(got, "+ Create") {
		t.Fatalf("filtered:\n%s", got)
	}
	if r := m.Key("enter"); !r.Done || r.Chosen != 2 {
		t.Fatalf("chose %+v", r)
	}
	m.Key("down")
	if m.Selected() != 0 {
		t.Fatalf("down from the match: %d", m.Selected())
	}
	m.Key("backspace")
	if m.Query() != "lo" {
		t.Fatalf("query %q", m.Query())
	}
	m.Key("ctrl-u")
	m.Type("zz\n")
	if got := plain(m.Render(40, 12)); !strings.Contains(got, "No matching refs") && !strings.Contains(got, "+ Create") {
		t.Fatalf("nothing matches:\n%s", got)
	}
	if r := m.Key("esc"); !r.Done || r.Chosen != -1 {
		t.Fatalf("esc %+v", r)
	}
}

// TestMouse: the pointer selects, a click chooses, ✕ closes, the wheel
// scrolls.
func TestMouse(t *testing.T) {
	m := New(pick())
	m.Render(40, 6) // three list rows
	m.Move(5, 5)    // the "branches" group row
	if m.Selected() != 0 {
		t.Fatalf("a group row was selected: %d", m.Selected())
	}
	m.Move(5, 6)
	if m.Selected() != 1 {
		t.Fatalf("hover %d", m.Selected())
	}
	if r := m.Click(40, 1); !r.Done || r.Chosen != -1 {
		t.Fatalf("✕ %+v", r)
	}
	m.Scroll(false)
	if got := plain(m.Render(40, 6)); !strings.Contains(got, "origin/main") || strings.Contains(got, "+ Create") {
		t.Fatalf("scrolled:\n%s", got)
	}
	if r := m.Click(5, 4); !r.Done || r.Chosen < 0 {
		t.Fatalf("click %+v", r)
	}
}

func TestSize(t *testing.T) {
	w, h := Size(pick(), 200, 60)
	if w != 48 || h != 13 {
		t.Fatalf("size %d×%d", w, h) // 5 items, 3 groups; title, input, line, border
	}
	if w, h := Size(pick(), 40, 10); w != 36 || h != 8 {
		t.Fatalf("small client %d×%d", w, h)
	}
}

type fakeSpawner struct{ args, env []string }

func (f *fakeSpawner) Spawn(_ string, args []string) error { f.args = args; return nil }

func (f *fakeSpawner) SpawnEnv(_ string, args, env []string) error {
	f.args, f.env = args, env
	return nil
}

type versionRunner string

func (v versionRunner) Run(context.Context, string, []string) ([]byte, []byte, error) {
	return []byte(v), nil, nil
}

// TestShowLoad: the pick is written for the popup, which runs
// `imux __quickpick <id>`; Load reads it once.
func TestShowLoad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("IMUX_HOME", home)
	sp := &fakeSpawner{}
	c := &tmux.Client{Bin: "tmux", Socket: "s", Runner: versionRunner("tmux 3.2a\n"), Spawner: sp}
	p := pick()
	p.Client, p.Pane = "/dev/pts/3", "%3"
	if err := Show(context.Background(), c, "/opt/imux", p, 200, 60); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(home, "menus", "*.pick.json"))
	if len(files) != 1 {
		t.Fatalf("files %q", files)
	}
	id := strings.TrimSuffix(filepath.Base(files[0]), ".pick.json")
	want := []string{
		"-L", "s", "-u", "display-popup", "-c", "/dev/pts/3", "-t", "%3", "-E", "-w", "48", "-h", "13", "-x", "C", "-y", "C",
		tmux.ShJoin("/opt/imux", "__quickpick", id), ";",
		"run-shell", "-b", tmux.EscapeFormat(tmux.ShJoin("/opt/imux", "__quickpick", "--run", id)),
	}
	if !reflect.DeepEqual(sp.args, want) {
		t.Fatalf("spawned %q\nwant %q", sp.args, want)
	}
	got, err := Load(id)
	if err != nil || !reflect.DeepEqual(got, p) {
		t.Fatalf("loaded %+v %v", got, err)
	}
	if _, err := Load(id); err == nil {
		t.Fatal("loaded twice")
	}
	if _, err := Load("../x"); err == nil {
		t.Fatal("a bad id")
	}
}

func TestExec(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	out := filepath.Join(t.TempDir(), "ran")
	if err := Exec(sh, []string{"-c", `printf '%s' "$1" > "$2"`, "sh", "a $(b) #c", out}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); string(b) != "a $(b) #c" {
		t.Fatalf("argv reached the program as %q", b)
	}
	if err := Exec(sh, nil, nil, nil); err != nil {
		t.Fatal("an item without argv runs nothing")
	}
}

// TestArgv: an item with Query gets what was typed as --initial, before
// its data.
func TestArgv(t *testing.T) {
	it := Item{Argv: []string{"__scm", "ask", "create-branch", "--cwd", "/p"}, Query: true}
	if got := Argv(it, "feat/x"); !reflect.DeepEqual(got, []string{"__scm", "ask", "create-branch", "--cwd", "/p", "--initial", "feat/x"}) {
		t.Errorf("no data: %q", got)
	}
	it.Argv = []string{"__scm", "pick", "x", "--", "-ref"}
	if got := Argv(it, "a b"); !reflect.DeepEqual(got, []string{"__scm", "pick", "x", "--initial", "a b", "--", "-ref"}) {
		t.Errorf("data: %q", got)
	}
	if got := Argv(it, ""); !reflect.DeepEqual(got, it.Argv) {
		t.Errorf("nothing typed: %q", got)
	}
	it.Query = false
	if got := Argv(it, "a"); !reflect.DeepEqual(got, it.Argv) {
		t.Errorf("no Query: %q", got)
	}
}

// TestLater: a choice that does not run in the popup runs once it closed,
// once; a popup closed without one runs nothing.
func TestLater(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	t.Setenv("IMUX_HOME", t.TempDir())
	id, err := popmenu.NewID()
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "ran")
	if err := Later(id, []string{"-c", `printf '%s' "$1" >> "$2"`, "sh", "x;$(y)", out}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := RunLater(sh, id); err != nil {
			t.Fatal(err)
		}
	}
	if b, _ := os.ReadFile(out); string(b) != "x;$(y)" {
		t.Fatalf("ran %q", b)
	}
	if err := Later("../x", nil); err == nil {
		t.Fatal("a bad id")
	}
}
