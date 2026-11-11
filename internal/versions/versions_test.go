package versions

import (
	"context"
	"os/exec"
	"strings"
	"testing"

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

const aptTmux = "env LC_ALL=C apt-cache policy tmux"

func TestTmuxRow(t *testing.T) {
	i18n.SetLocale("en")
	ctx := context.Background()
	for _, c := range []struct {
		name    string
		bins    map[string]string
		outputs map[string]string
		root    bool
		status  Status
		fix     string
		note    string
	}{
		{
			name: "Ubuntu 22.04: apt's newer tmux is the same 3.2a",
			bins: map[string]string{"tmux": "/usr/bin/tmux", "apt-get": "/usr/bin/apt-get"},
			outputs: map[string]string{
				"/usr/bin/tmux -V": "tmux 3.2a\n",
				aptTmux:            "tmux:\n  Installed: 3.2a-4ubuntu0.2\n  Candidate: 3.2a-4ubuntu0.3\n",
			},
			status: Old, note: "apt has no newer tmux here",
		},
		{
			name: "apt has a newer tmux",
			bins: map[string]string{"tmux": "/usr/bin/tmux", "apt-get": "/usr/bin/apt-get"},
			outputs: map[string]string{
				"/usr/bin/tmux -V": "tmux 3.3a\n",
				aptTmux:            "tmux:\n  Installed: 3.3a-3\n  Candidate: 1:3.5a-1\n",
			},
			status: Old, fix: "sudo apt-get install -y --only-upgrade tmux", note: "3.4+: the agent bar takes clicks",
		},
		{
			name: "as root, no sudo",
			bins: map[string]string{"tmux": "/usr/bin/tmux", "apt-get": "/usr/bin/apt-get"},
			outputs: map[string]string{
				"/usr/bin/tmux -V": "tmux 3.3a\n",
				aptTmux:            "tmux:\n  Installed: 3.3a-3\n  Candidate: 3.4-1\n",
			},
			root: true, status: Old, fix: "apt-get install -y --only-upgrade tmux", note: "3.4+: the agent bar takes clicks",
		},
		{
			name:    "apt has none newer, Homebrew is there",
			bins:    map[string]string{"tmux": "/usr/bin/tmux", "apt-get": "/usr/bin/apt-get", "brew": "/home/linuxbrew/.linuxbrew/bin/brew"},
			outputs: map[string]string{"/usr/bin/tmux -V": "tmux 3.2a\n", aptTmux: "tmux:\n  Installed: 3.2a-4\n  Candidate: 3.2a-4\n"},
			status:  Old, fix: "brew install tmux",
		},
		{
			name:    "Homebrew's tmux",
			bins:    map[string]string{"tmux": "/opt/homebrew/bin/tmux", "brew": "/opt/homebrew/bin/brew"},
			outputs: map[string]string{"/opt/homebrew/bin/tmux -V": "tmux 3.3a\n"},
			status:  Old, fix: "brew upgrade tmux",
		},
		{
			name:    "new enough",
			bins:    map[string]string{"tmux": "/opt/homebrew/bin/tmux", "brew": "/opt/homebrew/bin/brew"},
			outputs: map[string]string{"/opt/homebrew/bin/tmux -V": "tmux 3.6\n"},
			status:  OK,
		},
		{
			name:    "3.4: menus take the mouse there too (D54)",
			bins:    map[string]string{"tmux": "/usr/bin/tmux", "apt-get": "/usr/bin/apt-get"},
			outputs: map[string]string{"/usr/bin/tmux -V": "tmux 3.4\n", aptTmux: "tmux:\n  Installed: 3.4-1\n  Candidate: 3.5a-1\n"},
			status:  OK,
		},
		{
			name:   "missing, Fedora",
			bins:   map[string]string{"dnf": "/usr/bin/dnf"},
			status: Missing, fix: "sudo dnf install -y tmux", note: "not installed",
		},
		{
			name:   "missing, no package manager known",
			bins:   map[string]string{},
			status: Missing, note: "update tmux with your package manager",
		},
	} {
		e := fakeEnv(c.bins, c.outputs, nil)
		e.Root = c.root
		r := tmuxRow(ctx, e)
		if r.Status != c.status || r.Show != c.fix || !strings.Contains(r.Note, c.note) || strings.Join(r.Fix, " ") != c.fix {
			t.Errorf("%s: status %d fix %q show %q note %q", c.name, r.Status, r.Fix, r.Show, r.Note)
		}
		if !r.Tmux || r.Key != "t" {
			t.Errorf("%s: %+v", c.name, r)
		}
	}
	if r := tmuxRow(ctx, fakeEnv(map[string]string{"tmux": "/usr/bin/tmux"}, map[string]string{"/usr/bin/tmux -V": "tmux next-3.6\n"}, nil)); r.Status != OK || r.Version != "next-3.6" {
		t.Errorf("a development tmux: %+v", r)
	}
}

func TestAgentAndImuxRows(t *testing.T) {
	i18n.SetLocale("en")
	e := fakeEnv(map[string]string{"tmux": "/usr/bin/tmux"}, map[string]string{"/usr/bin/tmux -V": "tmux 3.5a"}, map[session.AgentID]string{
		"claude": "2.1.319", "copilot": "1.0.107", "codex": "brew:0.50.0", "opencode": "1.4.2",
	})
	rows := Collect(context.Background(), e)
	if len(rows) != 2+len(agent.Ordered(e.Cfg)) || rows[0].Name != "i-mux" || rows[1].Name != "tmux" {
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
	if r := row(t, rows, "Antigravity CLI"); r.Status != Missing || r.Fix != nil || !strings.Contains(r.Note, "antigravity.google/cli/install.sh") {
		t.Errorf("missing agent %+v", r)
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
	if r := imuxRow(Env{Self: "/x/imux", Current: "v0.33.0", Latest: "0.33.0"}); r.Status != OK || r.Fix != nil || r.Note != "the newest" {
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
		{Key: "c", Name: "Claude Code", Version: "2.1.319", Show: "claude update"},
		{Key: "x", Name: "Codex CLI", Status: Missing, Note: "not installed · install: npm i -g @openai/codex"},
		{Key: "q", Name: "Odd", Status: Unknown, Show: "odd update"},
	}
	got := Format(rows)
	t.Logf("\n%s", got)
	for _, want := range []string{
		"  u  i-mux        0.33.0   ✓ the newest\n",
		"  t  tmux         3.2a     ⚠ works; 3.4+: the agent bar takes clicks\n                           apt has no newer tmux here\n",
		"  c  Claude Code  2.1.319  update: claude update\n",
		"  x  Codex CLI    –        ✗ not installed · install: npm i -g @openai/codex\n",
		"  q  Odd          –        update: odd update\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q", want)
		}
	}
	if aptPolicy("tmux:\n  Installed: (none)\n  Candidate: 3.2a-4\n"); upstream("1:3.5a-2ubuntu1") != "3.5a" || upstream("3.2a") != "3.2a" {
		t.Error("upstream")
	}
	if i, c := aptPolicy("tmux:\n  Installed: (none)\n  Candidate: 3.2a-4\n"); i != "" || c != "3.2a-4" {
		t.Errorf("aptPolicy %q %q", i, c)
	}
}
