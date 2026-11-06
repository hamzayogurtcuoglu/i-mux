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
	for _, bad := range []string{"bind -n 'C-\\'", "MouseDown1Status", "MouseDown3Border", "status 2", "status-format[1]"} {
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
	if ks := BoundKeys(in); strings.Join(ks, ",") != "prefix a,prefix S" {
		t.Fatalf("BoundKeys %q", ks)
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
	// terminal-features is appended only once across reloads.
	if err := c.Reload(ctx, conf); err != nil {
		t.Fatal(err)
	}
	tf, _ := c.Do(Cmd{"show-options", "-s", "terminal-features"})
	if n := strings.Count(tf, "extkeys"); n != 1 {
		t.Fatalf("extkeys feature appears %d times:\n%s", n, tf)
	}
	keys, _ := c.Do(Cmd{"list-keys", "-T", "root"})
	if !strings.Contains(keys, "__menu") {
		t.Fatalf("root menu key not bound:\n%s", keys)
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
