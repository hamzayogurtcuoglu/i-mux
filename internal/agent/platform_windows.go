//go:build windows

package agent

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// Agents on Windows (ARCHITECTURE.md §7.9, D85): each installs with its own maker's Windows
// command, into a folder of its own that a new install adds to PATH only for new terminals.

// windowsInfo is what changes on Windows: the install command (run by PowerShell) and where it
// lands, as folders under %USERPROFILE%, %LOCALAPPDATA% or %APPDATA%.
var windowsInfo = map[session.AgentID]struct {
	install string
	dirs    []string // "<VAR>\rest": an environment variable's folder, then the rest
}{
	session.Claude:      {"irm https://claude.ai/install.ps1 | iex", []string{`USERPROFILE\.local\bin`}},
	session.Antigravity: {"irm https://antigravity.google/cli/install.ps1 | iex", []string{`LOCALAPPDATA\agy\bin`}},
	session.Copilot:     {"winget install --id GitHub.Copilot -e", []string{`LOCALAPPDATA\Microsoft\WinGet\Links`}},
	session.Codex:       {"winget install --id OpenAI.Codex -e", []string{`LOCALAPPDATA\Microsoft\WinGet\Links`}},
	// OpenCode has no Windows installer of its own (§7.9).
	session.OpenCode: {"npm i -g @opencode/cli@latest", nil},
}

// npmDir is where npm puts a global CLI's shims, for every agent installed with npm.
const npmDir = `APPDATA\npm`

// platformInfo gives an agent its Windows install command.
func platformInfo(a *Info) {
	if w, ok := windowsInfo[a.ID]; ok {
		a.InstallHint, a.InstallCmd = w.install, w.install
	}
}

// installDirs are the folders the agent's install puts it in, then npm's.
func installDirs(info Info) []string {
	var dirs []string
	for _, d := range append(append([]string(nil), windowsInfo[info.ID].dirs...), npmDir) {
		v, rest, _ := strings.Cut(d, `\`)
		if base := os.Getenv(v); base != "" {
			dirs = append(dirs, filepath.Join(base, rest))
		}
	}
	return dirs
}

// programFiles are the files name may be in dir: as given when it has an extension, else
// with each of PATHEXT's (claude.exe, opencode.cmd).
func programFiles(dir, name string) []string {
	if filepath.Ext(name) != "" {
		return []string{filepath.Join(dir, name)}
	}
	exts := os.Getenv("PATHEXT")
	if exts == "" {
		exts = ".COM;.EXE;.BAT;.CMD"
	}
	var out []string
	for _, e := range strings.Split(exts, ";") {
		if e != "" {
			out = append(out, filepath.Join(dir, name+strings.ToLower(e)))
		}
	}
	return out
}
