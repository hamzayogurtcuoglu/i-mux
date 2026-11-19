//go:build windows

package agent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// On Windows an agent installs with its maker's Windows command, and is found in the folder that
// command puts it in, or npm's, before a new terminal has it on PATH (§7.9).
func TestWindowsInstallAndFolders(t *testing.T) {
	cfg := config.Default()
	for id, want := range map[session.AgentID]string{
		session.Claude:   "irm https://claude.ai/install.ps1 | iex",
		session.Copilot:  "winget install --id GitHub.Copilot -e",
		session.OpenCode: "npm i -g @opencode/cli@latest",
	} {
		if got := Install(cfg, id); got != want {
			t.Errorf("%s installs with %q, want %q", id, got, want)
		}
	}

	local, roaming := t.TempDir(), t.TempDir()
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("APPDATA", roaming)
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	old := LookPathFunc
	LookPathFunc = func(string) (string, error) { return "", errors.New("not on PATH") }
	t.Cleanup(func() { LookPathFunc = old })

	agy := filepath.Join(local, "agy", "bin", "agy.exe")
	shim := filepath.Join(roaming, "npm", "opencode.cmd")
	for _, p := range []string{agy, shim} {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for id, want := range map[session.AgentID]string{session.Antigravity: agy, session.OpenCode: shim} {
		if r := Resolve(cfg, id, nil); !r.Installed || r.Path != want {
			t.Errorf("%s: %+v, want %s", id, r, want)
		}
	}
	if r := Resolve(cfg, session.Codex, nil); r.Installed {
		t.Errorf("codex found: %+v", r)
	}
}
