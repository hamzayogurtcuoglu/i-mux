package cli

import (
	"os"
	"reflect"
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
