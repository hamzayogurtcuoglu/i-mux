package doctor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

type fakeTmux struct{ version string }

func (f fakeTmux) Run(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
	if len(args) == 1 && args[0] == "-V" {
		if f.version == "" {
			return nil, nil, &os.PathError{Op: "exec", Path: "tmux", Err: os.ErrNotExist}
		}
		return []byte("tmux " + f.version + "\n"), nil, nil
	}
	return nil, nil, nil
}

// script writes an executable shell script and returns its path.
func script(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	testutil.WriteFile(t, p, []byte(body))
	if err := os.Chmod(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func find(r *Report, sub string) *Result {
	for i := range r.Results {
		if strings.Contains(r.Results[i].Msg, sub) {
			return &r.Results[i]
		}
	}
	return nil
}

func TestDoctor(t *testing.T) {
	h := testutil.Isolate(t)
	i18n.SetLocale("en")
	if err := os.MkdirAll(h.Imux, 0o700); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(h.Root, "bin")
	log := filepath.Join(h.Root, "calls")
	// A claude that is too old and lacks --session-id in its help; a gemini
	// node script without node anywhere.
	claudeBin := script(t, bin, "claude", "#!/bin/sh\necho \"$@\" >> "+log+"\ncase \"$1\" in --version) echo 1.0.110;; --help) echo 'usage: claude --resume';; esac\n")
	geminiBin := script(t, bin, "gemini", "#!/usr/bin/env node\nconsole.log('x')\n")
	cfg := config.Default()
	res := map[session.AgentID]agent.Resolved{
		"claude": {Agent: "claude", Installed: true, Path: claudeBin, PathEnv: bin, Version: "1.0.110"},
		"gemini": {Agent: "gemini", Installed: true, Path: geminiBin, PathEnv: bin, Version: "0.31.0"},
	}
	o := Options{
		Cfg: cfg, Tmux: &tmux.Client{Bin: "tmux", Socket: "x", Runner: fakeTmux{"3.2a"}}, Self: claudeBin,
		Resolved: res, Users: func() []string { return []string{"alice"} },
		Watcher: func() (bool, int) { return true, 4242 },
	}
	r := Run(context.Background(), o)
	if r.Failed() {
		t.Fatalf("unexpected hard failure: %+v", r.Results)
	}
	checks := []struct {
		sub   string
		level Level
	}{
		{"tmux 3.2a", OK},
		{"agent bar is not clickable", Info},
		{"is private (0700)", OK},
		{"Claude Code 1.0.110", OK},
		{"Claude Code 1.0.110 is older than 2.0.0", Warn},
		{"does not mention --session-id", Warn},
		{"Gemini CLI needs node", Warn},
		{"Copilot CLI: not installed (npm i -g @github/copilot)", Info},
		{"watcher running (pid 4242)", OK},
		{"other users on this machine (alice)", Warn},
		{"VS Code", Info},
	}
	for _, c := range checks {
		x := find(r, c.sub)
		if x == nil || x.Level != c.level {
			t.Errorf("%q: got %+v", c.sub, x)
		}
	}
	// Agents are only asked for --version/--help, never given a prompt.
	b, _ := os.ReadFile(log)
	for _, call := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if call != "--help" && call != "--version" {
			t.Errorf("agent called with %q", call)
		}
	}

	// Loose permissions are reported.
	_ = os.Chmod(h.Imux, 0o755)
	testutil.WriteFile(t, filepath.Join(h.Imux, "state.json"), []byte("{}"))
	_ = os.Chmod(filepath.Join(h.Imux, "state.json"), 0o644)
	r = Run(context.Background(), o)
	if x := find(r, "should be 0700"); x == nil || x.Level != Warn {
		t.Errorf("home perm: %+v", x)
	}
	if x := find(r, "state.json"); x == nil || x.Level != Warn {
		t.Errorf("file perm: %+v", x)
	}

	// Missing tmux and no agents are hard failures; --ci skips agents.
	o.Tmux.Runner = fakeTmux{""}
	o.Resolved = map[session.AgentID]agent.Resolved{}
	r = Run(context.Background(), o)
	if !r.Failed() || find(r, "tmux is not installed") == nil || find(r, "no agent CLI") == nil {
		t.Fatalf("failures: %+v", r.Results)
	}
	o.Tmux.Runner = fakeTmux{"3.8"}
	o.CI = true
	r = Run(context.Background(), o)
	if r.Failed() || find(r, "Claude") != nil {
		t.Fatalf("--ci: %+v", r.Results)
	}
	o.Tmux.Runner = fakeTmux{"2.9a"}
	if r = Run(context.Background(), o); !r.Failed() || find(r, "too old") == nil {
		t.Fatalf("old tmux: %+v", r.Results)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.110", "2.0.0", -1},
		{"2.1.3", "2.0.0", 1},
		{"0.30.0", "0.30.0", 0},
		{"0.38.0-alpha.1", "0.38.0", 0},
		{"v1.2", "1.2.0", 0},
		{"10.0", "9.9.9", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compare(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
