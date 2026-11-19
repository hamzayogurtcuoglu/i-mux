package session

import (
	"path/filepath"
	"testing"
)

// On Windows every agent's home is under %USERPROFILE%, as each agent's
// own documentation or source says (§7.8), and HOME (Git Bash's) is not
// the home.
func TestResolveWindowsHomes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", `/c/Users/other`)
	for _, v := range []string{"CLAUDE_CONFIG_DIR", "COPILOT_HOME", "CODEX_HOME", "XDG_DATA_HOME"} {
		t.Setenv(v, "")
	}
	want := map[AgentID]string{
		Claude:      filepath.Join(home, ".claude"),
		Codex:       filepath.Join(home, ".codex"),
		Copilot:     filepath.Join(home, ".copilot"),
		Antigravity: filepath.Join(home, ".gemini", "antigravity-cli"),
		OpenCode:    filepath.Join(home, ".local", "share", "opencode"),
	}
	var e Env
	for id, w := range want {
		if got := e.Resolve(id); got != w {
			t.Errorf("Resolve(%s) = %q, want %q", id, got, w)
		}
	}
	t.Setenv("CODEX_HOME", `D:\codex`)
	if got := e.Resolve(Codex); got != `D:\codex` {
		t.Errorf("CODEX_HOME: %q", got)
	}
}
