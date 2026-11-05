package gemini

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

const jsonChat = `{
  "sessionId": "aaaaaaaa-1111-4222-8333-444444444444",
  "projectHash": "HASH",
  "startTime": "2026-11-05T10:00:00.000Z",
  "lastUpdated": "2026-11-05T10:05:00.000Z",
  "messages": [
    {"id": "m1", "timestamp": "2026-11-05T10:00:01.000Z", "type": "user", "content": [{"text": "review src/auth.ts"}, {"inlineData": {"mimeType": "image/png"}}]},
    {"id": "m2", "timestamp": "2026-11-05T10:00:02.000Z", "type": "gemini", "content": "Let me look.", "model": "gemini-2.5-pro",
     "thoughts": [{"subject": "s", "description": "secret thinking"}],
     "tokens": {"input": 100, "output": 20, "cached": 50, "thoughts": 5, "tool": 0, "total": 175},
     "toolCalls": [
       {"id": "c1", "name": "read_file", "args": {"absolute_path": "/home/test/proj/src/auth.ts"}, "status": "success"},
       {"id": "c2", "name": "run_shell_command", "args": {"command": "npm test"}, "status": "error"},
       {"id": "c3", "name": "write_todos", "args": {"todos": [{"description": "fix auth", "status": "in_progress"}, {"description": "add test", "status": "pending"}]}, "status": "success"}
     ]},
    {"id": "m3", "timestamp": "2026-11-05T10:00:03.000Z", "type": "info", "content": "Switched to fallback model"},
    {"id": "m4", "timestamp": "2026-11-05T10:00:04.000Z", "type": "error", "content": "You have exhausted your daily quota on this model."},
    {"id": "m5", "timestamp": "2026-11-05T10:00:05.000Z", "type": "user", "content": "/stats"},
    {"id": "m6", "timestamp": "2026-11-05T10:00:06.000Z", "type": "user", "content": "expanded @file content here", "displayContent": "check @README.md çok iyi"},
    {"id": "m7", "timestamp": "2026-11-05T10:00:07.000Z", "type": "mystery", "content": "x"}
  ]
}`

func writeChat(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, "chats", name)
	testutil.WriteFile(t, p, []byte(body))
	return p
}

func TestLoadJSON(t *testing.T) {
	h := testutil.Isolate(t)
	dir := filepath.Join(h.Gemini, "tmp", ProjectHash(h.Project))
	p := writeChat(t, dir, "session-2026-11-05T10-00-aaaaaaaa.json", jsonChat)
	s, err := New(session.Env{}).Load(context.Background(), session.Ref{Path: p}, session.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "aaaaaaaa-1111-4222-8333-444444444444" || s.Model != "gemini-2.5-pro" {
		t.Fatalf("meta %+v", s.Ref)
	}
	var got []string
	for _, e := range s.Entries {
		got = append(got, string(rune('0'+e.Role))+string(rune('0'+e.Kind))+":"+e.Text)
	}
	want := []string{
		"00:review src/auth.ts\n[attachment]",
		"10:Let me look.",
		"20:Switched to fallback model",
		"01:/stats",
		"00:check @README.md çok iyi",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
	tools := s.Entries[1].Tools
	if len(tools) != 3 || tools[0].Failed || !tools[1].Failed || tools[0].Args["absolute_path"] == nil {
		t.Fatalf("tools %+v", tools)
	}
	if len(s.Todos) != 2 || s.Todos[0].Status != session.TodoInProgress {
		t.Fatalf("todos %+v", s.Todos)
	}
	if len(s.Errors) != 1 || !s.Errors[0].LimitLike() {
		t.Fatalf("errors %+v", s.Errors)
	}
	if s.Usage.InputTokens != 100 || s.Usage.CacheReadTokens != 50 || s.Usage.Replies != 1 {
		t.Fatalf("usage %+v", s.Usage)
	}
	if !strings.Contains(strings.Join(s.Warnings, ";"), "unknown message type") {
		t.Fatalf("warnings %v", s.Warnings)
	}
	for _, e := range s.Entries {
		if strings.Contains(e.Text, "secret thinking") {
			t.Fatal("thoughts leaked")
		}
	}
}

const jsonlChat = `{"kind":"main","sessionId":"bbbbbbbb-1111-4222-8333-444444444444","projectHash":"HASH","startTime":"2026-11-05T11:00:00.000Z","lastUpdated":"2026-11-05T11:00:00.000Z"}
{"$set":{"lastUpdated":"2026-11-05T11:00:01.000Z","messages":[{"id":"a","timestamp":"2026-11-05T11:00:01.000Z","type":"user","content":"first"}]}}
{"$set":{"lastUpdated":"2026-11-05T11:00:02.000Z","messages":[{"id":"a","timestamp":"2026-11-05T11:00:01.000Z","type":"user","content":"first"},{"id":"b","timestamp":"2026-11-05T11:00:02.000Z","type":"gemini","content":"partial"}]}}
{"$set":{"lastUpdated":"2026-11-05T11:00:03.000Z","messages":[{"id":"b","timestamp":"2026-11-05T11:00:02.000Z","type":"gemini","content":"complete answer"}]}}
{"id":"c","timestamp":"2026-11-05T11:00:04.000Z","type":"user","content":[{"text":"bare line"}]}
not json
{"weird":true}
`

func TestLoadJSONL(t *testing.T) {
	h := testutil.Isolate(t)
	dir := filepath.Join(h.Gemini, "tmp", "proj-short")
	p := writeChat(t, dir, "session-2026-11-05T11-00-bbbbbbbb.jsonl", jsonlChat)
	s, err := New(session.Env{}).Load(context.Background(), session.Ref{Path: p}, session.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "bbbbbbbb-1111-4222-8333-444444444444" || !s.StartedAt.Equal(time.Date(2026, 11, 5, 11, 0, 0, 0, time.UTC)) {
		t.Fatalf("meta %+v", s.Ref)
	}
	var got []string
	for _, e := range s.Entries {
		got = append(got, e.Text)
	}
	if strings.Join(got, "|") != "first|complete answer|bare line" {
		t.Fatalf("got %q", got)
	}
	w := strings.Join(s.Warnings, ";")
	if !strings.Contains(w, "1 malformed") || !strings.Contains(w, "1 unknown line") {
		t.Fatalf("warnings %v", s.Warnings)
	}
}

func TestProjectDirLookup(t *testing.T) {
	h := testutil.Isolate(t)
	p := New(session.Env{}).(*Provider)
	// (1) registry → short id
	short := filepath.Join(h.Gemini, "tmp", "proj")
	writeChat(t, short, "session-2026-11-05T12-00-cccccccc.json", `{"sessionId":"c1","messages":[]}`)
	testutil.WriteFile(t, filepath.Join(h.Gemini, "projects.json"), []byte(`{"projects":{"`+h.Project+`":"proj","/home/test/evil":"../../etc"}}`))
	// (2) legacy sha256 dir
	legacy := filepath.Join(h.Gemini, "tmp", ProjectHash(h.Project))
	writeChat(t, legacy, "session-2026-11-04T12-00-dddddddd.json", `{"sessionId":"d1","messages":[]}`)
	// (3) .project_root scan
	other := filepath.Join(h.Gemini, "tmp", "zz-other")
	testutil.WriteFile(t, filepath.Join(other, ".project_root"), []byte(h.Project+"\n"))
	writeChat(t, other, "session-2026-11-03T12-00-eeeeeeee.jsonl", `{"kind":"main","sessionId":"e1"}`+"\n")
	// unrelated project
	unrelated := filepath.Join(h.Gemini, "tmp", "unrelated")
	testutil.WriteFile(t, filepath.Join(unrelated, ".project_root"), []byte("/home/test/else"))
	writeChat(t, unrelated, "session-2026-11-05T13-00-ffffffff.json", `{"sessionId":"f1","messages":[]}`)

	dirs := p.ProjectDirs(h.Project)
	if len(dirs) != 3 || dirs[0] != short || dirs[1] != legacy || dirs[2] != other {
		t.Fatalf("dirs %q", dirs)
	}
	refs, err := p.Find(context.Background(), session.Query{CWD: h.Project})
	if err != nil || len(refs) != 3 {
		t.Fatalf("%v %+v", err, refs)
	}
	ids := map[string]bool{}
	for _, r := range refs {
		ids[r.ID] = true
		if r.CWD != h.Project {
			t.Fatalf("cwd %q", r.CWD)
		}
	}
	if !ids["c1"] || !ids["d1"] || !ids["e1"] {
		t.Fatalf("ids %v", ids)
	}
	refs, _ = p.Find(context.Background(), session.Query{CWD: h.Project, ID: "d1"})
	if len(refs) != 1 || refs[0].ID != "d1" {
		t.Fatalf("by id %+v", refs)
	}
	refs, _ = p.Find(context.Background(), session.Query{})
	if len(refs) != 4 {
		t.Fatalf("all: %d", len(refs))
	}
}

func TestPreview(t *testing.T) {
	h := testutil.Isolate(t)
	dir := filepath.Join(h.Gemini, "tmp", "p")
	pj := writeChat(t, dir, "session-2026-11-05T10-00-aaaaaaaa.json", jsonChat)
	pl := writeChat(t, dir, "session-2026-11-05T11-00-bbbbbbbb.jsonl", jsonlChat)
	p := New(session.Env{}).(*Provider)
	if got, err := p.Preview(context.Background(), session.Ref{Path: pj}); err != nil || got != "review src/auth.ts\n[attachment]" {
		t.Fatalf("%q %v", got, err)
	}
	if got, err := p.Preview(context.Background(), session.Ref{Path: pl}); err != nil || got != "first" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestOversizedJSON(t *testing.T) {
	h := testutil.Isolate(t)
	p := filepath.Join(h.Gemini, "tmp", "x", "chats", "session-big.json")
	testutil.WriteFile(t, p, []byte("{}"))
	f, _ := os.OpenFile(p, os.O_RDWR, 0o600)
	_ = f.Truncate(MaxJSONBytes + 1)
	f.Close()
	s, err := New(session.Env{}).Load(context.Background(), session.Ref{Path: p}, session.LoadOptions{})
	if err != nil || len(s.Warnings) != 1 {
		t.Fatalf("%v %v", err, s)
	}
}

func FuzzGeminiLoad(f *testing.F) {
	f.Add([]byte(jsonChat), false)
	f.Add([]byte(jsonlChat), true)
	f.Add([]byte(`{"messages":[{"type":"user","content":{"text":1}}]}`), false)
	dir := f.TempDir()
	f.Fuzz(func(t *testing.T, data []byte, isJSONL bool) {
		name := "session-x.json"
		if isJSONL {
			name += "l"
		}
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Skip()
		}
		pr := New(session.Env{Home: dir})
		_, _ = pr.Load(context.Background(), session.Ref{Path: p}, session.LoadOptions{})
		_, _ = pr.(*Provider).Preview(context.Background(), session.Ref{Path: p})
	})
}
