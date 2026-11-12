package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/auto"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// fakeClaude puts a `claude` on PATH that records its arguments, folder
// and environment in dir and answers with dir/reply (and dir/stderr, exit
// code dir/exit). No tmux runs: the client's binary is missing.
func fakeClaude(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > " + dir + "/argv\n" +
		"pwd > " + dir + "/pwd\n" +
		"echo \"${NO_COLOR:-unset} ${TMUX:-unset}\" > " + dir + "/env\n" +
		"cat " + dir + "/reply 2>/dev/null\n" +
		"cat " + dir + "/stderr >&2 2>/dev/null\n" +
		"exit $(cat " + dir + "/exit 2>/dev/null || echo 0)\n"
	testutil.WriteFile(t, filepath.Join(bin, "claude"), []byte(script))
	if err := os.Chmod(filepath.Join(bin, "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("IMUX_TMUX_BIN", filepath.Join(dir, "no-tmux"))
	t.Setenv("TMUX", "/tmp/tmux-1/default,1,0")
	return dir
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestAutoExec (T66.2): a run starts the definition's CLI headless in its
// project, records the report (the next run gets it), a quiet run and a
// failure; one that is already going on is not started twice; a definition
// that went is skipped.
func TestAutoExec(t *testing.T) {
	h := testutil.Isolate(t)
	fake := fakeClaude(t)
	root := h.Project
	agentFile := filepath.Join(root, ".claude/agents/sim-tag.md")
	testutil.WriteFile(t, agentFile, []byte("---\nname: sim-tag\ndescription: Watches tags\n---\nCompare the tags.\n"))
	def := auto.List(root, "")[0]
	if _, err := auto.Change(def.ID, &def, root, time.Now(), func(s *auto.Schedule) { s.CLI, s.Model = "claude", "haiku" }); err != nil {
		t.Fatal(err)
	}
	exec := func() {
		t.Helper()
		if _, errOut, code := runCLI(t, "__auto", "exec", def.ID, "--manual"); code != 0 {
			t.Fatalf("exec: %d %s", code, errOut)
		}
	}

	testutil.WriteFile(t, filepath.Join(fake, "reply"), []byte("## v2.3 is out\nThe pipeline started.\n"))
	exec()
	r := auto.Last(def.ID)
	if r == nil || r.Status != auto.StatusOK || r.Summary != "v2.3 is out" || !strings.Contains(r.Report, "The pipeline started.") ||
		r.CLI != "claude" || r.Model != "haiku" || !r.Manual || len(r.Session) != 36 || r.End.Before(r.Start) {
		t.Fatalf("run %+v", r)
	}
	argv := strings.Split(readFile(t, filepath.Join(fake, "argv")), "\n")
	want := []string{"-p", "--session-id", r.Session, "--agent", "sim-tag", "--model", "haiku", "--dangerously-skip-permissions"}
	if len(argv) < len(want)+1 || strings.Join(argv[:len(want)], " ") != strings.Join(want, " ") {
		t.Fatalf("argv %q", argv)
	}
	prompt := strings.Join(argv[len(want):], "\n")
	if strings.Contains(prompt, "Compare the tags.") || !strings.Contains(prompt, "started by hand") || !strings.Contains(prompt, "This is the first run.") {
		t.Errorf("prompt:\n%s", prompt)
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(fake, "pwd"))); got != root {
		t.Errorf("ran in %q", got)
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(fake, "env"))); got != "1 unset" {
		t.Errorf("NO_COLOR and TMUX: %q", got)
	}
	if log := readFile(t, auto.LogPath(def.ID, r.Log)); !strings.Contains(log, " -p --session-id ") || !strings.Contains(log, "<prompt>") || !strings.Contains(log, "v2.3 is out") {
		t.Errorf("log:\n%s", log)
	}

	testutil.WriteFile(t, filepath.Join(fake, "reply"), []byte(auto.NothingNew+"\n"))
	time.Sleep(time.Second) // a log a second
	exec()
	if r := auto.Last(def.ID); r.Status != auto.StatusQuiet || r.Report != "" {
		t.Errorf("quiet run %+v", r)
	}
	if p := readFile(t, filepath.Join(fake, "argv")); !strings.Contains(p, "Your report from the last run") || !strings.Contains(p, "The pipeline started.") {
		t.Errorf("the last report is not passed on:\n%s", p)
	}

	testutil.WriteFile(t, filepath.Join(fake, "reply"), nil)
	testutil.WriteFile(t, filepath.Join(fake, "stderr"), []byte("\nError: bad model\n"))
	testutil.WriteFile(t, filepath.Join(fake, "exit"), []byte("3\n"))
	time.Sleep(time.Second)
	exec()
	if r := auto.Last(def.ID); r.Status != auto.StatusFailed || r.Exit != 3 || r.Summary != "Error: bad model" {
		t.Errorf("failed run %+v", r)
	}

	// Already running: no second run.
	ex, err := auto.Begin(def.ID)
	if err != nil {
		t.Fatal(err)
	}
	exec()
	if err := ex.Finish(auto.Run{Start: time.Now(), Status: auto.StatusStopped}); err != nil {
		t.Fatal(err)
	}
	if n := len(auto.Runs(def.ID)); n != 4 {
		t.Errorf("%d runs", n)
	}

	// On schedule: not when it is not due (it ran a moment ago, by hand).
	if _, err := auto.Change(def.ID, nil, "", time.Now(), func(s *auto.Schedule) { s.When = "5m" }); err != nil {
		t.Fatal(err)
	}
	if _, errOut, code := runCLI(t, "__auto", "exec", def.ID); code != 0 || len(auto.Runs(def.ID)) != 4 {
		t.Fatalf("a run not due: %d %s, %d runs", code, errOut, len(auto.Runs(def.ID)))
	}

	if err := os.Remove(agentFile); err != nil {
		t.Fatal(err)
	}
	exec()
	if r := auto.Last(def.ID); r.Status != auto.StatusSkipped || r.Summary != "its definition is gone" {
		t.Errorf("gone %+v", r)
	}
	if _, _, code := runCLI(t, "__auto", "exec", "not-an-id"); code == 0 {
		t.Error("a bad id ran")
	}
}

func TestWrapLine(t *testing.T) {
	got := wrapLine(" The pipeline started for v2.3 and every check passed", 20)
	if strings.Join(got, "|") != " The pipeline| started for v2.3| and every check| passed" {
		t.Errorf("%q", got)
	}
	if got := wrapLine(" "+strings.Repeat("x", 25), 10); len(got) != 3 || got[0] != " xxxxxxxxx" {
		t.Errorf("a long word %q", got)
	}
	if !validLog("20261112-090015.log") || validLog("../x.log") || validLog("20261112-090015.log/..") {
		t.Error("validLog")
	}
}
