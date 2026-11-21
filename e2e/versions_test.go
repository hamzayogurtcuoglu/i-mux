//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestVersions (T38.2): `imux versions` lists i-mux, its engine and the agents with
// their versions and update commands; in the popup (Settings → Versions and
// updates) a row's letter runs its update there, and the list is read again.
func TestVersions(t *testing.T) {
	h := newH(t, "")
	out := h.mustImux("versions")
	t.Logf("\n%s", out)
	for _, want := range []string{"i-mux", "i-mux-core", "Claude Code", "9.9.9-fake", "update: claude update", "update: agy update", "update: opencode upgrade"} {
		if !strings.Contains(out, want) {
			t.Errorf("versions lacks %q", want)
		}
	}

	// c runs Claude's updater; a letter with nothing to run says so; Enter
	// alone closes.
	cmd := exec.Command(h.Bin, "versions", "--popup")
	cmd.Dir = h.Homes.Project
	cmd.Stdin = strings.NewReader("c\nz\n\n")
	b, err := cmd.CombinedOutput()
	got := string(b)
	if err != nil {
		t.Fatalf("versions --popup: %v\n%s", err, got)
	}
	for _, want := range []string{"$ claude update", "claude is up to date", "Done.", `Nothing to run for "z".`} {
		if !strings.Contains(got, want) {
			t.Errorf("popup lacks %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "i-mux · Versions and updates"); n != 2 {
		t.Errorf("the list was shown %d times, want 2 (again after the update)", n)
	}
	log, _ := os.ReadFile(filepath.Join(h.Log, "updates"))
	if strings.TrimSpace(string(log)) != "claude update" {
		t.Errorf("updates run: %q", log)
	}
}

// TestVersionsInstall (T84.3, D79): a missing agent's row shows the command
// that installs it, and in the popup its letter runs it there; the list is
// read again, and an agent still not found afterwards says so.
func TestVersionsInstall(t *testing.T) {
	h := newH(t, "[agents.codex]\nbin = \"/nonexistent/codex\"\ninstall = 'echo installing codex > \"$IMUX_FAKE_LOG/codex-install\"'\n")
	if out := h.mustImux("versions"); !strings.Contains(out, "✗ not installed · install: echo installing codex") {
		t.Fatalf("the missing agent's row:\n%s", out)
	}
	cmd := exec.Command(h.Bin, "versions", "--popup")
	cmd.Dir = h.Homes.Project
	cmd.Stdin = strings.NewReader("x\n\n")
	b, err := cmd.CombinedOutput()
	got := string(b)
	if err != nil {
		t.Fatalf("versions --popup: %v\n%s", err, got)
	}
	for _, want := range []string{"$ echo installing codex", "Done.", "codex is not on the PATH i-mux uses"} {
		if !strings.Contains(got, want) {
			t.Errorf("popup lacks %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "i-mux · Versions and updates"); n != 2 {
		t.Errorf("the list was shown %d times, want 2 (again after the install)", n)
	}
	if b, _ := os.ReadFile(filepath.Join(h.Log, "codex-install")); strings.TrimSpace(string(b)) != "installing codex" {
		t.Errorf("the install command did not run: %q", b)
	}
}
