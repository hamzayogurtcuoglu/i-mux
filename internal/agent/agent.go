// Package agent is the registry of supported agent CLIs: ids, names, menu
// mnemonics, install hints, launch templates, and binary resolution with the
// PATH each binary needs (nvm-installed CLIs need their node next to them).
package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
)

// Info describes one agent CLI.
type Info struct {
	ID          session.AgentID
	Name        string // full product name: "Claude Code"
	Short       string // UI name: "Claude"
	Key         string // menu mnemonic
	Bin         string // default binary name
	InstallHint string
}

var registry = []Info{
	{ID: session.Claude, Key: "c", Bin: "claude", InstallHint: "npm i -g @anthropic-ai/claude-code"},
	{ID: session.Antigravity, Key: "a", Bin: "agy", InstallHint: "curl -fsSL https://antigravity.google/cli/install.sh | bash"},
	{ID: session.Gemini, Key: "g", Bin: "gemini", InstallHint: "npm i -g @google/gemini-cli"},
	{ID: session.Copilot, Key: "p", Bin: "copilot", InstallHint: "npm i -g @github/copilot"},
	{ID: session.Codex, Key: "x", Bin: "codex", InstallHint: "npm i -g @openai/codex"},
}

func init() {
	for i := range registry {
		registry[i].Name = session.DisplayName(registry[i].ID)
		registry[i].Short = session.ShortName(registry[i].ID)
	}
}

// All returns every agent in registry order.
func All() []Info { return slices.Clone(registry) }

// Get returns the agent with id.
func Get(id session.AgentID) (Info, bool) {
	for _, a := range registry {
		if a.ID == id {
			return a, true
		}
	}
	return Info{}, false
}

// MustGet returns the agent with id or a placeholder.
func MustGet(id session.AgentID) Info {
	if a, ok := Get(id); ok {
		return a
	}
	return Info{ID: id, Name: string(id), Short: string(id), Bin: string(id)}
}

// Ordered returns the agents in config order (unknown entries skipped, missing
// ones appended).
func Ordered(cfg *config.Config) []Info {
	var out []Info
	seen := map[session.AgentID]bool{}
	for _, id := range cfg.Order {
		if a, ok := Get(session.AgentID(id)); ok && !seen[a.ID] {
			out = append(out, a)
			seen[a.ID] = true
		}
	}
	for _, a := range registry {
		if !seen[a.ID] {
			out = append(out, a)
		}
	}
	return out
}

// Vars are the template placeholders.
type Vars struct {
	Prompt string // {prompt}
	ID     string // {id}
	UUID   string // {uuid}
}

// Expand replaces placeholders in a launch template. An argument that is
// exactly "{prompt}" (or "{id}", "{uuid}") is dropped when the value is
// empty, together with the flag right before it ("-i {prompt}" without a
// prompt passes nothing), so a launch without a prompt passes no empty or
// dangling argument. "--flag={prompt}" is dropped likewise. Values are
// inserted verbatim: argv never goes through a shell.
func Expand(tpl []string, v Vars) []string {
	empty := func(a string) bool {
		return (a == "{prompt}" && v.Prompt == "") || (a == "{id}" && v.ID == "") || (a == "{uuid}" && v.UUID == "")
	}
	out := make([]string, 0, len(tpl))
	for i, a := range tpl {
		if empty(a) {
			if i > 0 && len(out) > 0 && strings.HasPrefix(tpl[i-1], "-") && !strings.Contains(tpl[i-1], "{") &&
				!strings.Contains(tpl[i-1], "=") {
				out = out[:len(out)-1] // the flag that introduced it
			}
			continue
		}
		if f, val, ok := strings.Cut(a, "="); ok && strings.HasPrefix(f, "-") && empty(val) {
			continue
		}
		if !strings.Contains(a, "{") {
			out = append(out, a)
			continue
		}
		// Substitute {id}/{uuid} first so that a prompt containing literal
		// placeholders is never re-expanded.
		a = strings.ReplaceAll(a, "{id}", v.ID)
		a = strings.ReplaceAll(a, "{uuid}", v.UUID)
		a = strings.Replace(a, "{prompt}", v.Prompt, 1)
		out = append(out, a)
	}
	return out
}

// HasPrompt reports whether a template takes the prompt.
func HasPrompt(tpl []string) bool {
	for _, a := range tpl {
		if strings.Contains(a, "{prompt}") {
			return true
		}
	}
	return false
}

// NewUUID returns a random version-4 UUID.
func NewUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Resolved is where an agent binary lives and how to launch it.
type Resolved struct {
	Agent     session.AgentID
	Installed bool
	Path      string // absolute path as found (symlinks not resolved)
	PathEnv   string // PATH to launch with
	Version   string
	FromCache bool // resolved from state.json's bin cache
}

// LookPathFunc is exec.LookPath; tests replace it.
var LookPathFunc = exec.LookPath

// Resolve finds agent id's binary. Order: config `bin` (absolute path, or a
// name looked up on PATH), the current PATH, then the bin cache (menus and
// the watcher run with the tmux server's environment, which may lack nvm's
// PATH). PathEnv is the PATH to use, with the binary's *unresolved*
// directory first (nvm's bin/ holds both the CLI and node).
func Resolve(cfg *config.Config, id session.AgentID, st *state.State) Resolved {
	info := MustGet(id)
	r := Resolved{Agent: id}
	name := info.Bin
	if b := cfg.Agent(string(id)).Bin; b != "" {
		name = b
	}
	var path string
	if filepath.IsAbs(name) {
		if isExecutable(name) {
			path = name
		}
	} else if p, err := LookPathFunc(name); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			path = abs
		}
	}
	if path != "" {
		r.Installed = true
		r.Path = path
		r.PathEnv = prependDir(filepath.Dir(path), os.Getenv("PATH"))
		if st != nil {
			if b := st.Bins[string(id)]; b != nil && b.Path == path {
				r.Version = b.Version
			}
		}
		return r
	}
	if st != nil {
		if b := st.Bins[string(id)]; b != nil && b.Path != "" && isExecutable(b.Path) &&
			(cfg.Agent(string(id)).Bin == "" || cfg.Agent(string(id)).Bin == b.Path) {
			r.Installed = true
			r.Path = b.Path
			r.PathEnv = prependDir(filepath.Dir(b.Path), b.PathEnv)
			r.Version = b.Version
			r.FromCache = true
		}
	}
	return r
}

// ResolveAll resolves every agent in config order.
func ResolveAll(cfg *config.Config, st *state.State) map[session.AgentID]Resolved {
	out := map[session.AgentID]Resolved{}
	for _, a := range Ordered(cfg) {
		out[a.ID] = Resolve(cfg, a.ID, st)
	}
	return out
}

func prependDir(dir, pathEnv string) string {
	parts := filepath.SplitList(pathEnv)
	out := []string{dir}
	for _, p := range parts {
		if p != "" && p != dir {
			out = append(out, p)
		}
	}
	return strings.Join(out, string(os.PathListSeparator))
}

func isExecutable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0
}

var versionRe = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?(?:[-+][0-9A-Za-z.\-]+)?`)

// probeTimeout bounds every agent probe (`--version`, `--help`). Node-based
// CLIs can take over a second to start on a busy machine, and macOS scans a
// newly written executable on its first run, so 2 s proved too tight. Version
// probes run in parallel and are cached for a day, so the bound only matters
// for a binary that hangs.
var probeTimeout = 5 * time.Second

// SetProbeTimeout changes the probe timeout and returns a func that restores
// it. It exists for tests, whose fake agents are freshly written scripts and
// must not depend on machine load.
func SetProbeTimeout(d time.Duration) (restore func()) {
	old := probeTimeout
	probeTimeout = d
	return func() { probeTimeout = old }
}

// Probe runs `<path> args...` with the probe timeout and returns stdout, plus
// stderr when withStderr is set. Callers must never pass a prompt. On timeout
// the whole process group is killed: a child the CLI started would otherwise
// keep the output pipe open and the probe would wait for it. WaitDelay is a
// backstop if something outside the group still holds the pipe.
func Probe(ctx context.Context, path, pathEnv string, withStderr bool, args ...string) []byte {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = append(os.Environ(), "PATH="+pathEnv, "NO_COLOR=1")
	cmd.Stdin = nil
	var out bytes.Buffer
	cmd.Stdout = &out
	if withStderr {
		cmd.Stderr = &out
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	_ = cmd.Run() // a non-zero exit or a timeout still returns what was printed
	return out.Bytes()
}

// ProbeVersion runs `<bin> --version` (never with a prompt) and returns the
// first version-looking token of its stdout.
func ProbeVersion(path, pathEnv string) string {
	return versionRe.FindString(string(Probe(context.Background(), path, pathEnv, false, "--version")))
}

// RefreshCache resolves every agent in the current (interactive) process and
// records path, PATH and version in state.json. Versions are re-probed only
// when the path changed or the entry is older than a day.
func RefreshCache(cfg *config.Config, now time.Time) (map[session.AgentID]Resolved, error) {
	st, err := state.Read()
	if err != nil {
		return nil, err
	}
	res := map[session.AgentID]Resolved{}
	type probe struct {
		id session.AgentID
		r  Resolved
	}
	var toProbe []probe
	for _, a := range Ordered(cfg) {
		r := Resolve(cfg, a.ID, st)
		res[a.ID] = r
		if !r.Installed || r.FromCache {
			continue
		}
		old := st.Bins[string(a.ID)]
		if old == nil || old.Path != r.Path || old.Version == "" || now.Sub(old.CheckedAt) > 24*time.Hour {
			toProbe = append(toProbe, probe{a.ID, r})
		}
	}
	if len(toProbe) > 0 {
		done := make(chan struct{})
		for i := range toProbe {
			go func(p *probe) {
				p.r.Version = ProbeVersion(p.r.Path, p.r.PathEnv)
				done <- struct{}{}
			}(&toProbe[i])
		}
		for range toProbe {
			<-done
		}
	}
	err = state.Update(func(s *state.State) error {
		for id, r := range res {
			if !r.Installed || r.FromCache {
				continue
			}
			b := &state.Bin{Path: r.Path, PathEnv: os.Getenv("PATH"), Version: r.Version, CheckedAt: now}
			if old := s.Bins[string(id)]; old != nil && old.Path == r.Path {
				b.Version, b.CheckedAt = old.Version, old.CheckedAt
			}
			for _, p := range toProbe {
				if p.id == id {
					b.Version, b.CheckedAt = p.r.Version, now
				}
			}
			r.Version = b.Version
			res[id] = r
			s.SetBin(string(id), b)
		}
		return nil
	})
	return res, err
}
