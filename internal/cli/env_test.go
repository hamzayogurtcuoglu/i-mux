package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
