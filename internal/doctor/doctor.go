// Package doctor checks imux's environment: tmux, terminfo, the agent CLIs
// (path, version, PATH for nvm installs, flags), ~/.imux permissions, the
// binary's permissions and the watcher. It never starts an agent with a
// prompt: agents are only asked for --version and --help.
package doctor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/clipimage"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Level of a check result.
type Level int

const (
	OK   Level = iota
	Info       // a hint
	Warn       // works, but something should be fixed
	Fail       // imux cannot work
)

// Symbol renders a level.
func (l Level) Symbol() string {
	switch l {
	case OK:
		return "✓"
	case Info:
		return "·"
	case Warn:
		return "!"
	}
	return "✗"
}

// Result is one finding.
type Result struct {
	Level Level
	Msg   string
}

// Report is the outcome of all checks.
type Report struct{ Results []Result }

func (r *Report) add(l Level, key string, args ...any) {
	r.Results = append(r.Results, Result{l, i18n.T(key, args...)})
}

// Failed reports whether any check failed hard.
func (r *Report) Failed() bool {
	for _, x := range r.Results {
		if x.Level == Fail {
			return true
		}
	}
	return false
}

// Options control the checks.
type Options struct {
	Cfg      *config.Config
	Tmux     *tmux.Client
	Self     string // imux binary
	CI       bool   // skip the agent checks (release smoke tests)
	Watcher  func() (running bool, pid int)
	Resolved map[session.AgentID]agent.Resolved // nil = resolve now
	Users    func() []string                    // other human users; nil = /etc/passwd
}

// MinVersions are the oldest agent versions imux's parsers and launch
// templates were verified against; older ones get an "outdated" hint.
var MinVersions = map[session.AgentID]string{
	session.Claude: "2.0.0",
	session.Codex:  "0.38.0",
}

// Run performs every check.
func Run(ctx context.Context, o Options) *Report {
	r := &Report{}
	checkTmux(ctx, r, o)
	checkHome(r)
	checkSelf(r, o.Self)
	if !o.CI {
		checkAgents(ctx, r, o)
	}
	if o.Watcher != nil {
		if running, pid := o.Watcher(); running {
			r.add(OK, "doctor.watcher_running", pid)
		} else {
			r.add(Info, "doctor.watcher_stopped")
		}
	}
	if o.Cfg.Paste.Images && o.Cfg.Paste.Command == "" {
		if src, hint := (&clipimage.Reader{}).Source(); src != "" {
			r.add(OK, "doctor.paste_ok", src)
		} else {
			r.add(Info, "doctor.paste_missing", hint)
		}
	}
	if o.Cfg.Handoff.Inject == "argv" && o.Cfg.Handoff.Deliver == "inline" {
		users := o.Users
		if users == nil {
			users = otherUsers
		}
		if others := users(); len(others) > 0 {
			r.add(Warn, "doctor.argv_exposure", strings.Join(others, ", "))
		}
	}
	keyHints(r, o.Cfg)
	return r
}

func checkTmux(ctx context.Context, r *Report, o Options) {
	v, err := o.Tmux.BinVersion(ctx)
	if errors.Is(err, tmux.ErrNotFound) {
		r.add(Fail, "doctor.tmux_missing")
		return
	}
	if err != nil {
		r.add(Fail, "doctor.tmux_error", err)
		return
	}
	path, _ := exec.LookPath(o.Tmux.Bin)
	if !v.AtLeast(tmux.MinVersion[0], tmux.MinVersion[1]) {
		r.add(Fail, "doctor.tmux_old", v.String(), fmt.Sprintf("%d.%d", tmux.MinVersion[0], tmux.MinVersion[1]))
		return
	}
	r.add(OK, "doctor.tmux_ok", v.String(), path)
	if !v.AtLeast(tmux.ClickMinVersion[0], tmux.ClickMinVersion[1]) {
		r.add(Info, "doctor.tmux_no_click", v.String(), fmt.Sprintf("%d.%d", tmux.ClickMinVersion[0], tmux.ClickMinVersion[1]))
	}
	if tmux.TerminfoExists("tmux-256color") {
		r.add(OK, "doctor.terminfo_ok")
	} else {
		r.add(Warn, "doctor.terminfo_missing")
	}
}

func checkHome(r *Report) {
	home := paths.Home()
	fi, err := os.Stat(home)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		r.add(Info, "doctor.home_missing", paths.Shorten(home))
		return
	case err != nil:
		r.add(Fail, "doctor.home_error", paths.Shorten(home), err)
		return
	case fi.Mode().Perm()&0o077 != 0:
		r.add(Warn, "doctor.home_perm", paths.Shorten(home), fmt.Sprintf("%#o", fi.Mode().Perm()))
	default:
		r.add(OK, "doctor.home_ok", paths.Shorten(home))
	}
	var loose []string
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == home {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 || d.Name() == "tmux.local.conf" {
			return nil
		}
		info, err := d.Info()
		if err == nil && info.Mode().Perm()&0o077 != 0 {
			rel, _ := filepath.Rel(home, p)
			loose = append(loose, rel)
		}
		if d.IsDir() && len(loose) > 20 {
			return filepath.SkipDir
		}
		return nil
	})
	if len(loose) > 0 {
		if len(loose) > 5 {
			loose = append(loose[:5], "…")
		}
		r.add(Warn, "doctor.files_perm", strings.Join(loose, ", "))
	}
	tmp, err := os.CreateTemp(home, ".doctor-*")
	if err != nil {
		r.add(Fail, "doctor.home_unwritable", paths.Shorten(home), err)
		return
	}
	tmp.Close()
	os.Remove(tmp.Name())
}

// checkSelf warns when the binary or its directory is writable by others:
// the generated tmux.conf runs it from key bindings and hooks.
func checkSelf(r *Report, self string) {
	if self == "" {
		return
	}
	for _, p := range []string{self, filepath.Dir(self)} {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		if m := fi.Mode(); m.Perm()&0o022 != 0 && m&os.ModeSticky == 0 {
			r.add(Warn, "doctor.self_writable", p)
			return
		}
	}
}

func checkAgents(ctx context.Context, r *Report, o Options) {
	res := o.Resolved
	if res == nil {
		res = agent.ResolveAll(o.Cfg, nil)
	}
	installed := 0
	for _, a := range agent.Ordered(o.Cfg) {
		x := res[a.ID]
		if !x.Installed {
			r.add(Info, "doctor.agent_missing", a.Name, a.InstallHint)
			continue
		}
		installed++
		if a.ID == session.OpenCode && !checkOpenCode(r) {
			continue // it cannot start: probing it would only add noise
		}
		ver := x.Version
		if ver == "" {
			ver = agent.ProbeVersion(x.Path, x.PathEnv)
		}
		if ver == "" {
			r.add(Warn, "doctor.agent_noversion", a.Name, x.Path)
		} else {
			r.add(OK, "doctor.agent_ok", a.Name, ver, paths.Shorten(x.Path))
			if min, ok := MinVersions[a.ID]; ok && compareVersions(ver, min) < 0 {
				r.add(Warn, "doctor.agent_outdated", a.Name, ver, min, a.InstallHint)
			}
		}
		if needsNode(x.Path) && !hasNode(x) {
			r.add(Warn, "doctor.agent_node", a.Name, filepath.Dir(x.Path))
		}
		checkFlags(ctx, r, o.Cfg, a, x)
	}
	if installed == 0 {
		r.add(Fail, "doctor.no_agents")
	}
}

// needsNode reports whether path is a `#!/usr/bin/env node` script.
func needsNode(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	line, _ := bufio.NewReader(f).ReadString('\n')
	return strings.HasPrefix(line, "#!") && strings.Contains(line, "node")
}

func hasNode(x agent.Resolved) bool {
	if _, err := os.Stat(filepath.Join(filepath.Dir(x.Path), "node")); err == nil {
		return true
	}
	for _, d := range filepath.SplitList(x.PathEnv) {
		if fi, err := os.Stat(filepath.Join(d, "node")); err == nil && !fi.IsDir() {
			return true
		}
	}
	return false
}

// checkFlags runs `<agent> --help` (never with a prompt) and checks that the
// flags used by the launch templates appear in it.
func checkFlags(ctx context.Context, r *Report, cfg *config.Config, a agent.Info, x agent.Resolved) {
	help := string(agent.Probe(ctx, x.Path, x.PathEnv, true, "--help"))
	if strings.TrimSpace(help) == "" {
		return
	}
	ac := cfg.Agent(string(a.ID))
	var missing []string
	seen := map[string]bool{}
	for _, tpl := range [][]string{ac.Args, ac.NewArgs, ac.ResumeArgs} {
		for _, f := range tpl {
			if !strings.HasPrefix(f, "-") || seen[f] {
				continue
			}
			seen[f] = true
			flag, _, _ := strings.Cut(f, "=")
			if !strings.Contains(help, flag) {
				missing = append(missing, flag)
			}
		}
	}
	if len(missing) > 0 {
		r.add(Warn, "doctor.agent_flags", a.Name, strings.Join(missing, " "))
	}
}

// compareVersions compares dotted numeric versions ("1.0.110" < "2.0.0").
func compareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionParts(v string) [3]int {
	var out [3]int
	v, _, _ = strings.Cut(v, "-")
	for i, p := range strings.SplitN(v, ".", 3) {
		n, _ := strconv.Atoi(strings.TrimFunc(p, func(r rune) bool { return r < '0' || r > '9' }))
		out[i] = n
	}
	return out
}

// otherUsers lists other human accounts (uid >= 1000 with a login shell),
// for the argv-exposure warning.
func otherUsers() []string {
	b, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return nil
	}
	me := ""
	if u, err := user.Current(); err == nil {
		me = u.Username
	}
	var out []string
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Split(line, ":")
		if len(f) < 7 || f[0] == me || f[0] == "nobody" {
			continue
		}
		uid, err := strconv.Atoi(f[2])
		if err != nil || uid < 1000 || uid >= 60000 {
			continue
		}
		if sh := f[6]; strings.HasSuffix(sh, "nologin") || strings.HasSuffix(sh, "false") || sh == "" {
			continue
		}
		out = append(out, f[0])
	}
	return out
}

// keyHints explains how to reach the menu.
func keyHints(r *Report, cfg *config.Config) {
	if k := cfg.Tmux.MenuKeyRoot; k != "" {
		r.add(Info, "doctor.key_root", k)
	}
	if k := cfg.Tmux.MenuKeyPrefix; k != "" {
		r.add(Info, "doctor.key_prefix", k)
	}
	if cfg.Tmux.MenuKeyRoot != "" {
		r.add(Info, "doctor.key_vscode", cfg.Tmux.MenuKeyRoot)
	}
}

// checkOpenCode: OpenCode needs a writable state folder (a root-owned
// ~/.local/state, made by an earlier sudo, stops it), and imux lists its
// sessions with the sqlite3 CLI (§7.7).
func checkOpenCode(r *Report) bool {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		r.add(Warn, "doctor.opencode_sqlite")
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		state = filepath.Join(paths.UserHome(), ".local", "state")
	}
	if !writableDir(state) {
		r.add(Fail, "doctor.opencode_state", paths.Shorten(state), state)
		return false
	}
	return true
}

// writableDir reports whether dir (or, when it does not exist yet, its
// nearest existing parent) can get new entries.
func writableDir(dir string) bool {
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil {
			f, err := os.CreateTemp(d, ".imux-doctor-*")
			if err != nil {
				return false
			}
			name := f.Name()
			f.Close()
			os.Remove(name)
			return true
		}
		if d == filepath.Dir(d) {
			return false
		}
	}
}
