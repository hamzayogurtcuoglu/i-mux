package agent

import (
	"runtime"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// TestInstallHints: Windows gets PowerShell and npm commands for every
// agent; macOS and Linux keep the registry's (curl, Homebrew, npm).
func TestInstallHints(t *testing.T) {
	unix := installHints(All(), "linux")
	win := installHints(All(), "windows")
	for i, a := range win {
		if a.InstallHint == "" || strings.Contains(a.InstallHint, "curl") || strings.Contains(a.InstallHint, "brew") || strings.Contains(a.InstallHint, "| bash") {
			t.Errorf("%s on Windows: %q", a.ID, a.InstallHint)
		}
		if a.Name == "" || a.ID != unix[i].ID || a.Bin != unix[i].Bin {
			t.Errorf("%s: only the hint may change: %+v", a.ID, a)
		}
		// The install popup runs InstallCmd, else the hint: one PowerShell
		// command, without the hints' "(or: ...)".
		run := a.InstallCmd
		if run == "" {
			run = a.InstallHint
		}
		if strings.ContainsAny(run, "()") || strings.Contains(run, "curl") || strings.Contains(run, "| bash") {
			t.Errorf("%s runs %q on Windows", a.ID, run)
		}
	}
	if h := WindowsInstallHints[session.Antigravity]; h != "irm https://antigravity.google/cli/install.ps1 | iex" {
		t.Errorf("antigravity %q", h)
	}
	if runtime.GOOS != "windows" {
		if h := MustGet(session.Antigravity).InstallHint; h != "curl -fsSL https://antigravity.google/cli/install.sh | bash" {
			t.Errorf("the macOS/Linux hint changed: %q", h)
		}
	} else if h := MustGet(session.Claude).InstallHint; !strings.HasPrefix(h, "irm https://claude.ai/install.ps1") {
		t.Errorf("windows claude hint %q", h)
	}
}
