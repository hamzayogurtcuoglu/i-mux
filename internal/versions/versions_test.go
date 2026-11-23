package versions

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// fakeEnv is a system with the given binaries (name → path), command
// outputs (argv joined by spaces → stdout) and installed agents (id →
// version).
func fakeEnv(bins map[string]string, outputs map[string]string, agents map[session.AgentID]string) Env {
	return Env{
		Cfg: config.Default(), Self: "/home/test/.local/bin/imux", Current: "v0.33.0",
		LookPath: func(name string) (string, error) {
			if p, ok := bins[name]; ok {
				return p, nil
			}
			return "", exec.ErrNotFound
		},
		Output: func(_ context.Context, argv ...string) string { return outputs[strings.Join(argv, " ")] },
		Resolve: func(id session.AgentID) agent.Resolved {
			v, ok := agents[id]
			if !ok {
				return agent.Resolved{Agent: id}
			}
			path := "/home/test/.local/bin/" + agent.MustGet(id).Bin
			if strings.HasPrefix(v, "brew:") {
				path = "/opt/homebrew/bin/" + agent.MustGet(id).Bin
			}
			return agent.Resolved{Agent: id, Installed: true, Path: path, Version: v}
		},
		Probe: func(r agent.Resolved) string { return strings.TrimPrefix(r.Version, "brew:") },
	}
}

func row(t *testing.T, rows []Row, name string) Row {
	t.Helper()
	for _, r := range rows {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("no row %q", name)
	return Row{}
}

func TestAgentAndImuxRows(t *testing.T) {
	testutil.SkipOnWindows(t, "POSIX install paths (/home/test/.local/bin)")
	i18n.SetLocale("en")
	e := fakeEnv(map[string]string{"tmux": "/usr/bin/tmux"}, map[string]string{"/usr/bin/tmux -V": "tmux 3.5a"}, map[session.AgentID]string{
		"claude": "2.1.319", "copilot": "1.0.107", "codex": "brew:0.50.0", "opencode": "1.4.2",
	})
	rows := Collect(context.Background(), e)
	if len(rows) != 2+len(agent.Ordered(e.Cfg)) || rows[0].Name != "i-mux" || rows[1].Name != "i-mux-core" {
		t.Fatalf("rows %+v", rows)
	}
	for name, want := range map[string]string{
		"Claude Code":        "/home/test/.local/bin/claude update",
		"GitHub Copilot CLI": "/home/test/.local/bin/copilot update",
		"Codex CLI":          "brew upgrade codex",
		"OpenCode":           "/home/test/.local/bin/opencode upgrade --major", // OpenCode 1: i-mux needs 2
	} {
		if r := row(t, rows, name); strings.Join(r.Fix, " ") != want || r.Status != OK {
			t.Errorf("%s: fix %q status %d", name, r.Fix, r.Status)
		}
	}
	if r := row(t, rows, "Claude Code"); r.Show != "claude update" || r.Version != "2.1.319" || r.Key != "c" {
		t.Errorf("claude %+v", r)
	}
	// A missing agent: its letter runs the command Install now runs (D79);
	// without curl, the row says how to get it and nothing runs.
	agy := "curl -fsSL https://antigravity.google/cli/install.sh | bash"
	if r := row(t, rows, "Antigravity CLI"); r.Status != Missing || r.Fix != nil || r.Show != agy ||
		!strings.Contains(r.Note, "with curl, which is not installed here") || !strings.Contains(r.Note, "sudo apt install curl") {
		t.Errorf("missing agent without curl: %+v", r)
	}
	look := e.LookPath
	e.LookPath = func(name string) (string, error) {
		if name == "curl" {
			return "/usr/bin/curl", nil
		}
		return look(name)
	}
	if r := row(t, Collect(context.Background(), e), "Antigravity CLI"); r.Status != Missing || strings.Join(r.Fix, " ") != "sh -c "+agy ||
		r.Show != agy || r.Note != "not installed" || r.Agent != session.Antigravity || r.Key != "a" {
		t.Errorf("missing agent: %+v", r)
	}
	// Codex from npm.
	if fix, show := agentUpdate(agent.MustGet(session.Codex), "/usr/local/lib/node_modules/.bin/codex", "0.50.0"); show != "npm install -g @openai/codex@latest" || len(fix) != 4 {
		t.Errorf("codex from npm: %q %q", fix, show)
	}
	if r := row(t, rows, "OpenCode"); r.Show != "opencode upgrade --major" {
		t.Errorf("opencode %+v", r)
	}

	// i-mux: newer out, the newest, unknown.
	if r := imuxRow(Env{Self: "/x/imux", Current: "v0.33.0", Latest: "0.34.0"}); r.Status != Old || r.Note != "0.34.0 is out" || r.Show != "imux update" || r.Fix[1] != "update" {
		t.Errorf("newer out %+v", r)
	}
	// Only a live answer (D98) makes it the newest, without a letter; the
	// daily check's may be hours old, and imux update asks again itself.
	if r := imuxRow(Env{Self: "/x/imux", Current: "v0.33.0", Latest: "0.33.0"}); r.Fix == nil || r.Note != "" {
		t.Errorf("daily check only: %+v", r)
	}
	if r := imuxRow(Env{Self: "/x/imux", Current: "v0.33.0", Latest: "0.33.0", Live: true}); r.Status != OK || r.Fix != nil || r.Note != "the newest" {
		t.Errorf("newest %+v", r)
	}
	if r := imuxRow(Env{Self: "/x/imux", Current: "v0.33.0"}); r.Fix == nil || r.Note != "" || r.Version != "0.33.0" {
		t.Errorf("unknown latest %+v", r)
	}
}

func TestFormat(t *testing.T) {
	i18n.SetLocale("en")
	rows := []Row{
		{Key: "u", Name: "i-mux", Version: "0.33.0", Note: "the newest"},
		{Key: "t", Name: "tmux", Version: "3.2a", Status: Old, Note: "works; 3.4+: the agent bar takes clicks\napt has no newer tmux here"},
		{Key: "c", Name: "Claude Code", Version: "2.1.319", Show: "claude update", Fix: []string{"claude", "update"}},
		{Key: "x", Name: "Codex CLI", Status: Missing, Note: "not installed", Show: "npm i -g @openai/codex", Fix: []string{"sh", "-c", "npm i -g @openai/codex"}},
		{Key: "e", Name: "OpenCode", Status: Missing, Note: "not installed\nOpenCode installs with curl, which is not installed here.", Show: "curl -fsSL https://opencode.ai/v2/install | bash"},
		{Key: "q", Name: "Odd", Status: Unknown, Show: "odd update", Fix: []string{"odd", "update"}},
	}
	got := Format(rows)
	t.Logf("\n%s", got)
	for _, want := range []string{
		// A row with nothing to run shows no letter (T98.3).
		"     i-mux        0.33.0   ✓ the newest\n",
		"     tmux         3.2a     ⚠ works; 3.4+: the agent bar takes clicks\n                           apt has no newer tmux here\n",
		"  c  Claude Code  2.1.319  update: claude update\n",
		"  x  Codex CLI    –        ✗ not installed · install: npm i -g @openai/codex\n",
		"     OpenCode     –        ✗ not installed\n                           OpenCode installs with curl, which is not installed here.\n                           install: curl -fsSL https://opencode.ai/v2/install | bash\n",
		"  q  Odd          –        update: odd update\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q", want)
		}
	}
}

// TestNothing (T98.3): a letter with nothing to run says why.
func TestNothing(t *testing.T) {
	i18n.SetLocale("en")
	rows := []Row{{Key: "u", Name: "i-mux"}, {Key: "t", Name: "i-mux-core", Tmux: true}}
	for key, want := range map[string]string{
		"u": "i-mux is up to date: nothing to do.",
		"t": "i-mux-core comes with i-mux: updating i-mux updates it.",
		"z": "",
	} {
		if got := Nothing(rows, key); got != want {
			t.Errorf("%s: %q, want %q", key, got, want)
		}
	}
}
