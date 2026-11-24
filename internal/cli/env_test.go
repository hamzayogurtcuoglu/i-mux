package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/envfile"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestEnvDialogPopup: the Env view's + opens the New environment dialog in
// a popup in the middle of the client, New skill's size within it, which
// runs `imux env new` for that client; with no name and no terminal,
// `imux env new` is a usage error.
func TestEnvDialogPopup(t *testing.T) {
	testutil.SkipOnWindows(t, "the fake tmux is an sh script")
	testutil.Isolate(t)
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	script := "#!/bin/sh\n" +
		"case \"$*\" in\n" +
		"-V) echo 'tmux 3.4' ;;\n" +
		"*display-message*client_width*) printf '60\\t20\\n' ;;\n" +
		"*) printf '%s\\n' \"$*\" >> " + log + " ;;\n" +
		"esac\n"
	bin := filepath.Join(dir, "tmux")
	testutil.WriteFile(t, bin, []byte(script))
	if err := os.Chmod(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IMUX_TMUX_BIN", bin)
	if _, stderr, code := runCLI(t, "env", "dialog", "--client", "/dev/pts/7"); code != ExitOK {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	got := ""
	for i := 0; i < 250 && !strings.Contains(got, "display-popup"); i++ {
		time.Sleep(20 * time.Millisecond) // the popup's tmux is not waited for
		b, _ := os.ReadFile(log)
		got = string(b)
	}
	if !strings.Contains(got, "display-popup -c /dev/pts/7 -E -w 60 -h 20 -x C -y C ") ||
		!strings.Contains(got, " env new --client /dev/pts/7") {
		t.Fatalf("popup %q", got)
	}
	if _, _, code := runCLI(t, "env", "new"); code != ExitUsage {
		t.Fatalf("env new without a terminal: exit %d", code)
	}
}

// TestEnvFlag (D101): --env of split, tab and worktree new takes an
// environment's name: one that cannot be a name is a usage error before
// anything runs; one with no file is an error that names it.
func TestEnvFlag(t *testing.T) {
	testutil.Isolate(t)
	for _, args := range [][]string{
		{"split", "claude", "--env", "Not A Name"}, {"tab", "shell", "--env", "x/y"}, {"worktree", "new", "fix", "--env", "UPPER"},
	} {
		if _, _, code := runCLI(t, args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want a usage error", args, code)
		}
	}
	if err := checkEnv("glm"); err == nil || !strings.Contains(err.Error(), "glm") {
		t.Errorf("a missing environment: %v", err)
	}
	testutil.WriteFile(t, envfile.Path("glm"), []byte("A=1\n"))
	if err := checkEnv("glm"); err != nil {
		t.Errorf("glm: %v", err)
	}
	if err := checkEnv(""); err != nil {
		t.Errorf("none: %v", err)
	}
}
