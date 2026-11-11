package popmenu

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// menuCmd is a display-menu as i-mux builds one: a disabled first item, a
// separator, an escaped "#" and an item without a key.
func menuCmd() tmux.Cmd {
	return tmux.Cmd{
		"display-menu", "-M", "-O", "-c", "/dev/pts/3", "-t", "%3", "-T", "i-mux ## Claude", "-x", "C", "-y", "C", "--",
		"-Claude: limited until 15:00", "", "",
		"",
		"Switch to Codex", "x", "run-shell -b 'imux switch codex --pane %3'",
		"Issue ##42", "", "display-message hi",
		"",
		"Close pane", "k", "kill-pane -t %3",
	}
}

func TestParse(t *testing.T) {
	m, err := Parse(menuCmd())
	if err != nil {
		t.Fatal(err)
	}
	want := &Menu{Title: "i-mux # Claude", Client: "/dev/pts/3", Pane: "%3", X: "C", Y: "C", Items: []Item{
		{Name: "Claude: limited until 15:00", Disabled: true},
		{Sep: true},
		{Name: "Switch to Codex", Key: "x", Cmd: "run-shell -b 'imux switch codex --pane %3'"},
		{Name: "Issue #42", Cmd: "display-message hi"},
		{Sep: true},
		{Name: "Close pane", Key: "k", Cmd: "kill-pane -t %3"},
	}}
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("got %+v\nwant %+v", m, want)
	}
	// Without "--", at a position, and the errors.
	m, err = Parse(tmux.Cmd{"display-menu", "-x", "#{e|+:#{popup_pane_left},3}", "-y", "9", "One", "o", "x"})
	if err != nil || m.X != "#{e|+:#{popup_pane_left},3}" || m.Y != "9" || len(m.Items) != 1 {
		t.Fatalf("%+v %v", m, err)
	}
	for _, bad := range []tmux.Cmd{{"display-popup"}, {"display-menu", "-Z"}, {"display-menu", "-c"}, {"display-menu", "--", "One", "o"}} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

// TestSize: the popup holds the longest row (the title with ✕, an item and
// its key) and every item below the title row, inside tmux's border.
func TestSize(t *testing.T) {
	m, _ := Parse(menuCmd())
	w, h := Size(m)
	if inner := sidebar.DisplayWidth("  Claude: limited until 15:00 "); w != inner+2 || h != len(m.Items)+3 {
		t.Fatalf("size %dx%d", w, h)
	}
	if w, _ := Size(&Menu{Title: "T", Items: []Item{{Name: "a", Key: "a"}}}); w != 18 {
		t.Fatalf("narrow menu width %d", w)
	}
}

// TestView: hover selects, a click or a key chooses, separators and disabled
// items are skipped, the arrows move, Enter, Esc, q and ✕ close.
func TestView(t *testing.T) {
	m, _ := Parse(menuCmd())
	v := NewView(m)
	w, h := Size(m)
	lines := v.Render(w-2, h-2)
	if len(lines) != h-2 || !strings.Contains(sidebar.Plain(lines[0]), "i-mux # Claude") || !strings.HasSuffix(sidebar.Plain(lines[0]), "✕ ") {
		t.Fatalf("title row %q", lines)
	}
	for _, l := range lines {
		if sidebar.DisplayWidth(sidebar.Plain(l)) != w-2 {
			t.Fatalf("row %q is not %d wide", sidebar.Plain(l), w-2)
		}
	}
	if r := sidebar.Plain(lines[3]); !strings.HasPrefix(r, "  Switch to Codex ") || !strings.HasSuffix(r, " x ") || !strings.Contains(sidebar.Plain(lines[2]), "────") {
		t.Fatalf("rows %q", lines)
	}
	if v.Selected() != -1 {
		t.Fatal("something selected at first")
	}
	v.Move(5, 2) // the disabled item
	if v.Selected() != -1 {
		t.Fatal("a disabled item was selected")
	}
	v.Move(5, 4)
	if v.Selected() != 2 || !strings.HasPrefix(v.Render(w-2, h-2)[3], selBg) {
		t.Fatal("hover did not select")
	}
	for _, c := range []struct {
		x, y int
		want Result
	}{
		{5, 3, none}, // a separator
		{5, 2, none}, // disabled
		{5, 1, none}, // the title
		{w - 3, 1, Result{Chosen: -1, Done: true}}, // ✕
		{5, 5, chosen(3)},
		{5, 7, chosen(5)},
		{5, 9, none}, // below the items
	} {
		if got := v.Click(c.x, c.y); got != c.want {
			t.Errorf("click at %d,%d: %+v", c.x, c.y, got)
		}
	}
	keys := NewView(m)
	keys.Render(w-2, h-2)
	if got := keys.Key("k"); got != chosen(5) {
		t.Fatalf("an item's key k: %+v", got)
	}
	if got := keys.Key("x"); got != chosen(2) {
		t.Fatalf("x: %+v", got)
	}
	keys.Key("down")
	keys.Key("down")
	if keys.Selected() != 3 {
		t.Fatalf("down twice: %d", keys.Selected())
	}
	if got := keys.Key("enter"); got != chosen(3) {
		t.Fatalf("enter: %+v", got)
	}
	keys.Key("end")
	keys.Key("up")
	if keys.Selected() != 3 {
		t.Fatalf("end, up: %d", keys.Selected())
	}
	for _, k := range []string{"esc", "q", "ctrl-c"} {
		if got := keys.Key(k); got != (Result{Chosen: -1, Done: true}) {
			t.Errorf("%s: %+v", k, got)
		}
	}
	if !matches("Enter", "enter") || !matches("C-a", "ctrl-a") || matches("c", "C") || matches("", "x") {
		t.Fatal("key names")
	}
}

// TestViewScrolls: a menu taller than the screen scrolls to the selection.
func TestViewScrolls(t *testing.T) {
	m := &Menu{Title: "Branches"}
	for _, b := range []string{"a", "b", "c", "d", "e", "f"} {
		m.Items = append(m.Items, Item{Name: b, Cmd: "x"})
	}
	v := NewView(m)
	v.Render(20, 4) // the title and 3 items
	for i := 0; i < 5; i++ {
		v.Key("down")
	}
	lines := v.Render(20, 4)
	if v.Selected() != 4 || !strings.Contains(sidebar.Plain(lines[3]), "e") || !strings.Contains(sidebar.Plain(lines[1]), "c") {
		t.Fatalf("selected %d, rows %q", v.Selected(), lines)
	}
}

// fakeSpawner records the tmux client Show spawns.
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

// TestShow: the menu is written for the popup, which one tmux client shows
// at the menu's place (after clearing a status-line message, which would
// freeze the popup on tmux 3.4), then that client sources the choice and
// removes it.
func TestShow(t *testing.T) {
	home := t.TempDir()
	t.Setenv("IMUX_HOME", home)
	sp := &fakeSpawner{}
	c := &tmux.Client{Bin: "tmux", Socket: "s", Runner: versionRunner("tmux 3.4\n"), Spawner: sp}
	if err := Show("/opt/imux")(context.Background(), c, menuCmd()); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(home, "menus", "*.json"))
	if len(files) != 1 {
		t.Fatalf("menu files %q", files)
	}
	id := strings.TrimSuffix(filepath.Base(files[0]), ".json")
	b, _ := os.ReadFile(files[0])
	var m Menu
	if err := json.Unmarshal(b, &m); err != nil || m.Pane != "%3" || len(m.Items) != 6 {
		t.Fatalf("written menu %s: %v", b, err)
	}
	// Windows has no Unix permission bits; the file is in the user's profile.
	if fi, _ := os.Stat(files[0]); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 || !ValidID(id) {
		t.Fatalf("file %s", fi.Mode())
	}
	w, h := Size(&m)
	out := filepath.Join(home, "menus", id+".tmux")
	want := []string{
		"-L", "s", "-u",
		"display-message", "-d", "1", "-c", "/dev/pts/3", "", ";",
		"display-popup", "-c", "/dev/pts/3", "-t", "%3", "-E", "-w", itoa(w), "-h", itoa(h), "-x", "C", "-y", "C", tmux.ShJoin("/opt/imux", "__menu-popup", id), ";",
		"source-file", "-q", out, ";",
		"run-shell", "-b", tmux.EscapeFormat("rm -f " + tmux.ShQuote(out)),
	}
	if !reflect.DeepEqual(sp.args, want) || !reflect.DeepEqual(sp.env, []string{"TMUX_PANE=%3"}) {
		t.Fatalf("spawned %q env %q\nwant %q", sp.args, sp.env, want)
	}
	// Something else than a display-menu is spawned as it is.
	if err := Show("/opt/imux")(context.Background(), c, tmux.Cmd{"display-menu", "-Z"}); err != nil || sp.args[len(sp.args)-1] != "-Z" {
		t.Fatalf("spawned %q", sp.args)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }
