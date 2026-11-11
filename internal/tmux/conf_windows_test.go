//go:build windows

package tmux

import (
	"regexp"
	"strings"
	"testing"
)

// fixedPowerShell makes PowerShell() return a fixed path for the test.
func fixedPowerShell(t *testing.T, path string) {
	t.Helper()
	old := powerShell
	powerShell = func() string { return path }
	t.Cleanup(func() { powerShell = old })
}

// TestGenerateConfPsmux checks what the Windows config does differently
// from tmux's (whose golden files conf_unix_test.go compares): PowerShell
// command lines, ids that name their session, and psmux's pane titles.
func TestGenerateConfPsmux(t *testing.T) {
	fixedPowerShell(t, `C:\PS\powershell.exe`)
	got := GenerateConf(confInput("3.4"))
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	if !strings.Contains(got, "\nset -g  default-shell 'C:\\PS\\powershell.exe'\n") {
		t.Errorf("default-shell is not PowerShell:\n%s", got)
	}
	// Every command imux runs is a PowerShell call of imux, silenced.
	runs := 0
	for _, l := range lines {
		if !strings.Contains(l, "run-shell") {
			continue
		}
		runs++
		if !strings.Contains(l, "& /opt/imux/bin/imux ") || !strings.Contains(l, Silent) {
			t.Errorf("run-shell is not a silenced PowerShell call: %s", l)
		}
		if strings.Contains(l, "/dev/null") || strings.Contains(l, "|| true") {
			t.Errorf("sh syntax in a PowerShell command: %s", l)
		}
	}
	if runs == 0 {
		t.Fatal("no run-shell commands")
	}
	// Pane, window and client ids expand with their session.
	bare := regexp.MustCompile(`#\{(pane_id|window_id)\}([^~]|$)|#\{client_name\}([^~]|$)|#D`)
	for _, l := range lines {
		if m := bare.FindString(l); m != "" {
			t.Errorf("id without its session (%q): %s", m, l)
		}
	}
	if !strings.Contains(got, "--pane #{pane_id}~#{session_name} --client #{client_name}~#{client_session}") {
		t.Error("menu binding does not pass session-qualified ids")
	}
	// psmux splits a block's commands only at " ; " (select-pane -t=; send
	// -M took every click on a pane with it).
	for _, l := range lines {
		if strings.HasPrefix(l, "bind -n MouseDown1Pane ") && !strings.Contains(l, "select-pane -t= ; send -M }") {
			t.Errorf("press binding: %s", l)
		}
	}
	// psmux shows #{pane_title} only: the last pane-border-format wins.
	var border string
	for _, l := range lines {
		if strings.HasPrefix(l, "set -g  pane-border-format ") {
			border = l
		}
	}
	if border != "set -g  pane-border-format ' #{pane_title} '" {
		t.Errorf("last pane-border-format %q", border)
	}
	if !strings.Contains(got, "\nset -g  allow-set-title off\n") {
		t.Error("allow-set-title not off")
	}
	// The user's local config is still sourced last.
	if last := lines[len(lines)-1]; last != "source-file -q /home/test/.imux/tmux.local.conf" {
		t.Errorf("last line %q", last)
	}
}
