// Package versions lists what i-mux runs on, with the versions installed
// and the command that updates each: i-mux, tmux and the agent CLIs
// (Settings → Versions and updates, `imux versions`). It asks nothing of
// the network: each CLI's own updater knows whether a newer one is out.
package versions

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/update"
)

// Status says how a row stands.
type Status int

// Statuses.
const (
	OK      Status = iota
	Old            // works; a newer version gives more
	Missing        // not installed
	Unknown        // installed, its version could not be read
)

// Row is one thing i-mux runs on.
type Row struct {
	Key     string // the letter that runs Fix in the popup
	Name    string
	Version string // "" when missing or unreadable
	Status  Status
	Note    string          // why it is old, or what to do (lines)
	Fix     []string        // the command that updates or installs it; nil: none to run
	Show    string          // Fix as the user would type it
	Tmux    bool            // the tmux row (its update needs a restart of i-mux's server)
	Agent   session.AgentID // an agent's row ("" for i-mux and tmux)
}

// Env is what Collect looks at; tests replace every part.
type Env struct {
	Cfg      *config.Config
	Self     string // the imux binary
	Current  string // this i-mux's version
	Latest   string // the newest release seen ("" when unknown)
	Root     bool   // running as root: no sudo
	LookPath func(string) (string, error)
	// Output runs a short command and returns its stdout ("" on failure).
	Output  func(ctx context.Context, argv ...string) string
	Resolve func(id session.AgentID) agent.Resolved
	Probe   func(r agent.Resolved) string // the agent's version ("" unknown)
}

// TmuxWants is the tmux that gives every feature: the agent bar takes
// clicks. Menus take the mouse on every tmux (D54).
var TmuxWants = tmux.ClickMinVersion

// Collect reads every row: i-mux, tmux, then the agents in config order.
func Collect(ctx context.Context, e Env) []Row {
	rows := []Row{imuxRow(e), engineRow(ctx, e)}
	agents := agent.Ordered(e.Cfg)
	out := make([]Row, len(agents))
	var wg sync.WaitGroup
	for i, a := range agents {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = agentRow(e, a)
		}()
	}
	wg.Wait()
	return append(rows, out...)
}

func imuxRow(e Env) Row {
	r := Row{Key: "u", Name: "i-mux", Version: strings.TrimPrefix(e.Current, "v"), Fix: []string{e.Self, "update"}, Show: "imux update"}
	switch {
	case update.Newer(e.Latest, e.Current):
		r.Status, r.Note = Old, i18n.T("versions.imux_new", strings.TrimPrefix(e.Latest, "v"))
	case e.Latest != "":
		r.Note, r.Fix, r.Show = i18n.T("versions.newest"), nil, ""
	}
	return r
}

// engineRow is tmux's row where there is no tmux (Windows, D85): the engine, i-mux-core, which
// comes and updates with i-mux, so no package manager is offered (§7.9).
func engineRow(ctx context.Context, e Env) Row {
	r := Row{Key: "t", Name: "i-mux-core", Tmux: true}
	path := tmux.EngineBin()
	if !filepath.IsAbs(path) {
		r.Status, r.Note = Missing, i18n.T("versions.engine_missing")
		return r
	}
	v, ok := tmux.ParseVersion(e.Output(ctx, path, "-V"))
	if !ok || v.EngineVersion() == "" {
		r.Status, r.Note = Unknown, i18n.T("versions.unknown")
		return r
	}
	r.Version = v.EngineVersion()
	return r
}

// fromWinGet reports whether a binary's path is WinGet's (its Links folder or a package it
// installed), as Codex's WinGet install is on Windows (§7.9).
func fromWinGet(path string) bool {
	return strings.Contains(strings.ToLower(filepath.ToSlash(path)), "/microsoft/winget/")
}

// fromBrew reports whether a binary's real path is in a Homebrew prefix.
func fromBrew(real string) bool {
	for _, p := range []string{"/Cellar/", "/homebrew/", "/linuxbrew/"} {
		if strings.Contains(real, p) {
			return true
		}
	}
	return false
}

func agentRow(e Env, a agent.Info) Row {
	r := Row{Key: a.Key, Name: a.Name, Agent: a.ID}
	res := e.Resolve(a.ID)
	if !res.Installed {
		// The command Install now runs (D41), run by its letter (D79);
		// without the program it starts with, how to get that.
		cmd := agent.Install(e.Cfg, a.ID)
		r.Status, r.Note, r.Show = Missing, i18n.T("versions.missing"), cmd
		if p := needs(e, cmd); p != "" {
			r.Note += "\n" + i18n.T("install.needs", a.Short, p) + "\n" + howTo(p)
		} else {
			r.Fix = sysx.InstallShell(cmd) // sh -c, or Windows PowerShell (§7.9)
		}
		return r
	}
	r.Version = e.Probe(res)
	if r.Version == "" {
		r.Status = Unknown
	}
	r.Fix, r.Show = agentUpdate(a, res.Path, r.Version)
	return r
}

// needs is the program an install command starts with when it is not here
// (npm, curl, brew), as Install now checks (D41); "" otherwise.
func needs(e Env, cmd string) string {
	f := strings.Fields(cmd)
	if len(f) == 0 {
		return ""
	}
	switch f[0] {
	case "npm", "curl", "brew", "winget":
		if _, err := e.LookPath(f[0]); err != nil {
			return f[0]
		}
	}
	return ""
}

// howTo says how to get a program an install command needs.
func howTo(tool string) string {
	switch tool {
	case "npm":
		return i18n.T("install.needs_npm")
	case "curl":
		return i18n.T("install.needs_curl")
	case "brew":
		return i18n.T("install.needs_brew")
	case "winget":
		return i18n.T("install.needs_winget")
	}
	return ""
}

// agentUpdate is the command that updates agent a installed at path: its
// own updater, or for Codex the package manager that installed it.
func agentUpdate(a agent.Info, path, version string) ([]string, string) {
	args := a.UpdateArgs
	if a.ID == session.OpenCode && strings.HasPrefix(version, "1.") {
		args = []string{"upgrade", "--major"} // i-mux needs OpenCode 2
	}
	if args != nil {
		return append([]string{path}, args...), a.Bin + " " + strings.Join(args, " ")
	}
	if a.ID != session.Codex {
		return nil, ""
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		real = path
	}
	if fromBrew(real) {
		return []string{"brew", "upgrade", "codex"}, "brew upgrade codex"
	}
	if fromWinGet(real) {
		cmd := []string{"winget", "upgrade", "--id", "OpenAI.Codex", "-e"}
		return cmd, shellText(cmd)
	}
	cmd := []string{"npm", "install", "-g", "@openai/codex@latest"}
	return cmd, shellText(cmd)
}

// pad fills s with spaces to w columns (names and versions are ASCII).
func pad(s string, w int) string { return s + strings.Repeat(" ", max(w-len([]rune(s)), 0)) }

// shellText is argv as one would type it.
func shellText(argv []string) string { return strings.Join(argv, " ") }

// Format draws the rows: key, name, version, then what is wrong and the
// command, aligned.
func Format(rows []Row) string {
	nameW, verW := 0, 0
	for _, r := range rows {
		nameW = max(nameW, len(r.Name))
		verW = max(verW, len(r.Version), 1)
	}
	var b strings.Builder
	indent := strings.Repeat(" ", 2+1+2+nameW+2+verW+2)
	for _, r := range rows {
		ver := r.Version
		if ver == "" {
			ver = "–"
		}
		mark := "✓"
		switch r.Status {
		case Old:
			mark = "⚠"
		case Missing:
			mark = "✗"
		case Unknown:
			mark = "?"
		}
		// What is wrong first, then the command; an agent that works
		// shows only its command: whether a newer one is out, its own
		// updater knows.
		var lines []string
		for i, n := range strings.Split(r.Note, "\n") {
			if n == "" {
				continue
			}
			if i == 0 {
				n = mark + " " + n
			}
			lines = append(lines, n)
		}
		if r.Show != "" {
			k := "versions.update"
			if r.Status == Missing {
				k = "versions.install_cmd"
			}
			if cmd := i18n.T(k, r.Show); r.Status == Missing && len(lines) == 1 {
				lines[0] += " · " + cmd // "✗ not installed · install: …"
			} else {
				lines = append(lines, cmd)
			}
		}
		if len(lines) == 0 {
			lines = []string{mark}
		}
		b.WriteString("  " + r.Key + "  " + pad(r.Name, nameW) + "  " + pad(ver, verW) + "  " + lines[0] + "\n")
		for _, l := range lines[1:] {
			b.WriteString(indent + l + "\n")
		}
	}
	return b.String()
}
