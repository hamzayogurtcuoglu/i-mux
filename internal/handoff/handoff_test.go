package handoff

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/gitinfo"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

var update = flag.Bool("update", false, "rewrite golden files")

var (
	t0  = time.Date(2026, 11, 5, 14, 0, 0, 0, time.UTC)
	cwd = "/home/test/proj"
)

func opts() Options {
	return Options{
		From: session.Claude, To: session.Antigravity,
		Now: time.Date(2026, 11, 5, 14, 32, 0, 0, time.UTC), Location: time.UTC,
		IncludeSourcePath: true, Redact: true,
		Git: &gitinfo.Info{
			State: gitinfo.OK, Branch: "feature/login", Status: []string{" M src/auth.ts", "?? src/new.ts"}, StatusTotal: 2,
			DiffStat: []string{" src/auth.ts | 4 ++--", " 1 file changed, 2 insertions(+), 2 deletions(-)"}, DiffTotal: 2,
		},
	}
}

func at(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }

func user(min int, text string) session.Entry {
	return session.Entry{Role: session.RoleUser, At: at(min), Text: text}
}

func asst(min int, text string, tools ...session.ToolCall) session.Entry {
	return session.Entry{Role: session.RoleAssistant, At: at(min), Text: text, Tools: tools}
}

func tc(name string, args map[string]any) session.ToolCall {
	return session.ToolCall{Name: name, Args: args}
}

func shortSession() *session.Session {
	s := &session.Session{
		Ref:   session.Ref{Agent: session.Claude, ID: "s-short", Path: "/home/test/.claude/projects/-home-test-proj/s-short.jsonl", CWD: cwd},
		Model: "claude-opus-4-1",
		Entries: []session.Entry{
			user(0, "fix the login redirect bug"),
			asst(1, "Looking at the auth handler.",
				tc("Read", map[string]any{"file_path": cwd + "/src/auth.ts"}),
				tc("Bash", map[string]any{"command": "npm test"})),
			asst(2, "", tc("Edit", map[string]any{"file_path": cwd + "/src/auth.ts"})),
			asst(3, "The redirect now keeps the `next` parameter. Tests pass."),
			user(4, "great, also add a test for it. Token: ghp_"+strings.Repeat("Ab1", 12)),
		},
		Todos: []session.Todo{{Text: "find the bug", Status: session.TodoDone}, {Text: "add a test", Status: session.TodoInProgress}, {Text: "update docs"}},
	}
	return s
}

func longSession(n int) *session.Session {
	s := &session.Session{Ref: session.Ref{Agent: session.Claude, ID: "s-long", Path: "/home/test/.claude/projects/-home-test-proj/s-long.jsonl", CWD: cwd}, Model: "claude-sonnet-4"}
	s.Entries = append(s.Entries, user(0, "build a rate limiter for the API ğüşıöç "+strings.Repeat("details ", 50)))
	for i := 1; i < n; i++ {
		s.Entries = append(s.Entries,
			asst(i, fmt.Sprintf("Step %d: %s", i, strings.Repeat("working on it 🎉 ", 30)),
				tc("Bash", map[string]any{"command": fmt.Sprintf("go test ./pkg%d/...", i)}),
				tc("Edit", map[string]any{"file_path": fmt.Sprintf("%s/pkg%d/limiter.go", cwd, i)})),
			user(i, fmt.Sprintf("ok, continue with step %d", i+1)))
	}
	return s
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/handoff -update)", err)
	}
	if string(want) != got {
		t.Fatalf("%s differs from golden file; run with -update to accept.\n--- got ---\n%s", name, got)
	}
}

func build(t *testing.T, src []*session.Session, o Options) *Doc {
	t.Helper()
	d, err := Build(src, o)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(d.Text, session.MarkerPrefix) {
		t.Fatal("marker missing")
	}
	if d.Runes > o.Budget && o.Budget > 0 || utf8.RuneCountInString(d.Text) != d.Runes {
		t.Fatalf("runes %d over budget %d", d.Runes, o.Budget)
	}
	return d
}

func TestGoldenShort(t *testing.T) {
	o := opts()
	o.Reason = Reason{Limit: true, ResetAt: time.Date(2026, 11, 5, 15, 0, 0, 0, time.UTC)}
	d := build(t, []*session.Session{shortSession()}, o)
	if d.Redactions != 2 { // conversation + "Where it stopped"
		t.Fatalf("redactions %d", d.Redactions)
	}
	golden(t, "short", d.Text)
}

func TestGoldenLong(t *testing.T) {
	o := opts()
	o.Budget = 8000
	d := build(t, []*session.Session{longSession(120)}, o)
	if d.Omitted == 0 || d.Included == 0 {
		t.Fatalf("included %d omitted %d", d.Included, d.Omitted)
	}
	golden(t, "long", d.Text)
}

func TestGoldenCompacted(t *testing.T) {
	s := shortSession()
	s.Entries = s.Entries[3:]
	s.Omitted = 7
	s.Compaction = &session.Compaction{At: at(2), Trigger: "auto", Summary: "The user asked to fix the login redirect bug. The handler in src/auth.ts dropped the next parameter."}
	golden(t, "compacted", build(t, []*session.Session{s}, opts()).Text)
}

func TestGoldenChained(t *testing.T) {
	first := build(t, []*session.Session{shortSession()}, opts()).Text
	s := &session.Session{
		Ref: session.Ref{Agent: session.Antigravity, ID: "g1", Path: "/home/test/.gemini/antigravity-cli/brain/g1/.system_generated/logs/transcript_full.jsonl", CWD: cwd},
		Entries: []session.Entry{
			{Role: session.RoleUser, Kind: session.KindHandoff, At: at(10), Text: first},
			asst(11, "Continuing: I added the test in src/auth.test.ts."),
			user(12, "now document it"),
		},
		Chained: session.ParseChained(first),
	}
	o := opts()
	o.From, o.To = session.Antigravity, session.Codex
	second := build(t, []*session.Session{s}, o).Text
	golden(t, "chained", second)
	if !strings.Contains(second, "## Original request\nfix the login redirect bug") {
		t.Fatal("original request lost")
	}
	// Chain of a chain: the original request survives a second hop.
	s2 := &session.Session{
		Ref:     session.Ref{Agent: session.Codex, ID: "x1", CWD: cwd},
		Entries: []session.Entry{{Role: session.RoleUser, Kind: session.KindHandoff, At: at(20), Text: second}, asst(21, "Docs updated.")},
		Chained: session.ParseChained(second),
	}
	o.From, o.To = session.Codex, session.Claude
	third := build(t, []*session.Session{s2}, o).Text
	golden(t, "chain-of-chain", third)
	if !strings.Contains(third, "## Original request\nfix the login redirect bug") || !strings.Contains(third, "now document it") {
		t.Fatal("chain of a chain lost history")
	}
	if strings.Count(third, "[i-mux handoff ·") != 1 {
		t.Fatal("nested marker leaked into the document")
	}
}

func TestGoldenDelta(t *testing.T) {
	g := &session.Session{Ref: session.Ref{Agent: session.Antigravity, ID: "g", CWD: cwd}, Entries: []session.Entry{
		user(30, "rename the helper"), asst(31, "Renamed to parseNext.", tc("replace", map[string]any{"file_path": cwd + "/src/auth.ts"})),
	}}
	x := &session.Session{Ref: session.Ref{Agent: session.Codex, ID: "x", CWD: cwd}, Entries: []session.Entry{
		user(40, "run the tests"), asst(41, "All green.", tc("shell", map[string]any{"command": []any{"bash", "-lc", "npm test"}})),
	}}
	o := opts()
	o.From, o.To, o.Delta = session.Codex, session.Claude, true
	d := build(t, []*session.Session{g, x}, o)
	golden(t, "delta", d.Text)
	for _, absent := range []string{"## Original request", "## Current task list", "Full transcript"} {
		if strings.Contains(d.Text, absent) {
			t.Fatalf("delta must not contain %q", absent)
		}
	}
}

func TestGoldenEmptyAndNonGit(t *testing.T) {
	o := opts()
	o.Git = &gitinfo.Info{State: gitinfo.NotRepo}
	s := &session.Session{Ref: session.Ref{Agent: session.Claude, CWD: cwd}}
	golden(t, "empty-nongit", build(t, []*session.Session{s}, o).Text)
}

func TestReconstructed(t *testing.T) {
	s := &session.Session{
		Ref: session.Ref{Agent: session.Codex, CWD: cwd}, Reconstructed: true,
		Entries: []session.Entry{{Role: session.RoleNotice, Text: "> do things\n• done"}},
	}
	d := build(t, []*session.Session{s}, opts())
	if !strings.Contains(d.Text, "reconstructed from terminal scrollback") || strings.Contains(d.Text, "Full transcript") {
		t.Fatal(d.Text)
	}
}

func TestByteGuard(t *testing.T) {
	o := opts()
	o.Budget = 60000
	o.MaxArgBytes = 20000
	d := build(t, []*session.Session{longSession(200)}, o)
	if d.Bytes > o.MaxArgBytes {
		t.Fatalf("bytes %d > %d", d.Bytes, o.MaxArgBytes)
	}
}

func TestPropertyRandomSessions(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	alphabet := []string{"a", "b", " ", "\n", "ğ", "ü", "ş", "ı", "🎉", "`", "$(", "\"", "##", "**"}
	word := func(n int) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteString(alphabet[rng.Intn(len(alphabet))])
		}
		return b.String()
	}
	for iter := 0; iter < 150; iter++ {
		s := &session.Session{Ref: session.Ref{Agent: session.AllAgents[rng.Intn(4)], CWD: cwd, Path: "/home/test/x.jsonl"}}
		n := rng.Intn(80)
		for i := 0; i < n; i++ {
			e := session.Entry{Role: session.Role(rng.Intn(3)), At: at(i), Text: word(rng.Intn(3000))}
			for k := rng.Intn(20); k > 0; k-- {
				e.Tools = append(e.Tools, tc([]string{"Bash", "Edit", "read_file", "shell", "mcp__a__b", "x"}[rng.Intn(6)],
					map[string]any{"command": word(rng.Intn(300)), "file_path": cwd + "/" + word(10)}))
			}
			s.Entries = append(s.Entries, e)
		}
		if rng.Intn(4) == 0 {
			s.Compaction = &session.Compaction{Summary: word(rng.Intn(20000))}
		}
		if rng.Intn(4) == 0 {
			s.Todos = []session.Todo{{Text: word(500)}, {Text: word(5000), Status: session.TodoDone}}
		}
		o := opts()
		o.Budget = 2000 + rng.Intn(60000)
		o.MaxArgBytes = 4000 + rng.Intn(120000)
		o.Delta = rng.Intn(5) == 0
		d1, err := Build([]*session.Session{s}, o)
		if err != nil {
			t.Fatal(err)
		}
		if d1.Runes > o.Budget || d1.Bytes > o.MaxArgBytes {
			t.Fatalf("iter %d: runes %d/%d bytes %d/%d", iter, d1.Runes, o.Budget, d1.Bytes, o.MaxArgBytes)
		}
		if !utf8.ValidString(d1.Text) || !strings.HasPrefix(d1.Text, session.MarkerPrefix) {
			t.Fatalf("iter %d: invalid output", iter)
		}
		d2, _ := Build([]*session.Session{s}, o)
		if d1.Text != d2.Text {
			t.Fatalf("iter %d: not deterministic", iter)
		}
	}
}

func BenchmarkBuild(b *testing.B) {
	s := longSession(2000)
	o := opts()
	for i := 0; i < b.N; i++ {
		if _, err := Build([]*session.Session{s}, o); err != nil {
			b.Fatal(err)
		}
	}
}
