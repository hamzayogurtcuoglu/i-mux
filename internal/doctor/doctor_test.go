package doctor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// TestMain lifts the agent probe timeout. The fake agents are freshly written
// scripts, and macOS scans a new executable on its first run, which can exceed
// the production timeout when every package's tests run in parallel.
func TestMain(m *testing.M) {
	agent.SetProbeTimeout(30 * time.Second)
	os.Exit(m.Run())
}

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
	// A claude that is too old and lacks --session-id in its help; a copilot
	// node script without node anywhere.
	claudeBin := script(t, bin, "claude", "#!/bin/sh\necho \"$@\" >> "+log+"\ncase \"$1\" in --version) echo 1.0.110;; --help) echo 'usage: claude --resume';; esac\n")
	copilotBin := script(t, bin, "copilot", "#!/usr/bin/env node\nconsole.log('x')\n")
	cfg := config.Default()
	cfg.Handoff.Deliver = "inline" // the handoff in argv: visible to other users
	res := map[session.AgentID]agent.Resolved{
		"claude":  {Agent: "claude", Installed: true, Path: claudeBin, PathEnv: bin, Version: "1.0.110"},
		"copilot": {Agent: "copilot", Installed: true, Path: copilotBin, PathEnv: bin, Version: "1.0.0"},
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
		{"GitHub Copilot CLI needs node", Warn},
		{"Antigravity CLI: not installed (curl -fsSL https://antigravity.google/cli/install.sh | bash)", Info},
		{"Codex CLI: not installed (npm i -g @openai/codex)", Info},
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

// TestDoctorOpenCode1: OpenCode 1 cannot start the sessions imux asks for
// (`--session ses_imux…` is "Session not found" there), so it is a hard
// failure with the upgrade command. A cached 1.x version is re-probed first:
// the bin cache lags an in-place `opencode upgrade --major` by up to a day.
func TestDoctorOpenCode1(t *testing.T) {
	h := testutil.Isolate(t)
	i18n.SetLocale("en")
	if err := os.MkdirAll(h.Imux, 0o700); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(h.Root, "bin")
	verFile := filepath.Join(h.Root, "opencode-version")
	testutil.WriteFile(t, verFile, []byte("1.19.5\n"))
	ocBin := script(t, bin, "opencode", "#!/bin/sh\n[ \"$1\" = --version ] && /bin/cat "+verFile+"\nexit 0\n")
	o := Options{
		Cfg: config.Default(), Tmux: &tmux.Client{Bin: "tmux", Socket: "x", Runner: fakeTmux{"3.4"}},
		Resolved: map[session.AgentID]agent.Resolved{
			"opencode": {Agent: "opencode", Installed: true, Path: ocBin, PathEnv: bin, Version: "1.19.5"},
		},
		Users: func() []string { return nil }, Watcher: func() (bool, int) { return true, 1 },
	}

	r := Run(context.Background(), o)
	x := find(r, "OpenCode 1.19.5 is OpenCode 1")
	if x == nil || x.Level != Fail || !strings.Contains(x.Msg, "opencode upgrade --major") || !r.Failed() {
		t.Fatalf("opencode 1: %+v", r.Results)
	}

	// Upgraded since the cache was written: the binary says 2.x, no failure.
	testutil.WriteFile(t, verFile, []byte("opencode v2.2.6\n"))
	r = Run(context.Background(), o)
	if x := find(r, "OpenCode 2.2.6"); x == nil || x.Level != OK || find(r, "is OpenCode 1") != nil {
		t.Fatalf("opencode 2 behind a stale cache: %+v", r.Results)
	}

	// Not installed: the hint ends at OpenCode 2 (the installer stops at 1).
	o.Resolved = map[session.AgentID]agent.Resolved{}
	r = Run(context.Background(), o)
	if x := find(r, "OpenCode: not installed"); x == nil || !strings.Contains(x.Msg, "opencode upgrade --major") {
		t.Fatalf("opencode hint: %+v", r.Results)
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
