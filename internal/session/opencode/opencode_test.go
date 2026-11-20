package opencode

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// A synthetic export in the shape OpenCode 2.2.6 writes (§7.7).
const exportJSON = `{
  "info": {
    "id": "ses_imuxaaaa",
    "model": {"id": "kimi-k2", "providerID": "nvidia"},
    "tokens": {"input": 1200, "output": 80, "cache": {"read": 300, "write": 0}},
    "time": {"created": 1794087849671, "updated": 1794087849985},
    "title": "fix the login",
    "location": {"directory": "/home/u/proj"}
  },
  "messages": [
    {"type": "user", "time": {"created": 1794087849872}, "text": "fix the login redirect", "files": []},
    {"type": "assistant", "time": {"created": 1794087849889}, "model": {"id": "kimi-k2", "providerID": "nvidia"},
     "content": [
       {"type": "text", "text": "Let me look. "},
       {"type": "tool", "id": "c1", "name": "read", "state": {"status": "completed", "input": {"path": "/home/u/proj/src/login.ts"}}},
       {"type": "tool", "id": "c2", "name": "bash", "state": {"status": "error", "input": {"command": "npm test"}}},
       {"type": "tool", "id": "c3", "name": "todowrite", "state": {"status": "completed", "input": {"todos": [{"content": "fix redirect", "status": "in_progress"}, {"content": "add test", "status": "pending"}]}}}
     ]},
    {"type": "idle", "time": {"created": 1794087849900}},
    {"type": "assistant", "time": {"created": 1794087849950}, "content": [], "error": {"name": "APIError", "data": {"message": "Rate limit exceeded"}}},
    {"type": "assistant", "time": {"created": 1794087849960}, "content": [{"type": "text", "text": "Fixed the redirect."}]}
  ]
}`

func TestParse(t *testing.T) {
	s, err := Parse([]byte(exportJSON), session.Ref{ID: "ses_imuxaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	if s.CWD != "/home/u/proj" || s.Model != "nvidia/kimi-k2" || s.StartedAt.IsZero() || s.Agent != session.OpenCode {
		t.Fatalf("meta %+v", s.Ref)
	}
	if len(s.Entries) != 3 || s.Entries[0].Role != session.RoleUser || s.Entries[0].Text != "fix the login redirect" {
		t.Fatalf("entries %+v", s.Entries)
	}
	a := s.Entries[1]
	if a.Text != "Let me look." || len(a.Tools) != 3 || a.Tools[0].Name != "read" || a.Tools[0].Args["file_path"] != "/home/u/proj/src/login.ts" ||
		!a.Tools[1].Failed || a.Tools[1].Args["command"] != "npm test" {
		t.Fatalf("assistant %+v", a)
	}
	if len(s.Todos) != 2 || s.Todos[0].Status != session.TodoInProgress {
		t.Fatalf("todos %+v", s.Todos)
	}
	if len(s.Errors) != 1 || s.Errors[0].Text != "Rate limit exceeded" || !s.Errors[0].LimitLike() {
		t.Fatalf("errors %+v", s.Errors)
	}
	if s.Usage.Replies != 3 || s.Usage.InputTokens != 1200 || s.Usage.CacheReadTokens != 300 {
		t.Fatalf("usage %+v", s.Usage)
	}
	// A handoff as the first message: chained.
	h := strings.Replace(exportJSON, "fix the login redirect", "[i-mux handoff · Claude Code → OpenCode · 2026-11-08 10:00]\\n\\n## Original request\\nbuild it\\n## What to do now\\nwait", 1)
	s, _ = Parse([]byte(h), session.Ref{})
	if s.Chained == nil || s.Chained.OriginalRequest != "build it" || s.Entries[0].Kind != session.KindHandoff || s.FirstUserText() != "build it" {
		t.Fatalf("chained %+v", s.Chained)
	}
	if _, err := Parse([]byte("nope"), session.Ref{}); err == nil {
		t.Fatal("junk")
	}
}

// Find and Preview read a database with the sqlite3 CLI; Load runs the
// opencode CLI (a fake here).
func TestFindLoadPreview(t *testing.T) {
	sq, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 not installed")
	}
	home := t.TempDir()
	db := filepath.Join(home, "opencode.db")
	now := time.Now()
	ms := func(d time.Duration) string { return itoa(now.Add(-d).UnixMilli()) }
	sql := `create table session_v2 (id text primary key, directory text, title text, parent_id text, time_archived integer, time_created integer, time_updated integer, tokens_input integer, tokens_output integer);
create table session_message (id text, session_id text, type text, seq integer, data text);
insert into session_v2 values ('ses_a', '/home/u/proj', 'a', null, null, ` + ms(3*time.Hour) + `, ` + ms(2*time.Hour) + `, 100, 10);
insert into session_v2 values ('ses_b', '/home/u/proj', 'b', null, null, ` + ms(time.Hour) + `, ` + ms(time.Minute) + `, 200, 20);
insert into session_v2 values ('ses_child', '/home/u/proj', 'c', 'ses_b', null, ` + ms(time.Hour) + `, ` + ms(time.Minute) + `, 5, 5);
insert into session_v2 values ('ses_other', '/home/u/other', 'o', null, null, ` + ms(time.Hour) + `, ` + ms(time.Minute) + `, 1, 1);
insert into session_message values ('m1', 'ses_b', 'user', 1, '{"text":"first ask"}');`
	if out, err := exec.Command(sq, db, sql).CombinedOutput(); err != nil {
		t.Fatalf("sqlite3: %v %s", err, out)
	}
	fake := filepath.Join(home, "opencode")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\ncat <<'X'\n"+exportJSON+"\nX\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := &Provider{Env: session.Env{OpenCodeHome: home, OpenCodeBin: fake}}
	ctx := context.Background()
	refs, err := p.Find(ctx, session.Query{CWD: "/home/u/proj"})
	if err != nil || len(refs) != 2 || refs[0].ID != "ses_b" || refs[1].ID != "ses_a" || refs[0].Path != db {
		t.Fatalf("find %+v %v", refs, err)
	}
	if refs, _ := p.Find(ctx, session.Query{CWD: "/home/u/proj", Since: now.Add(-30 * time.Minute)}); len(refs) != 1 || refs[0].ID != "ses_b" {
		t.Fatalf("since %+v", refs)
	}
	if refs, _ := p.Find(ctx, session.Query{ID: "ses_a"}); len(refs) != 1 || refs[0].CWD != "/home/u/proj" {
		t.Fatalf("by id %+v", refs)
	}
	if got, err := p.Preview(ctx, refs[0]); err != nil || got != "first ask" {
		t.Fatalf("preview %q %v", got, err)
	}
	s, err := p.Load(ctx, refs[0], session.LoadOptions{})
	if err != nil || s.Model != "nvidia/kimi-k2" || len(s.Entries) != 3 {
		t.Fatalf("load %+v %v", s, err)
	}
	in, out, n, err := p.Tokens(ctx, now.Add(-90*time.Minute))
	if err != nil || in != 206 || out != 26 || n != 3 {
		t.Fatalf("tokens %d %d %d %v", in, out, n, err)
	}
	// An id: SQL quotes are escaped.
	if refs, _ := p.Find(ctx, session.Query{ID: "x' or '1'='1"}); len(refs) != 0 {
		t.Fatalf("injection %+v", refs)
	}
}

func itoa(n int64) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

// A synthetic answer of `opencode api experimental.usage.limits` with a Go
// plan (D80).
const goLimitsJSON = `{"location":{"directory":"/home/u"},"data":{"workspace":{"id":"wrk_1","name":"Personal","usageURL":"https://example.invalid/usage"},` +
	`"go":{"rolling":{"percent":39,"limited":false,"resetsAt":1794865519000},` +
	`"weekly":{"percent":15,"limited":false,"resetsAt":1795392000000},` +
	`"monthly":{"percent":100,"limited":true,"resetsAt":"2026-12-14T10:31:01Z"}},"budgets":[]}}`

func TestParseGoUsage(t *testing.T) {
	g := ParseGoUsage([]byte(goLimitsJSON))
	if g == nil || g.FiveHour == nil || g.Weekly == nil || g.Monthly == nil {
		t.Fatalf("parse %+v", g)
	}
	if g.FiveHour.UsedPercent != 39 || g.FiveHour.Limited || !g.FiveHour.ResetsAt.Equal(time.UnixMilli(1794865519000)) {
		t.Fatalf("5-hour %+v", g.FiveHour)
	}
	if g.Weekly.UsedPercent != 15 || !g.Monthly.Limited || g.Monthly.ResetsAt.UTC().Format(time.RFC3339) != "2026-12-14T10:31:01Z" {
		t.Fatalf("weekly %+v monthly %+v", g.Weekly, g.Monthly)
	}
	if len(g.Windows()) != 3 {
		t.Fatalf("windows %d", len(g.Windows()))
	}
	// A line before the JSON (a warning) is skipped; a percent out of range is clamped.
	if g := ParseGoUsage([]byte("note: experimental\n" + `{"data":{"go":{"weekly":{"percent":140,"limited":true,"resetsAt":0}}}}`)); g == nil || g.Weekly.UsedPercent != 100 || g.FiveHour != nil || !g.Weekly.ResetsAt.IsZero() {
		t.Fatalf("partial %+v", g)
	}
	for _, in := range []string{
		`{"location":{},"data":null}`,                              // not signed in
		`{"data":{"workspace":{"id":"w"},"go":null,"budgets":[]}}`, // no Go plan
		`{"data":{"go":{"rolling":{"limited":false}}}}`,            // no percent
		`{"data":{"go":{}}}`,
		`not json`,
		``,
	} {
		if g := ParseGoUsage([]byte(in)); g != nil {
			t.Fatalf("%q: %+v", in, g)
		}
	}
}

func TestGoUsageRunsOpenCode(t *testing.T) {
	testutil.SkipOnWindows(t, "the fake opencode is an sh script")
	dir := t.TempDir()
	argv := filepath.Join(dir, "argv")
	fake := filepath.Join(dir, "opencode")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho \"$@\" > '"+argv+"'\ncat <<'X'\n"+goLimitsJSON+"\nX\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := &Provider{Env: session.Env{OpenCodeHome: dir, OpenCodeBin: fake}}
	g, err := p.GoUsage(context.Background())
	if err != nil || g == nil || g.FiveHour.UsedPercent != 39 {
		t.Fatalf("go usage %+v %v", g, err)
	}
	if b, _ := os.ReadFile(argv); strings.TrimSpace(string(b)) != "api experimental.usage.limits" {
		t.Fatalf("argv %q", b)
	}
	// A failing command is an error, and no usage.
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho 'no server' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if g, err := p.GoUsage(context.Background()); err == nil || g != nil {
		t.Fatalf("failure %+v %v", g, err)
	}
	// One that hangs is cut at GoTimeout.
	old := GoTimeout
	GoTimeout = 200 * time.Millisecond
	t.Cleanup(func() { GoTimeout = old })
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexec sleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if g, err := p.GoUsage(context.Background()); err == nil || g != nil || time.Since(start) > 3*time.Second {
		t.Fatalf("timeout %+v %v after %s", g, err, time.Since(start))
	}
}
