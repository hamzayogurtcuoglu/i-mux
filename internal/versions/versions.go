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
	Note    string   // why it is old, or what to do (lines)
	Fix     []string // the command that updates or installs it; nil: none to run
	Show    string   // Fix as the user would type it
	Tmux    bool     // the tmux row (its update needs a restart of i-mux's server)
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
	rows := []Row{imuxRow(e), tmuxRow(ctx, e)}
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

func tmuxRow(ctx context.Context, e Env) Row {
	r := Row{Key: "t", Name: "tmux", Tmux: true}
	path, err := e.LookPath("tmux")
	if err != nil {
		r.Status, r.Note = Missing, i18n.T("versions.tmux_missing")
		r.Fix = pmCmd(e, false)
		if r.Fix == nil {
			r.Note += "\n" + i18n.T("versions.tmux_pm")
		}
		r.Show = shellText(r.Fix)
		return r
	}
	v, ok := tmux.ParseVersion(e.Output(ctx, path, "-V"))
	if !ok {
		r.Status, r.Note = Unknown, i18n.T("versions.unknown")
		return r
	}
	r.Version = v.String()
	switch {
	case !v.AtLeast(tmux.MinVersion[0], tmux.MinVersion[1]):
		r.Status, r.Note = Old, i18n.T("versions.tmux_too_old")
	case !v.AtLeast(TmuxWants[0], TmuxWants[1]):
		r.Status, r.Note = Old, i18n.T("versions.tmux_34")
	default:
		return r
	}
	if brewTmux(e, path) {
		r.Fix = []string{"brew", "upgrade", "tmux"}
	} else if _, err := e.LookPath("apt-get"); err == nil {
		// apt gives the release's tmux (Ubuntu 22.04: 3.2a): say so rather
		// than offer an upgrade that changes nothing.
		inst, cand := aptPolicy(e.Output(ctx, "env", "LC_ALL=C", "apt-cache", "policy", "tmux"))
		if cand != "" && upstream(cand) != upstream(inst) {
			r.Fix = sudo(e, "apt-get", "install", "-y", "--only-upgrade", "tmux")
		} else if _, err := e.LookPath("brew"); err == nil {
			r.Fix = []string{"brew", "install", "tmux"}
		} else {
			r.Note += "\n" + i18n.T("versions.tmux_no_newer", "apt")
		}
	} else {
		r.Fix = pmCmd(e, true)
		if r.Fix == nil {
			r.Note += "\n" + i18n.T("versions.tmux_pm")
		}
	}
	r.Show = shellText(r.Fix)
	return r
}

// brewTmux reports whether tmux at path is Homebrew's.
func brewTmux(e Env, path string) bool {
	if _, err := e.LookPath("brew"); err != nil {
		return false
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		real = path
	}
	return fromBrew(real)
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

// pmCmd is the package manager's command that installs (or, with upgrade,
// updates) tmux, the way install.sh picks it; nil when none is known.
func pmCmd(e Env, upgrade bool) []string {
	pms := []struct {
		bin              string
		install, upgrade []string
	}{
		{"brew", []string{"brew", "install", "tmux"}, []string{"brew", "upgrade", "tmux"}},
		{"apt-get", []string{"apt-get", "install", "-y", "tmux"}, []string{"apt-get", "install", "-y", "--only-upgrade", "tmux"}},
		{"dnf", []string{"dnf", "install", "-y", "tmux"}, []string{"dnf", "upgrade", "-y", "tmux"}},
		{"pacman", []string{"pacman", "-Syu", "--needed", "--noconfirm", "tmux"}, []string{"pacman", "-Syu", "--needed", "--noconfirm", "tmux"}},
		{"zypper", []string{"zypper", "--non-interactive", "install", "tmux"}, []string{"zypper", "--non-interactive", "update", "tmux"}},
		{"apk", []string{"apk", "add", "tmux"}, []string{"apk", "upgrade", "tmux"}},
	}
	for _, pm := range pms {
		if _, err := e.LookPath(pm.bin); err != nil {
			continue
		}
		cmd := pm.install
		if upgrade {
			cmd = pm.upgrade
		}
		if pm.bin == "brew" {
			return cmd
		}
		return sudo(e, cmd...)
	}
	return nil
}

// sudo prefixes argv with sudo unless running as root.
func sudo(e Env, argv ...string) []string {
	if e.Root {
		return argv
	}
	return append([]string{"sudo"}, argv...)
}

// aptPolicy reads the installed and candidate versions from `apt-cache
// policy` ("" when unknown; "(none)" counts as unknown).
func aptPolicy(out string) (installed, candidate string) {
	for _, l := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(l), ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		if v == "(none)" {
			v = ""
		}
		switch k {
		case "Installed":
			installed = v
		case "Candidate":
			candidate = v
		}
	}
	return installed, candidate
}

// upstream is a Debian package version's upstream part: "1:3.2a-4ubuntu0.3"
// is "3.2a" (Ubuntu's patches of one tmux add no feature).
func upstream(v string) string {
	if _, after, ok := strings.Cut(v, ":"); ok {
		v = after
	}
	if i := strings.LastIndex(v, "-"); i > 0 {
		v = v[:i]
	}
	return v
}

func agentRow(e Env, a agent.Info) Row {
	r := Row{Key: a.Key, Name: a.Name}
	res := e.Resolve(a.ID)
	if !res.Installed {
		r.Status, r.Note = Missing, i18n.T("versions.install", a.InstallHint)
		return r
	}
	r.Version = e.Probe(res)
	if r.Version == "" {
		r.Status = Unknown
	}
	r.Fix, r.Show = agentUpdate(a, res.Path, r.Version)
	return r
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
			lines = append(lines, i18n.T(k, r.Show))
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
