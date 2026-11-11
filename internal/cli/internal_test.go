package cli

import (
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/launch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func TestExecSpecReadsDeletesAndExecs(t *testing.T) {
	testutil.Isolate(t)
	t.Setenv("TMUX_PANE", "%7")
	args := []string{"--session-id", "u", "it's \"q\"\n$(x) `y` ğüşıöç"}
	p, err := launch.Write(&launch.Spec{Agent: "claude", Bin: "/bin/true", Args: args, Dir: "/", Env: map[string]string{"PATH": "/x"}})
	if err != nil {
		t.Fatal(err)
	}
	var got *launch.Spec
	launchExec = func(s *launch.Spec) error { got = s; return nil }
	defer func() { launchExec = launch.Exec }()
	if _, errOut, code := runCLI(t, "__exec", p); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("spec file not deleted")
	}
	if got == nil || got.Pane != "%7" || !reflect.DeepEqual(got.Args, args) || got.Env["PATH"] != "/x" {
		t.Fatalf("spec %+v", got)
	}
	if _, _, code := runCLI(t, "__exec"); code != ExitUsage {
		t.Fatalf("missing arg: exit %d", code)
	}
	if _, _, code := runCLI(t, "__exec", p); code != ExitError {
		t.Fatalf("consumed spec: exit %d", code)
	}
}

func TestSnippetCommandWritesNothing(t *testing.T) {
	h := testutil.Isolate(t)
	inProject(t, h)
	out, errOut, code := runCLI(t, "snippet", "claude")
	if code != 0 || !strings.Contains(out, "imux send") || !strings.Contains(errOut, "CLAUDE.md") {
		t.Fatalf("snippet: %d %q %q", code, out, errOut)
	}
	if ents, _ := os.ReadDir(h.Project); len(ents) != 0 {
		t.Fatalf("snippet created files: %v", ents)
	}
	if _, _, code := runCLI(t, "snippet", "vim"); code != ExitUsage {
		t.Fatalf("unknown target: exit %d", code)
	}
}

func TestSetSetting(t *testing.T) {
	testutil.Isolate(t)
	if err := setSetting("icons", "nerd"); err != nil {
		t.Fatal(err)
	}
	if err := setSetting("completion", "on"); err != nil {
		t.Fatal(err)
	}
	if err := setSetting("on_limit", "off"); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := config.Load(paths.Config())
	if err != nil || cfg.Tmux.Icons != "nerd" || !cfg.Completion.Enabled || cfg.OnLimit != "off" {
		t.Fatalf("config %v %+v", err, cfg)
	}
	for _, bad := range [][2]string{{"icons", "big"}, {"nope", "x"}, {"commit_agent", "someone"}} {
		if err := setSetting(bad[0], bad[1]); err == nil {
			t.Errorf("%s=%s accepted", bad[0], bad[1])
		}
	}
	if err := setSetting("completion", "off"); err != nil {
		t.Fatal(err)
	}
	if cfg, _, _ := config.Load(paths.Config()); cfg.Completion.Enabled {
		t.Fatal("still on")
	}
}

func TestImageText(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows quotes paths instead: TestImageTextWindows")
	}
	if got := ImageText("claude", []string{"/a/b.png", "/my shots/c.png"}); got != `/a/b.png /my\ shots/c.png ` {
		t.Fatalf("claude %q", got)
	}
	if got := ImageText("antigravity", []string{"/a/b.png"}); got != "@/a/b.png " {
		t.Fatalf("antigravity %q", got)
	}
}
