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
// and environment in dir and writes dir/out to stdout and dir/stderr to
// stderr, then exits with dir/exit. No tmux runs: the client's binary is
// missing.
func fakeClaude(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" > " + dir + "/argv\n" +
		"pwd > " + dir + "/pwd\n" +
		"echo \"${NO_COLOR:-unset} ${TMUX:-unset}\" > " + dir + "/env\n" +
		"cat " + dir + "/out 2>/dev/null\n" +
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

// claudeReply is Claude Code's stream-json of a run that called a tool
// and answered report.
func claudeReply(report string) string {
	return `{"type":"system","subtype":"init","session_id":"s-claude","model":"claude-sonnet-5-5"}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"git ls-remote --tags origin"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","content":"abc refs/tags/v2.3","is_error":false}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":` + quoteJSON(report) + `}]}}
{"type":"result","subtype":"success","is_error":false,"result":` + quoteJSON(report) + `,"session_id":"s-claude"}
`
}

func quoteJSON(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
}

// TestAutoExec (T66.2, T66.6): a run starts the definition's CLI headless
// in its project with its streaming output; the answer, the conversation
// and the command line are read from it and kept (the next run gets the
// answer); a quiet run, a failure the stream reports and one only stderr
// tells; one already going on or not due is not started; a definition
// that went is skipped. The output view prints the run's steps.
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
		time.Sleep(1100 * time.Millisecond) // one log a second
		if _, errOut, code := runCLI(t, "__auto", "exec", def.ID, "--manual"); code != 0 {
			t.Fatalf("exec: %d %s", code, errOut)
		}
	}
	set := func(out, stderr, exit string) {
		testutil.WriteFile(t, filepath.Join(fake, "out"), []byte(out))
		testutil.WriteFile(t, filepath.Join(fake, "stderr"), []byte(stderr))
		testutil.WriteFile(t, filepath.Join(fake, "exit"), []byte(exit))
	}

	set(claudeReply("## v2.3 is out\nThe pipeline started."), "", "0")
	exec()
	r := auto.Last(def.ID)
	if r == nil || r.Status != auto.StatusOK || r.Summary != "v2.3 is out" || !strings.Contains(r.Report, "The pipeline started.") ||
		r.CLI != "claude" || r.Model != "haiku" || !r.Manual || r.Session != "s-claude" || r.End.Before(r.Start) {
		t.Fatalf("run %+v", r)
	}
	argv := strings.Split(readFile(t, filepath.Join(fake, "argv")), "\n")
	if strings.Join(argv[:5], " ") != "-p --output-format stream-json --verbose --session-id" || strings.Join(argv[6:11], " ") != "--agent sim-tag --model haiku --dangerously-skip-permissions" {
		t.Fatalf("argv %q", argv)
	}
	if want := " -p --output-format stream-json --verbose --session-id " + argv[5] + " --agent sim-tag --model haiku --dangerously-skip-permissions <prompt>"; !strings.HasSuffix(r.Command, want) {
		t.Errorf("command %q", r.Command)
	}
	prompt := strings.Join(argv[11:], "\n")
	if strings.Contains(prompt, "Compare the tags.") || !strings.Contains(prompt, "started by hand") || !strings.Contains(prompt, "This is the first run.") {
		t.Errorf("prompt:\n%s", prompt)
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(fake, "pwd"))); got != root {
		t.Errorf("ran in %q", got)
	}
	if got := strings.TrimSpace(readFile(t, filepath.Join(fake, "env"))); got != "1 unset" {
		t.Errorf("NO_COLOR and TMUX: %q", got)
	}
	if log := readFile(t, auto.LogPath(def.ID, r.Log)); log != claudeReply("## v2.3 is out\nThe pipeline started.") {
		t.Errorf("log:\n%s", log)
	}
	// The output view, printed: the header, the tool and the answer.
	if out, errOut, code := runCLI(t, "__auto", "show", def.ID); code != 0 || !strings.Contains(out, "✓ sim-tag · Claude · haiku") ||
		!strings.Contains(out, "● Bash git ls-remote --tags origin") || !strings.Contains(out, "⎿ abc refs/tags/v2.3") || !strings.Contains(out, " v2.3 is out") {
		t.Errorf("show %d %s:\n%s", code, errOut, out)
	}

	set(claudeReply(auto.NothingNew), "", "0")
	exec()
	if r := auto.Last(def.ID); r.Status != auto.StatusQuiet || r.Report != "" {
		t.Errorf("quiet run %+v", r)
	}
	if p := readFile(t, filepath.Join(fake, "argv")); !strings.Contains(p, "Your report from the last run") || !strings.Contains(p, "The pipeline started.") {
		t.Errorf("the last report is not passed on:\n%s", p)
	}

	set(`{"type":"result","subtype":"success","is_error":true,"result":"Claude AI usage limit reached"}`+"\n", "", "1")
	exec()
	if r := auto.Last(def.ID); r.Status != auto.StatusFailed || r.Exit != 1 || r.Summary != "Claude AI usage limit reached" {
		t.Errorf("failed run %+v", r)
	}
	set("", "error: unknown option '--output-format'\n", "2")
	exec()
	if r := auto.Last(def.ID); r.Status != auto.StatusFailed || r.Summary != "error: unknown option '--output-format'" {
		t.Errorf("stderr failure %+v", r)
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
	if n := len(auto.Runs(def.ID)); n != 5 {
		t.Errorf("%d runs", n)
	}
	// On schedule: not when it is not due (it ran a moment ago).
	if _, err := auto.Change(def.ID, nil, "", time.Now(), func(s *auto.Schedule) { s.When = "5m" }); err != nil {
		t.Fatal(err)
	}
	if _, errOut, code := runCLI(t, "__auto", "exec", def.ID); code != 0 || len(auto.Runs(def.ID)) != 5 {
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
	if !validLog("20261112-090015.log") || validLog("../x.log") || validLog("20261112-090015.log/..") {
		t.Error("validLog")
	}
}
