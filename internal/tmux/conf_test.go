package tmux

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/tmux -update)", err)
	}
	if string(want) != got {
		t.Fatalf("%s differs from golden file; run with -update to accept.\n--- got ---\n%s", name, got)
	}
}

func confInput(ver string) *ConfInput {
	v, _ := ParseVersion("tmux " + ver)
	return &ConfInput{
		Self:      "/opt/imux/bin/imux",
		Cfg:       config.Default(),
		Version:   v,
		Terminal:  "tmux-256color",
		AgentBar:  "#[align=left] imux #[range=user|imux:menu]☰#[norange default]",
		LocalConf: "/home/test/.imux/tmux.local.conf",
	}
}

func TestGenerateConfGolden(t *testing.T) {
	golden(t, "tmux-3.4.conf.golden", GenerateConf(confInput("3.4")))
	golden(t, "tmux-3.2.conf.golden", GenerateConf(confInput("3.2a")))
}

func TestGenerateConfOptions(t *testing.T) {
	in := confInput("3.8")
	in.Cfg.Tmux.MenuKeyRoot = ""
	in.Cfg.Tmux.Mouse = false
	in.AgentBar = ""
	got := GenerateConf(in)
	for _, bad := range []string{"bind -n 'C-\\'", "MouseDown1Status", "MouseDown3Border", "MouseDown1Pane", "SecondClick1Pane", "DoubleClick1Pane", "TripleClick1Pane", "status 2", "status-format[1]"} {
		if strings.Contains(got, bad) {
			t.Errorf("unexpected %q in:\n%s", bad, got)
		}
	}
	for _, want := range []string{"set -g  mouse off", "set -g  status on", "bind    a ", "bind    S "} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	// The user's local config is sourced last.
	lines := strings.Split(strings.TrimSpace(got), "\n")
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "source-file -q ") || !strings.Contains(last, "tmux.local.conf") {
		t.Fatalf("last line %q", last)
	}
	resize := "imux-resize h,imux-resize j,imux-resize k,imux-resize l,imux-resize Left,imux-resize Down,imux-resize Up,imux-resize Right," +
		"imux-resize H,imux-resize J,imux-resize K,imux-resize L,imux-resize S-Left,imux-resize S-Down,imux-resize S-Up,imux-resize S-Right," +
		"imux-resize Escape,imux-resize Enter,imux-resize q,"
	if ks := BoundKeys(in); strings.Join(ks, ",") != "root C-p,prefix a,prefix S,"+resize+"prefix b,prefix v,prefix -,prefix r,prefix ?,prefix H,prefix J,prefix K,prefix L,prefix c,prefix ,,root C-v,root M-v" {
		t.Fatalf("BoundKeys %q", ks)
	}
	// A reload reads them back, "prefix ," (a comma key) too.
	back := ParseBoundKeys(strings.Join(BoundKeys(in), ","))
	if len(back) != len(BoundKeys(in)) || back[len(back)-3] != [2]string{"prefix", ","} || back[3] != [2]string{"imux-resize", "h"} {
		t.Fatalf("ParseBoundKeys %q", back)
	}
	// D67: prefix H J K L swap, prefix r the resize mode, prefix ? the help;
	// the keys without the prefix only when asked for.
	for _, want := range []string{
		"bind    H run-shell -b '/opt/imux/bin/imux __pane swap left --pane #{pane_id}",
		"bind    L run-shell -b '/opt/imux/bin/imux __pane swap right",
		"bind    r { switch-client -T imux-resize ; display-message -d 0 ",
		"bind -T imux-resize h { resize-pane -L 2 ; switch-client -T imux-resize ; ",
		"bind -T imux-resize S-Up { resize-pane -U 10 ; ",
		"bind -T imux-resize Escape { switch-client -T root ",
		"bind    '?' run-shell -b '/opt/imux/bin/imux __menu --keys --pane",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("pane keys: missing %q", want)
		}
	}
	if strings.Contains(got, "bind -n C-Left") {
		t.Error("keys without the prefix while direct_pane_keys is off")
	}
	in.Cfg.Tmux.DirectPaneKeys, in.Cfg.Tmux.SwapPaneKeys, in.Cfg.Tmux.ResizeKey, in.Cfg.Tmux.HelpKey = true, false, "", ""
	direct := GenerateConf(in)
	for _, want := range []string{
		"bind -n C-Left select-pane -L", "bind -n C-Down select-pane -D",
		"bind -n C-M-Left run-shell -b '/opt/imux/bin/imux __menu --split h --before --pane",
		"bind -n C-M-Down run-shell -b '/opt/imux/bin/imux __menu --split v --pane",
		"bind -n C-M-Up run-shell -b '/opt/imux/bin/imux __menu --split v --before --pane",
	} {
		if !strings.Contains(direct, want) {
			t.Errorf("direct keys: missing %q", want)
		}
	}
	for _, gone := range []string{"__pane swap", "imux-resize", "__menu --keys"} {
		if strings.Contains(direct, gone) {
			t.Errorf("turned off but bound: %q", gone)
		}
	}
	if ks := strings.Join(BoundKeys(in), ","); !strings.Contains(ks, "root C-Left,root C-M-Left") || strings.Contains(ks, "imux-resize") || strings.Contains(ks, "prefix H") {
		t.Errorf("BoundKeys with direct keys: %s", ks)
	}
	in.Cfg.Tmux.DirectPaneKeys, in.Cfg.Tmux.SwapPaneKeys, in.Cfg.Tmux.ResizeKey, in.Cfg.Tmux.HelpKey = false, true, "r", "?"
	// Images off: Ctrl+V and Alt+V are the panes' again.
	in.Cfg.Paste.Images = false
	if got := GenerateConf(in); strings.Contains(got, "__paste-image") || strings.Contains(strings.Join(BoundKeys(in), ","), "C-v") {
		t.Error("paste bindings with images off")
	}
	for _, want := range []string{"bind    b ", "sidebar toggle", "bind    v ", "__menu --split h", "bind    - ", "__menu --split v", "set-hook -g pane-focus-in", "__sidebar-refresh"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	// The tab bar: at the top, with window ranges and the + range; c and ,
	// open a tab and name it.
	for _, want := range []string{"status-position top", "range=window|#{window_index}", "range=user|imux:tab", "@imux_tab_auto", "__menu --tab", "set-option -w @imux_tab"} {
		if !strings.Contains(got, want) {
			t.Errorf("tab bar: missing %q", want)
		}
	}
	in.Cfg.Tmux.Tabs = false
	if got := GenerateConf(in); strings.Contains(got, "imux:tab") || !strings.Contains(got, "status-position bottom") || strings.Contains(BoundKeys(in)[len(BoundKeys(in))-1], ",") {
		t.Error("tabs off")
	}
	// Without the sidebar no hook signals it.
	in.Cfg.Tmux.Sidebar = false
	if got := GenerateConf(in); strings.Contains(got, "__sidebar-refresh") {
		t.Error("sidebar hooks without a sidebar")
	}
}

func TestWriteConfOnlyOnChange(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tmux.conf")
	changed, err := WriteConf(p, "a\n", nil)
	if err != nil || !changed {
		t.Fatalf("first write: %v %v", changed, err)
	}
	fi1, _ := os.Stat(p)
	changed, err = WriteConf(p, "a\n", nil)
	if err != nil || changed {
		t.Fatalf("same content: %v %v", changed, err)
	}
	fi2, _ := os.Stat(p)
	if !fi1.ModTime().Equal(fi2.ModTime()) {
		t.Fatal("unchanged content rewrote the file")
	}
	changed, err = WriteConf(p, "b\n", nil)
	if err != nil || !changed {
		t.Fatalf("new content: %v %v", changed, err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
}

// TestConfLoadsInRealTmux sources the generated config on a real server
// (the installed tmux version) and checks that tmux accepts every line.
func TestConfLoadsInRealTmux(t *testing.T) {
	c := realClient(t)
	ctx := context.Background()
	v, err := c.BinVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	in := confInput(v.Raw)
	in.Version = v
	in.LocalConf = filepath.Join(dir, "tmux.local.conf")
	if err := os.WriteFile(in.LocalConf, []byte("set -g @imux_local_loaded yes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(dir, "tmux.conf")
	if _, err := WriteConf(conf, GenerateConf(in), nil); err != nil {
		t.Fatal(err)
	}
	// start-server with the generated file must not complain.
	cmd := exec.Command("tmux", "-L", c.Socket, "-f", conf, "start-server")
	if out, err := cmd.CombinedOutput(); err != nil || len(out) > 0 {
		t.Fatalf("start-server: %v %s", err, out)
	}
	c.Config = conf
	if _, err := c.Do(Cmd{"new-session", "-d", "-s", "conf", "sleep", "30"}); err != nil {
		t.Fatal(err)
	}
	// Sourcing it again (as Reload does) reports errors on stderr.
	if err := c.Reload(ctx, conf); err != nil {
		t.Fatalf("reload: %v", err)
	}
	out, err := c.Do(Cmd{"show-options", "-gv", "@imux_local_loaded"}, Cmd{"show-options", "-gv", "status"},
		Cmd{"show-options", "-gv", "@imux_keys"})
	if err != nil {
		t.Fatal(err)
	}
	f := strings.Fields(out)
	if len(f) < 3 || f[0] != "yes" || f[1] != "2" {
		t.Fatalf("options after load: %q", out)
	}
	// terminal-features and the Sync override are appended only once across
	// reloads.
	if err := c.Reload(ctx, conf); err != nil {
		t.Fatal(err)
	}
	tf, _ := c.Do(Cmd{"show-options", "-s", "terminal-features"})
	if n := strings.Count(tf, "extkeys"); n != 1 {
		t.Fatalf("extkeys feature appears %d times:\n%s", n, tf)
	}
	to, _ := c.Do(Cmd{"show-options", "-s", "terminal-overrides"})
	if n := strings.Count(to, "Sync="); n != 1 || !strings.Contains(to, "?2026") {
		t.Fatalf("the Sync override appears %d times:\n%s", n, to)
	}
	keys, _ := c.Do(Cmd{"list-keys", "-T", "root"})
	if !strings.Contains(keys, "__menu") {
		t.Fatalf("root menu key not bound:\n%s", keys)
	}
	for key, want := range map[string]string{"DoubleClick1Pane": "@imux_role", "MouseDown1Pane": OptMoved, "SecondClick1Pane": "@imux_press"} {
		bound := ""
		for _, l := range strings.Split(keys, "\n") {
			if strings.Contains(l, " "+key+" ") {
				bound = l
			}
		}
		if !strings.Contains(bound, want) {
			t.Fatalf("i-mux's %s is not bound: %q", key, bound)
		}
	}
	// A changed config unbinds keys it no longer uses.
	in.Cfg.Tmux.MenuKeyRoot = "M-m"
	if _, err := WriteConf(conf, GenerateConf(in), nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Reload(ctx, conf); err != nil {
		t.Fatal(err)
	}
	keys, _ = c.Do(Cmd{"list-keys", "-T", "root"})
	if strings.Contains(keys, `C-\`) && strings.Contains(keys, "__menu --pane") && !strings.Contains(keys, "M-m") {
		t.Fatalf("old key still bound:\n%s", keys)
	}
	if !strings.Contains(keys, "M-m") {
		t.Fatalf("new key not bound:\n%s", keys)
	}
}
