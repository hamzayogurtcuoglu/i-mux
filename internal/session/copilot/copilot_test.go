package copilot

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

const events = `{"type":"session.start","id":"e1","timestamp":"2026-11-05T09:00:00.000Z","data":{"context":{"cwd":"/home/test/proj","branch":"feat/x","gitRoot":"/home/test/proj"},"selectedModel":"gpt-5","copilotVersion":"1.0.0"}}
{"type":"user.message","id":"e2","timestamp":"2026-11-05T09:00:01.000Z","data":{"content":"add a health endpoint","transformedContent":"<expanded>add a health endpoint</expanded>"}}
{"type":"assistant.turn_start","id":"e3","timestamp":"2026-11-05T09:00:02.000Z","data":{}}
{"type":"assistant.message","id":"e4","timestamp":"2026-11-05T09:00:03.000Z","data":{"content":"I'll look at the router.","model":"claude-sonnet-4.5","toolRequests":[{"name":"report_intent","arguments":{"intent":"Exploring"},"toolCallId":"c0","type":"function"},{"name":"view","arguments":{"path":"/home/test/proj/src/router.ts"},"toolCallId":"c1","type":"function","intentionSummary":"view router"},{"name":"bash","arguments":{"command":"npm test"},"toolCallId":"c2","type":"function"}]}}
{"type":"tool.execution_start","id":"e5","timestamp":"2026-11-05T09:00:04.000Z","data":{"toolCallId":"c1"}}
{"type":"tool.execution_complete","id":"e6","timestamp":"2026-11-05T09:00:05.000Z","data":{"toolCallId":"c1","success":true}}
{"type":"tool.execution_complete","id":"e7","timestamp":"2026-11-05T09:00:06.000Z","data":{"toolCallId":"c2","success":false}}
{"type":"permission.requested","id":"e8","timestamp":"2026-11-05T09:00:06.500Z","data":{}}
{"type":"hook.start","id":"e9","timestamp":"2026-11-05T09:00:06.600Z","data":{}}
{"type":"assistant.message","id":"e10","timestamp":"2026-11-05T09:00:07.000Z","data":{"content":"","toolRequests":[{"name":"edit","arguments":{"path":"/home/test/proj/src/router.ts","old_str":"a","new_str":"b"},"toolCallId":"c3"},{"name":"create","arguments":{"path":"/home/test/proj/src/health.ts","file_text":"x"},"toolCallId":"c4"},{"name":"rg","arguments":{"pattern":"health"},"toolCallId":"c5"}]}}
{"type":"session.usage_checkpoint","id":"e11","timestamp":"2026-11-05T09:00:08.000Z","data":{"totalPremiumRequests":2}}
{"type":"abort","id":"e12","timestamp":"2026-11-05T09:00:09.000Z","data":{}}
{"type":"subagent.started","id":"e13","timestamp":"2026-11-05T09:00:09.100Z","data":{}}
{"type":"skill.invoked","id":"e14","timestamp":"2026-11-05T09:00:09.200Z","data":{}}
{"type":"system.message","id":"e15","timestamp":"2026-11-05T09:00:09.300Z","data":{"content":"sys"}}
{"type":"session.resume","id":"e16","timestamp":"2026-11-05T10:00:00.000Z","data":{"context":{"cwd":"/home/test/proj"},"sessionLimits":null}}
{"type":"user.message","id":"e17","timestamp":"2026-11-05T10:00:01.000Z","data":{"content":"/model"}}
{"type":"assistant.message","id":"e18","timestamp":"2026-11-05T10:00:02.000Z","data":{"content":"Done. ğüş 🎉"}}
{"type":"session.shutdown","id":"e19","timestamp":"2026-11-05T10:00:03.000Z","data":{"totalPremiumRequests":3.5,"codeChanges":{"filesModified":2,"linesAdded":10,"linesRemoved":1}}}
{"type":"brand.new","id":"e20","timestamp":"2026-11-05T10:00:04.000Z","data":{}}
garbage
`

func writeSession(t *testing.T, h *testutil.Homes, id string, withYAML bool) string {
	t.Helper()
	dir := filepath.Join(h.Copilot, "session-state", id)
	testutil.WriteFile(t, filepath.Join(dir, "events.jsonl"), []byte(events))
	if withYAML {
		testutil.WriteFile(t, filepath.Join(dir, "workspace.yaml"), []byte(
			"id: "+id+"\ncwd: \"/home/test/proj\"\ngit_root: /home/test/proj\nbranch: main # current\n"+
				"created_at: 2026-11-05T09:00:00.000Z\nsummary: |\n  multi\n  line\nname: 'it''s mine'\nnested:\n  key: v\n"))
	}
	return dir
}

func TestReadWorkspace(t *testing.T) {
	h := testutil.Isolate(t)
	dir := writeSession(t, h, "s1", true)
	ws := ReadWorkspace(filepath.Join(dir, "workspace.yaml"))
	if ws["cwd"] != "/home/test/proj" || ws["branch"] != "main" || ws["name"] != "it's mine" || ws["key"] != "" || ws["summary"] != "" {
		t.Fatalf("%v", ws)
	}
}

func TestLoad(t *testing.T) {
	h := testutil.Isolate(t)
	SQLiteLookPath = func() (string, error) { return "", errors.New("none") }
	t.Cleanup(func() { SQLiteLookPath = func() (string, error) { return exec.LookPath("sqlite3") } })
	dir := writeSession(t, h, "s1", true)
	testutil.WriteFile(t, filepath.Join(dir, "session.db"), []byte("not really sqlite"))
	s, err := New(session.Env{}).Load(context.Background(), session.Ref{Path: dir}, session.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "s1" || s.CWD != "/home/test/proj" || s.Model != "claude-sonnet-4.5" || s.GitBranch != "main" {
		t.Fatalf("meta %+v model=%s branch=%s", s.Ref, s.Model, s.GitBranch)
	}
	var texts []string
	for _, e := range s.Entries {
		texts = append(texts, e.Text)
	}
	want := "add a health endpoint|I'll look at the router.||/model|Done. ğüş 🎉"
	if strings.Join(texts, "|") != want {
		t.Fatalf("got %q", strings.Join(texts, "|"))
	}
	tools := s.Entries[1].Tools
	if len(tools) != 2 || tools[0].Name != "view" || tools[0].Failed || !tools[1].Failed {
		t.Fatalf("tools %+v", tools)
	}
	if s.Entries[3].Kind != session.KindSlashCommand {
		t.Fatal("slash command")
	}
	if s.Usage.PremiumRequests != 3.5 || s.Usage.Replies != 3 {
		t.Fatalf("usage %+v", s.Usage)
	}
	w := strings.Join(s.Warnings, ";")
	if !strings.Contains(w, "sqlite3 not found") || !strings.Contains(w, "1 unknown event") || !strings.Contains(w, "1 malformed") {
		t.Fatalf("warnings %v", s.Warnings)
	}
}

func TestTodosViaSQLite(t *testing.T) {
	bin, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 not installed")
	}
	h := testutil.Isolate(t)
	dir := writeSession(t, h, "s2", true)
	db := filepath.Join(dir, "session.db")
	sql := "create table todos(id text, title text, description text, status text);" +
		"insert into todos values('1','write handler','',  'done');" +
		"insert into todos values('2','add test','', 'in_progress');" +
		"insert into todos values('3','docs','', 'blocked');"
	if out, err := exec.Command(bin, db, sql).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	s, err := New(session.Env{}).Load(context.Background(), session.Ref{Path: dir}, session.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Todos) != 3 || s.Todos[0].Status != session.TodoDone || s.Todos[1].Status != session.TodoInProgress || s.Todos[2].Status != session.TodoBlocked {
		t.Fatalf("todos %+v warnings %v", s.Todos, s.Warnings)
	}
}

func TestFind(t *testing.T) {
	h := testutil.Isolate(t)
	writeSession(t, h, "with-yaml", true)
	writeSession(t, h, "no-yaml", false) // cwd from session.start
	if err := os.MkdirAll(filepath.Join(h.Copilot, "session-state", "no-events"), 0o700); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, filepath.Join(h.Copilot, "session-state", "no-events", "workspace.yaml"), []byte("cwd: /home/test/proj\n"))
	p := New(session.Env{})
	refs, err := p.Find(context.Background(), session.Query{CWD: "/home/test/proj"})
	if err != nil || len(refs) != 2 {
		t.Fatalf("%v %+v", err, refs)
	}
	for _, r := range refs {
		if r.ID == "no-events" || r.CWD != "/home/test/proj" {
			t.Fatalf("bad ref %+v", r)
		}
	}
	refs, _ = p.Find(context.Background(), session.Query{ID: "no-yaml"})
	if len(refs) != 1 {
		t.Fatalf("by id %+v", refs)
	}
	refs, _ = p.Find(context.Background(), session.Query{CWD: "/home/test/other"})
	if len(refs) != 0 {
		t.Fatalf("other cwd %+v", refs)
	}
}

func TestPreview(t *testing.T) {
	h := testutil.Isolate(t)
	dir := writeSession(t, h, "s3", false)
	got, err := New(session.Env{}).(*Provider).Preview(context.Background(), session.Ref{Path: dir})
	if err != nil || got != "add a health endpoint" {
		t.Fatalf("%q %v", got, err)
	}
}

func FuzzCopilotLoad(f *testing.F) {
	f.Add([]byte(events))
	f.Add([]byte(`{"type":"assistant.message","data":{"toolRequests":[{"toolCallId":"x"}]}}` + "\n" + `{"type":"tool.execution_complete","data":{"toolCallId":"x","success":false}}`))
	dir := f.TempDir()
	SQLiteLookPath = func() (string, error) { return "", errors.New("none") }
	f.Fuzz(func(t *testing.T, data []byte) {
		if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), data, 0o600); err != nil {
			t.Skip()
		}
		_, _ = New(session.Env{Home: dir}).Load(context.Background(), session.Ref{Path: dir}, session.LoadOptions{HeadBytes: 100, TailBytes: 100})
		_, _ = New(session.Env{Home: dir}).(*Provider).Preview(context.Background(), session.Ref{Path: dir})
	})
}
