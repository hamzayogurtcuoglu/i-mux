package versions

import (
	"context"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

const wingetPsmux = `C:\Users\me\AppData\Local\Microsoft\WinGet\Packages\marlocarlo.psmux_Microsoft.Winget.Source_8wekyb3d8bbwe\psmux.exe`

// TestPsmuxRow: on Windows the multiplexer row is psmux, shown as psmux's
// own version; its update is winget's (scoop's, cargo's), to type outside
// i-mux since Windows cannot replace a running psmux.exe; when missing,
// winget installs it from the popup.
func TestPsmuxRow(t *testing.T) {
	i18n.SetLocale("en")
	old := i18n.OS
	defer func() { i18n.OS = old }()
	i18n.OS = "windows"
	ctx := context.Background()
	psmuxV := "tmux 3.4.0\r\npsmux 3.4.0 (20aa1d7 2026-10-13)\r\n"
	for _, c := range []struct {
		name    string
		mux     string
		bins    map[string]string
		outputs map[string]string
		status  Status
		version string
		fix     string
		show    string
		note    string
	}{
		{
			name: "from winget",
			mux:  wingetPsmux, bins: map[string]string{wingetPsmux: wingetPsmux, "winget": `C:\winget.exe`},
			outputs: map[string]string{wingetPsmux + " -V": psmuxV},
			status:  OK, version: "3.4.0",
			show: "psmux -L imux kill-server; winget upgrade --id marlocarlo.psmux -e", note: "cannot replace a running psmux.exe",
		},
		{
			name: "from scoop, on PATH",
			mux:  "psmux", bins: map[string]string{"psmux": `C:\Users\me\scoop\shims\psmux.exe`, "winget": `C:\winget.exe`},
			outputs: map[string]string{`C:\Users\me\scoop\shims\psmux.exe -V`: psmuxV},
			status:  OK, version: "3.4.0", show: "psmux -L imux kill-server; scoop update psmux",
		},
		{
			name: "too old",
			mux:  wingetPsmux, bins: map[string]string{wingetPsmux: wingetPsmux, "winget": `C:\winget.exe`},
			outputs: map[string]string{wingetPsmux + " -V": "tmux 3.1\npsmux 0.9.0 (x)\n"},
			status:  Old, version: "0.9.0", show: "psmux -L imux kill-server; winget upgrade --id marlocarlo.psmux -e", note: "matches tmux 3.2",
		},
		{
			name: "missing, winget installs it",
			mux:  "psmux", bins: map[string]string{"winget": `C:\winget.exe`},
			status: Missing, fix: "winget install --id marlocarlo.psmux -e", show: "winget install --id marlocarlo.psmux -e", note: "not installed",
		},
		{
			name:   "missing, no winget",
			mux:    "psmux",
			status: Missing, note: "install psmux from https://github.com/marlocarlo/psmux",
		},
	} {
		e := fakeEnv(c.bins, c.outputs, nil)
		e.OS, e.Mux = "windows", c.mux
		r := tmuxRow(ctx, e)
		if r.Name != "psmux" || !r.Tmux || r.Key != "t" || r.Status != c.status || r.Version != c.version ||
			strings.Join(r.Fix, " ") != c.fix || r.Show != c.show || !strings.Contains(r.Note, c.note) {
			t.Errorf("%s: %+v", c.name, r)
		}
	}
}

// TestWindowsRows: the agents' updates are their own updaters, or npm for
// Codex; the Windows install hints come with the agent registry.
func TestWindowsRows(t *testing.T) {
	i18n.SetLocale("en")
	e := fakeEnv(map[string]string{"winget": `C:\winget.exe`}, nil, map[session.AgentID]string{"claude": "2.1.319", "codex": "0.50.0"})
	e.OS, e.Mux = "windows", "psmux"
	rows := Collect(context.Background(), e)
	if rows[1].Name != "psmux" || rows[1].Status != Missing {
		t.Fatalf("psmux row %+v", rows[1])
	}
	if r := row(t, rows, "Codex CLI"); r.Show != "npm install -g @openai/codex@latest" {
		t.Errorf("codex %+v", r)
	}
	if r := row(t, rows, "Claude Code"); r.Show != "claude update" {
		t.Errorf("claude %+v", r)
	}
}
