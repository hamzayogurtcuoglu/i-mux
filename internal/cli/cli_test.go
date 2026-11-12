package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

func runCLI(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	var o, e bytes.Buffer
	code = Run(&Env{Stdin: strings.NewReader(""), Stdout: &o, Stderr: &e}, args)
	return o.String(), e.String(), code
}

func TestVersion(t *testing.T) {
	out, _, code := runCLI(t, "version")
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	if !strings.HasPrefix(out, "imux ") || !strings.Contains(out, "commit") {
		t.Fatalf("unexpected version output %q", out)
	}
}

func TestHelpListsCommands(t *testing.T) {
	out, _, code := runCLI(t, "help")
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	for name, c := range commands {
		if c.Hidden {
			if strings.Contains(out, "  "+name+" ") {
				t.Errorf("hidden command %s listed", name)
			}
			continue
		}
		if !strings.Contains(out, name) {
			t.Errorf("help does not list %s", name)
		}
	}
}

func TestUnknownCommandExit2(t *testing.T) {
	_, errOut, code := runCLI(t, "definitely-not-a-command")
	if code != ExitUsage {
		t.Fatalf("exit %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(errOut, "unknown command") || !strings.Contains(errOut, "Commands:") {
		t.Fatalf("stderr should contain the error and the help: %q", errOut)
	}
}

func TestConfigCommands(t *testing.T) {
	home := t.TempDir()
	t.Setenv("IMUX_HOME", home)
	out, _, code := runCLI(t, "config", "path")
	if code != 0 || strings.TrimSpace(out) != home+"/config.toml" {
		t.Fatalf("path: %q %d", out, code)
	}
	if _, _, code := runCLI(t, "config", "init"); code != 0 {
		t.Fatalf("init exit %d", code)
	}
	if _, errOut, code := runCLI(t, "config", "init"); code == 0 || !strings.Contains(errOut, "--force") {
		t.Fatalf("second init should refuse: %d %q", code, errOut)
	}
	if _, _, code := runCLI(t, "config", "init", "--force"); code != 0 {
		t.Fatalf("init --force exit %d", code)
	}
	out, _, code = runCLI(t, "config", "show")
	if code != 0 || !strings.Contains(out, `on_limit = "ask"`) {
		t.Fatalf("show: %d %q", code, out)
	}
}

func TestCommandSummariesInCatalog(t *testing.T) {
	for name, c := range commands {
		if !c.Hidden && !i18n.Has(c.Summary) {
			t.Errorf("command %s: summary key %q missing from the i18n catalog", name, c.Summary)
		}
	}
}
