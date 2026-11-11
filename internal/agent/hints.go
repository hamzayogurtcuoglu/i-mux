package agent

import (
	"runtime"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// WindowsInstallHints are the install commands on native Windows (typed in
// PowerShell); the registry's hints are macOS's and Linux's. Antigravity
// and Claude Code ship PowerShell installers, the others npm packages.
var WindowsInstallHints = map[session.AgentID]string{
	session.Claude:      "irm https://claude.ai/install.ps1 | iex (or: npm i -g @anthropic-ai/claude-code)",
	session.Antigravity: "irm https://antigravity.google/cli/install.ps1 | iex",
	session.Copilot:     "npm i -g @github/copilot",
	session.Codex:       "npm i -g @openai/codex",
	session.OpenCode:    "npm i -g opencode-ai (then, if it is OpenCode 1: opencode upgrade --major)",
}

// WindowsInstallCmds are the commands the install popup runs on Windows
// where the hint is more than one (the others run their hint).
var WindowsInstallCmds = map[session.AgentID]string{
	session.Claude:   "irm https://claude.ai/install.ps1 | iex",
	session.OpenCode: "npm i -g opencode-ai",
}

// installHints returns the registry with the install hints and commands for
// goos.
func installHints(reg []Info, goos string) []Info {
	if goos != "windows" {
		return reg
	}
	out := make([]Info, len(reg))
	for i, a := range reg {
		if h, ok := WindowsInstallHints[a.ID]; ok {
			a.InstallHint = h
			a.InstallCmd = WindowsInstallCmds[a.ID]
		}
		out[i] = a
	}
	return out
}

func init() { registry = installHints(registry, runtime.GOOS) }
