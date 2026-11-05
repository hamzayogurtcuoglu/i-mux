package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

const sid = "11111111-2222-4333-8444-555555555555"

// lb builds synthetic Claude session lines.
type lb struct {
	lines []string
	n     int
	t     time.Time
	cwd   string
}

func newLB(cwd string) *lb {
	return &lb{t: time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC), cwd: cwd}
}

func (b *lb) add(m map[string]any) string {
	b.n++
	b.t = b.t.Add(time.Second)
	uuid := fmt.Sprintf("u-%04d", b.n)
	base := map[string]any{
		"uuid": uuid, "sessionId": sid, "timestamp": b.t.Format(time.RFC3339Nano),
		"cwd": b.cwd, "gitBranch": "main", "version": "2.1.3", "isSidechain": false,
	}
	for k, v := range m {
		base[k] = v
	}
	j, _ := json.Marshal(base)
	b.lines = append(b.lines, string(j))
	return uuid
}

func (b *lb) user(content any) string {
	return b.add(map[string]any{"type": "user", "message": map[string]any{"role": "user", "content": content}})
}

func (b *lb) asst(id string, blocks ...map[string]any) string {
	return b.add(map[string]any{"type": "assistant", "message": map[string]any{
		"id": id, "role": "assistant", "model": "claude-opus-4-1", "content": blocks,
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "cache_read_input_tokens": 100, "cache_creation_input_tokens": 1},
	}})
}

func text(s string) map[string]any { return map[string]any{"type": "text", "text": s} }
func tool(id, name string, in map[string]any) map[string]any {
	return map[string]any{"type": "tool_use", "id": id, "name": name, "input": in}
}

func (b *lb) raw(s string) { b.lines = append(b.lines, s) }

func (b *lb) write(t *testing.T, h *testutil.Homes) session.Ref {
	t.Helper()
	p := filepath.Join(h.Claude, "projects", EncodeCWD(b.cwd), sid+".jsonl")
	testutil.WriteFile(t, p, []byte(strings.Join(b.lines, "\n")+"\n"))
	return session.Ref{Agent: session.Claude, ID: sid, Path: p}
}

func load(t *testing.T, ref session.Ref) *session.Session {
	t.Helper()
	s, err := New(session.Env{}).Load(context.Background(), ref, session.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestEncodeCWD(t *testing.T) {
	if got := EncodeCWD("/home/test/Documents/i-mux.v2"); got != "-home-test-Documents-i-mux-v2" {
		t.Fatalf("got %q", got)
	}
}

func TestLoadFull(t *testing.T) {
	h := testutil.Isolate(t)
	b := newLB(h.Project)
	b.add(map[string]any{"type": "file-history-snapshot", "snapshot": map[string]any{}})
	b.user("<system-reminder>ignore me</system-reminder>fix the login redirect bug")
	b.asst("msg_1", text("Looking at it."))
	b.asst("msg_1", tool("t1", "Read", map[string]any{"file_path": h.Project + "/src/auth.ts"}))
	b.asst("msg_1", tool("t2", "Bash", map[string]any{"command": "npm test"}))
	b.user([]map[string]any{{"type": "tool_result", "tool_use_id": "t1", "content": "ok"}})
	b.user([]map[string]any{{"type": "tool_result", "tool_use_id": "t2", "content": "FAIL", "is_error": true}})
	b.asst("msg_2", tool("t3", "TodoWrite", map[string]any{"todos": []any{
		map[string]any{"content": "old", "status": "pending"},
	}}))
	b.asst("msg_3", tool("t4", "TodoWrite", map[string]any{"todos": []any{
		map[string]any{"content": "find bug", "status": "completed", "activeForm": "Finding"},
		map[string]any{"content": "fix bug", "status": "in_progress"},
		map[string]any{"content": "test", "status": "pending"},
	}}))
	b.add(map[string]any{"type": "user", "isMeta": true, "message": map[string]any{"role": "user", "content": "meta caveat"}})
	b.add(map[string]any{"type": "user", "isSidechain": true, "message": map[string]any{"role": "user", "content": "sidechain prompt"}})
	b.user("<command-name>/review</command-name><command-args>auth</command-args>")
	b.user("<local-command-stdout>Total cost $1</local-command-stdout>")
	b.add(map[string]any{"type": "assistant", "isApiErrorMessage": true, "error": "rate_limit", "message": map[string]any{
		"id": "x", "role": "assistant", "model": "<synthetic>", "content": []any{text("API Error: Rate limit reached")},
	}})
	b.add(map[string]any{"type": "system", "subtype": "api_error", "error": map[string]any{"message": "Connection error", "rateLimits": nil}, "retryAttempt": 1})
	b.add(map[string]any{"type": "brand-new-type"})
	b.raw(`{"type":"user", broken`)
	b.asst("msg_4", text("Fixed the redirect. ğüşıöç 🎉"))
	s := load(t, b.write(t, h))

	if s.ID != sid || s.CWD != h.Project || s.Model != "claude-opus-4-1" || s.GitBranch != "main" {
		t.Fatalf("meta %+v", s.Ref)
	}
	var roles []string
	for _, e := range s.Entries {
		roles = append(roles, fmt.Sprintf("%d:%d:%s", e.Role, e.Kind, e.Text))
	}
	want := []string{
		"0:0:fix the login redirect bug",
		"1:0:Looking at it.",
		"1:0:", "1:0:",
		"0:1:/review auth",
		"1:0:Fixed the redirect. ğüşıöç 🎉",
	}
	if strings.Join(roles, "|") != strings.Join(want, "|") {
		t.Fatalf("entries\n%s\nwant\n%s", strings.Join(roles, "\n"), strings.Join(want, "\n"))
	}
	if n := len(s.Entries[1].Tools); n != 2 {
		t.Fatalf("merged entry has %d tools", n)
	}
	if s.Entries[1].Tools[0].Failed || !s.Entries[1].Tools[1].Failed {
		t.Fatal("tool failure mapping wrong")
	}
	if s.Usage.Replies != 4 || s.Usage.InputTokens != 40 || s.Usage.CacheReadTokens != 400 {
		t.Fatalf("usage %+v", s.Usage)
	}
	if len(s.Todos) != 3 || s.Todos[0].Status != session.TodoDone || s.Todos[1].Status != session.TodoInProgress {
		t.Fatalf("todos %+v", s.Todos)
	}
	if len(s.Errors) != 1 || s.Errors[0].Code != "rate_limit" || !s.Errors[0].LimitLike() {
		t.Fatalf("errors %+v", s.Errors)
	}
	joined := strings.Join(s.Warnings, ";")
	if !strings.Contains(joined, "1 unknown line type") || !strings.Contains(joined, "1 malformed line") {
		t.Fatalf("warnings %v", s.Warnings)
	}
	for _, e := range s.Entries {
		if strings.Contains(e.Text, "meta caveat") || strings.Contains(e.Text, "sidechain") || strings.Contains(e.Text, "Total cost") || strings.Contains(e.Text, "API Error") {
			t.Fatalf("excluded text leaked: %q", e.Text)
		}
	}
}

func TestCompaction(t *testing.T) {
	for _, withPreserved := range []bool{false, true} {
		t.Run(fmt.Sprint(withPreserved), func(t *testing.T) {
			h := testutil.Isolate(t)
			b := newLB(h.Project)
			b.user("first request")
			keep := b.asst("m1", text("old answer"))
			b.user("second")
			meta := map[string]any{"trigger": "auto", "preTokens": 150000}
			if withPreserved {
				meta["preservedMessages"] = map[string]any{"uuids": []string{keep}}
			}
			b.add(map[string]any{"type": "system", "subtype": "compact_boundary", "compactMetadata": meta})
			b.add(map[string]any{"type": "attachment", "attachment": map[string]any{}})
			b.add(map[string]any{"type": "attachment", "attachment": map[string]any{}})
			b.add(map[string]any{
				"type": "user", "isCompactSummary": true, "isVisibleInTranscriptOnly": true,
				"message": map[string]any{"role": "user", "content": "Summary: user asked for X; we did Y."},
			})
			b.user("after compaction")
			s := load(t, b.write(t, h))
			if s.Compaction == nil || s.Compaction.Trigger != "auto" || !strings.Contains(s.Compaction.Summary, "we did Y") {
				t.Fatalf("compaction %+v", s.Compaction)
			}
			var texts []string
			for _, e := range s.Entries {
				texts = append(texts, e.Text)
			}
			want := []string{"after compaction"}
			if withPreserved {
				want = []string{"old answer", "after compaction"}
			}
			if strings.Join(texts, "|") != strings.Join(want, "|") {
				t.Fatalf("entries %q want %q", texts, want)
			}
			if s.Omitted != 3-len(want)+1 {
				t.Fatalf("omitted %d", s.Omitted)
			}
		})
	}
}

func TestChainedHandoff(t *testing.T) {
	h := testutil.Isolate(t)
	b := newLB(h.Project)
	b.user(session.Marker(session.Gemini, session.Claude, testutil.FrozenTime) + "\n\nYou are continuing...\n\n## Original request\nbuild a CLI\n\n## Conversation (condensed)\n**User:** hi\n")
	b.asst("m1", text("Continuing."))
	s := load(t, b.write(t, h))
	if s.Chained == nil || s.Chained.From != session.Gemini || s.Chained.OriginalRequest != "build a CLI" {
		t.Fatalf("chained %+v", s.Chained)
	}
	if s.Entries[0].Kind != session.KindHandoff {
		t.Fatal("handoff entry not marked")
	}
	if s.FirstUserText() != "build a CLI" {
		t.Fatalf("first user text %q", s.FirstUserText())
	}
}

func TestFind(t *testing.T) {
	h := testutil.Isolate(t)
	p := New(session.Env{}).(*Provider)
	dir := filepath.Join(h.Claude, "projects", EncodeCWD(h.Project))
	old := filepath.Join(dir, "aaaaaaaa-0000-4000-8000-000000000001.jsonl")
	newer := filepath.Join(dir, "aaaaaaaa-0000-4000-8000-000000000002.jsonl")
	testutil.WriteFile(t, old, []byte(`{"type":"user"}`+"\n"))
	testutil.WriteFile(t, newer, []byte(`{"type":"user"}`+"\n"))
	testutil.WriteFile(t, filepath.Join(dir, "memory", "x.jsonl"), []byte("{}\n"))
	testutil.Touch(t, old, testutil.FrozenTime.Add(-time.Hour))
	testutil.Touch(t, newer, testutil.FrozenTime)
	refs, err := p.Find(context.Background(), session.Query{CWD: h.Project})
	if err != nil || len(refs) != 2 || refs[0].Path != newer {
		t.Fatalf("%v %+v", err, refs)
	}
	refs, _ = p.Find(context.Background(), session.Query{CWD: h.Project, Since: testutil.FrozenTime.Add(-time.Minute)})
	if len(refs) != 1 {
		t.Fatalf("since: %+v", refs)
	}
	refs, _ = p.Find(context.Background(), session.Query{ID: "aaaaaaaa-0000-4000-8000-000000000001"})
	if len(refs) != 1 || refs[0].Path != old {
		t.Fatalf("by id: %+v", refs)
	}
	// Fallback: a cwd whose folder encoding is unknown, found by "cwd".
	other := filepath.Join(h.Claude, "projects", "-weird-truncated-name-1a2b", "bbbbbbbb-0000-4000-8000-000000000003.jsonl")
	testutil.WriteFile(t, other, []byte(`{"type":"user","cwd":"/home/test/very/long/path"}`+"\n"))
	refs, _ = p.Find(context.Background(), session.Query{CWD: "/home/test/very/long/path"})
	if len(refs) != 1 || refs[0].Path != other {
		t.Fatalf("fallback: %+v", refs)
	}
}

func TestPreview(t *testing.T) {
	h := testutil.Isolate(t)
	b := newLB(h.Project)
	b.user("<command-name>/init</command-name>")
	b.user("<system-reminder>x</system-reminder>  add a rate limiter\nplease")
	b.asst("m", text("ok"))
	ref := b.write(t, h)
	got, err := New(session.Env{}).(*Provider).Preview(context.Background(), ref)
	if err != nil || got != "add a rate limiter\nplease" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestLoadSinceAndGap(t *testing.T) {
	h := testutil.Isolate(t)
	b := newLB(h.Project)
	b.user("start")
	for i := 0; i < 400; i++ {
		b.asst(fmt.Sprintf("m%d", i), text(strings.Repeat("z", 200)))
	}
	b.user("the end")
	ref := b.write(t, h)
	s, err := New(session.Env{}).Load(context.Background(), ref, session.LoadOptions{HeadBytes: 2000, TailBytes: 4000})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Gap || s.Entries[0].Text != "start" || s.Entries[len(s.Entries)-1].Text != "the end" || len(s.Entries) > 30 {
		t.Fatalf("gap=%v n=%d", s.Gap, len(s.Entries))
	}
	since := s.Entries[len(s.Entries)-1].At
	s, _ = New(session.Env{}).Load(context.Background(), ref, session.LoadOptions{Since: since})
	if len(s.Entries) != 1 || s.Entries[0].Text != "the end" {
		t.Fatalf("since: %d entries", len(s.Entries))
	}
}

func TestLoadMissingFile(t *testing.T) {
	testutil.Isolate(t)
	_, err := New(session.Env{}).Load(context.Background(), session.Ref{Path: "/home/test/none.jsonl"}, session.LoadOptions{})
	if err == nil {
		t.Fatal("want error")
	}
}

func FuzzClaudeLoad(f *testing.F) {
	f.Add([]byte(`{"type":"user","message":{"role":"user","content":"hi"}}` + "\n" +
		`{"type":"assistant","message":{"id":"m","model":"x","content":[{"type":"tool_use","id":"t","name":"TodoWrite","input":{"todos":[{"content":"a","status":"done"}]}}]}}` + "\n" +
		`{"type":"system","subtype":"compact_boundary","compactMetadata":{"preservedMessages":{"uuids":["a"]}}}`))
	f.Add([]byte(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t","is_error":true}]}}`))
	f.Add([]byte("\x00\xff{{{"))
	dir := f.TempDir()
	p := filepath.Join(dir, "s.jsonl")
	f.Fuzz(func(t *testing.T, data []byte) {
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Skip()
		}
		_, _ = New(session.Env{Home: dir}).Load(context.Background(), session.Ref{Path: p}, session.LoadOptions{HeadBytes: 64, TailBytes: 64})
		_, _ = New(session.Env{Home: dir}).Load(context.Background(), session.Ref{Path: p}, session.LoadOptions{})
	})
}
