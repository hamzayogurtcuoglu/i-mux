package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff"
	"github.com/hamzayogurtcuoglu/i-mux/internal/launch"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/claude"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func writeClaude(t *testing.T, h *testutil.Homes, id, text string, mtime time.Time) string {
	t.Helper()
	p := filepath.Join(h.Claude, "projects", claude.EncodeCWD(h.Project), id+".jsonl")
	lines := `{"type":"user","sessionId":"` + id + `","cwd":"` + h.Project + `","timestamp":"2026-11-05T10:00:00Z","uuid":"a","message":{"role":"user","content":"` + text + `"}}
{"type":"assistant","sessionId":"` + id + `","timestamp":"2026-11-05T10:00:01Z","uuid":"b","message":{"id":"m1","role":"assistant","model":"claude-opus-4-1","content":[{"type":"text","text":"On it."}]}}
`
	testutil.WriteFile(t, p, []byte(lines))
	testutil.Touch(t, p, mtime)
	return p
}

func writeCodex(t *testing.T, h *testutil.Homes, text string, mtime time.Time) string {
	t.Helper()
	id := "cccccccc-1111-4222-8333-444444444444"
	p := filepath.Join(h.Codex, "sessions", "2026", "11", "05", "rollout-2026-11-05T09-00-00-"+id+".jsonl")
	lines := `{"timestamp":"2026-11-05T09:00:00Z","type":"session_meta","payload":{"id":"` + id + `","cwd":"` + h.Project + `"}}
{"timestamp":"2026-11-05T09:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + text + `"}]}}
`
	testutil.WriteFile(t, p, []byte(lines))
	testutil.Touch(t, p, mtime)
	return p
}

func inProject(t *testing.T, h *testutil.Homes) {
	t.Helper()
	old, _ := os.Getwd()
	if err := os.Chdir(h.Project); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestHandoffPrintNewestOfAnyAgent(t *testing.T) {
	h := testutil.Isolate(t)
	inProject(t, h)
	writeClaude(t, h, "11111111-0000-4000-8000-000000000001", "older claude task", testutil.FrozenTime.Add(-time.Hour))
	writeCodex(t, h, "newest codex task", testutil.FrozenTime)
	out, errOut, code := runCLI(t, "handoff", "--print", "--to", "antigravity")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.HasPrefix(out, "[i-mux handoff · Codex CLI → Antigravity CLI ·") || !strings.Contains(out, "newest codex task") {
		t.Fatalf("unexpected doc:\n%s", out)
	}
	if ents, _ := os.ReadDir(paths.Handoffs()); len(ents) != 0 {
		t.Fatal("--print must not write a handoff file")
	}
	out, _, code = runCLI(t, "handoff", "--print", "--from", "claude")
	if code != 0 || !strings.Contains(out, "older claude task") || !strings.Contains(out, "Claude Code →") {
		t.Fatalf("--from claude: %d\n%s", code, out)
	}
}

func TestHandoffExecsTarget(t *testing.T) {
	h := testutil.Isolate(t)
	inProject(t, h)
	writeClaude(t, h, "11111111-0000-4000-8000-000000000001", "fix the bug", testutil.FrozenTime)
	fake := filepath.Join(h.Home, "fakebin", "agy")
	testutil.WriteFile(t, fake, []byte("#!/bin/sh\nexit 0\n"))
	_ = os.Chmod(fake, 0o755)
	t.Setenv("PATH", filepath.Dir(fake)+":/usr/bin:/bin")
	var got *launch.Spec
	launchExec = func(s *launch.Spec) error { got = s; return nil }
	defer func() { launchExec = launch.Exec }()
	_, errOut, code := runCLI(t, "handoff", "--from", "claude", "--to", "antigravity")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	// A short message pointing to the saved file, which agy may read.
	if got == nil || got.Bin != fake || len(got.Args) != 3 || got.Args[0] != "--add-dir="+paths.Handoffs() || got.Args[1] != "-i" ||
		!strings.HasPrefix(got.Args[2], "[i-mux handoff · Claude Code → Antigravity CLI") || strings.Count(got.Args[2], "\n") != 2 ||
		!strings.HasPrefix(strings.Split(got.Args[2], "\n")[2], paths.Handoffs()) {
		t.Fatalf("spec %+v", got)
	}
	if got.Dir != h.Project || !strings.HasPrefix(got.Env["PATH"], filepath.Dir(fake)) {
		t.Fatalf("dir/env %+v", got)
	}
	ents, _ := os.ReadDir(paths.Handoffs())
	if len(ents) != 1 || !strings.HasSuffix(ents[0].Name(), "-claude-to-antigravity.md") {
		t.Fatalf("handoff files %v", ents)
	}
	fi, _ := ents[0].Info()
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode())
	}
	if !strings.Contains(errOut, "switching sends excerpts") {
		t.Fatalf("cross-provider notice missing: %s", errOut)
	}
	_, errOut, _ = runCLI(t, "handoff", "--from", "claude", "--to", "antigravity")
	if strings.Contains(errOut, "switching sends excerpts") {
		t.Fatal("notice shown twice")
	}
}

func TestHandoffTargetNotInstalled(t *testing.T) {
	h := testutil.Isolate(t)
	inProject(t, h)
	t.Setenv("PATH", "/usr/bin:/bin")
	writeClaude(t, h, "11111111-0000-4000-8000-000000000001", "x", testutil.FrozenTime)
	_, errOut, code := runCLI(t, "handoff", "--to", "copilot")
	if code == 0 || !strings.Contains(errOut, "npm i -g @github/copilot") {
		t.Fatalf("%d %s", code, errOut)
	}
}

func TestHandoffNoSession(t *testing.T) {
	h := testutil.Isolate(t)
	inProject(t, h)
	_, errOut, code := runCLI(t, "handoff", "--print")
	if code != 1 || !strings.Contains(errOut, "no agent session found") {
		t.Fatalf("%d %s", code, errOut)
	}
}

func TestSessionsList(t *testing.T) {
	h := testutil.Isolate(t)
	inProject(t, h)
	writeClaude(t, h, "11111111-0000-4000-8000-000000000001", "fix the login redirect bug in the auth handler please", time.Now().Add(-12*time.Minute))
	writeCodex(t, h, "add a rate limiter", time.Now().Add(-2*time.Hour))
	out, _, code := runCLI(t, "sessions")
	if code != 0 {
		t.Fatal(code)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "AGENT") {
		t.Fatalf("%q", out)
	}
	if !strings.Contains(lines[1], "claude") || !strings.Contains(lines[1], "12 min ago") || !strings.Contains(lines[1], "fix the login redirect bug in the auth…") {
		t.Fatalf("row 1 %q", lines[1])
	}
	if !strings.Contains(lines[2], "codex") || !strings.Contains(lines[2], "add a rate limiter") {
		t.Fatalf("row 2 %q", lines[2])
	}
	out, _, _ = runCLI(t, "sessions", "--agent", "codex")
	if strings.Contains(out, "claude") {
		t.Fatal("--agent filter")
	}
}

func TestPruneOncePerDay(t *testing.T) {
	testutil.Isolate(t)
	_ = paths.Ensure()
	old := filepath.Join(paths.Handoffs(), "20261001-000000-claude-to-antigravity.md")
	testutil.WriteFile(t, old, []byte("x"))
	testutil.Touch(t, old, testutil.FrozenTime.Add(-40*24*time.Hour))
	if !handoff.Prune(30, testutil.FrozenTime) {
		t.Fatal("first prune should run")
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("old handoff not pruned")
	}
	if handoff.Prune(30, testutil.FrozenTime.Add(time.Hour)) {
		t.Fatal("second prune within a day should not run")
	}
	if !handoff.Prune(30, testutil.FrozenTime.Add(25*time.Hour)) {
		t.Fatal("prune after a day should run")
	}
}
