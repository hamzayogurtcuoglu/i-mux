package cli

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/launch"
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
	if code != 0 || !strings.Contains(out, "ajan send") || !strings.Contains(errOut, "CLAUDE.md") {
		t.Fatalf("snippet: %d %q %q", code, out, errOut)
	}
	if ents, _ := os.ReadDir(h.Project); len(ents) != 0 {
		t.Fatalf("snippet created files: %v", ents)
	}
	if _, _, code := runCLI(t, "snippet", "vim"); code != ExitUsage {
		t.Fatalf("unknown target: exit %d", code)
	}
}
