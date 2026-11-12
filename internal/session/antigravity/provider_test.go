package antigravity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/toolsum"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// conv writes a synthetic conversation in Antigravity CLI's layout and
// returns its transcript path.
func conv(t *testing.T, root, id string, steps []map[string]any, task string) string {
	t.Helper()
	dir := filepath.Join(root, "brain", id)
	var b strings.Builder
	for i, s := range steps {
		s["step_index"] = i
		if _, ok := s["status"]; !ok {
			s["status"] = "DONE"
		}
		line, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	p := filepath.Join(dir, logsDir, "transcript_full.jsonl")
	testutil.WriteFile(t, p, []byte(b.String()))
	if task != "" {
		testutil.WriteFile(t, filepath.Join(dir, "task.md"), []byte(task))
	}
	return p
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func userInput(req string, at time.Time) map[string]any {
	return map[string]any{
		"source": "USER_EXPLICIT", "type": "USER_INPUT", "created_at": ts(at),
		"content": "<USER_REQUEST>\n" + req + "\n</USER_REQUEST>\n<ADDITIONAL_METADATA>\nThe current local time is: x.\n</ADDITIONAL_METADATA>\n" +
			"<USER_SETTINGS_CHANGE>\nThe user changed setting `Model Selection`.\n</USER_SETTINGS_CHANGE>",
	}
}

func history(t *testing.T, root string, entries ...histEntry) {
	t.Helper()
	var b strings.Builder
	for _, h := range entries {
		line, _ := json.Marshal(h)
		b.Write(line)
		b.WriteByte('\n')
	}
	testutil.WriteFile(t, filepath.Join(root, "history.jsonl"), []byte(b.String()))
}

func TestLoadTranscript(t *testing.T) {
	h := testutil.Isolate(t)
	root := filepath.Join(h.Home, ".gemini", "antigravity-cli")
	at := testutil.FrozenTime
	path := conv(t, root, "aaaaaaaa-0000-4000-8000-000000000001", []map[string]any{
		{"source": "SYSTEM", "type": "SYSTEM_MESSAGE", "created_at": ts(at), "content": "system prompt"},
		userInput("fix the login redirect", at),
		{
			"source": "MODEL", "type": "PLANNER_RESPONSE", "created_at": ts(at.Add(time.Second)),
			"content": "Let me look.", "thinking": "private reasoning", "input_tokens": 100, "cache_read_tokens": 40, "output_tokens": 10,
			"tool_calls": []map[string]any{
				{"name": "run_command", "args": map[string]any{"CommandLine": "npm test", "WaitMsBeforeAsync": 0, "toolSummary": "run tests"}},
				{"name": "read_file", "args": map[string]any{"Path": h.Project + "/src/auth.ts"}},
				{"name": "list_dir", "args": map[string]any{"DirectoryPath": h.Project + "/src"}},
			},
		},
		{"source": "MODEL", "type": "GENERIC", "created_at": ts(at.Add(2 * time.Second)), "content": "1 failing"},
		{"source": "MODEL", "type": "GENERIC", "status": "ERROR", "created_at": ts(at.Add(2 * time.Second)), "content": "", "error": "file not found"},
		{"source": "MODEL", "type": "LIST_DIRECTORY", "created_at": ts(at.Add(2 * time.Second)), "content": "auth.ts"},
		{"source": "MODEL", "type": "PLANNER_RESPONSE", "created_at": ts(at.Add(3 * time.Second)), "content": "The redirect drops the next= parameter. Fixing it now.", "output_tokens": 5},
		{"source": "MODEL", "type": "GENERIC", "status": "ERROR", "created_at": ts(at.Add(4 * time.Second)), "error": "RESOURCE_EXHAUSTED: quota exceeded"},
		{"source": "SYSTEM", "type": "CONVERSATION_HISTORY", "created_at": ts(at.Add(4 * time.Second))},
	}, "- [x] Find the bug\n- [/] Fix the redirect\n")
	// A broken line is skipped and reported, never fatal.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("{broken\n")
	f.Close()

	p := New(session.Env{}).(*Provider)
	refs, err := p.Find(context.Background(), session.Query{ID: "aaaaaaaa-0000-4000-8000-000000000001"})
	if err != nil || len(refs) != 1 {
		t.Fatalf("find by id: %v %+v", err, refs)
	}
	s, err := p.Load(context.Background(), refs[0], session.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Entries) != 3 {
		t.Fatalf("entries %+v", s.Entries)
	}
	u, a1, a2 := s.Entries[0], s.Entries[1], s.Entries[2]
	if u.Role != session.RoleUser || u.Text != "fix the login redirect" {
		t.Fatalf("user entry %+v (wrappers must be stripped)", u)
	}
	if a1.Role != session.RoleAssistant || a1.Text != "Let me look." || strings.Contains(a1.Text, "private reasoning") || len(a1.Tools) != 3 {
		t.Fatalf("assistant entry %+v", a1)
	}
	if a1.Tools[0].Failed || !a1.Tools[1].Failed || a1.Tools[2].Failed {
		t.Fatalf("tool results map in order: %+v", a1.Tools)
	}
	var lines []string
	for _, tc := range a1.Tools {
		lines = append(lines, toolsum.Summarize(tc, h.Project).Line)
	}
	if lines[0] != "$ npm test" || lines[1] != "read src/auth.ts (failed)" || !strings.HasPrefix(lines[2], "listed src") {
		t.Fatalf("tool lines %q", lines)
	}
	if !strings.Contains(a2.Text, "next= parameter") {
		t.Fatalf("second reply %+v", a2)
	}
	if s.Usage.Replies != 2 || s.Usage.InputTokens != 100 || s.Usage.CacheReadTokens != 40 || s.Usage.OutputTokens != 15 {
		t.Fatalf("usage %+v", s.Usage)
	}
	if len(s.Errors) != 1 || !s.Errors[0].LimitLike() {
		t.Fatalf("only the quota error is kept: %+v", s.Errors)
	}
	if len(s.Todos) != 2 || s.Todos[1].Status != session.TodoInProgress {
		t.Fatalf("todos %+v", s.Todos)
	}
	if !s.StartedAt.Equal(at) || s.ID != "aaaaaaaa-0000-4000-8000-000000000001" {
		t.Fatalf("started %v id %q", s.StartedAt, s.ID)
	}
	if len(s.Warnings) == 0 {
		t.Fatal("the malformed line should be reported")
	}
}

// The working directory comes from history.jsonl: same text close in time,
// else the closest entry within a minute.
func TestFindByWorkspace(t *testing.T) {
	h := testutil.Isolate(t)
	root := filepath.Join(h.Home, ".gemini", "antigravity-cli")
	at := testutil.FrozenTime
	// Each transcript was last written when its conversation happened.
	for i, c := range []struct{ id, req string }{
		{"11111111-0000-4000-8000-000000000001", "in the project"},
		{"22222222-0000-4000-8000-000000000002", "elsewhere"},
		{"33333333-0000-4000-8000-000000000003", "typed differently"},
		{"44444444-0000-4000-8000-000000000004", "no history"},
	} {
		when := at.Add(time.Duration(i) * time.Hour)
		testutil.Touch(t, conv(t, root, c.id, []map[string]any{userInput(c.req, when)}, ""), when)
	}
	history(t, root,
		histEntry{Display: "in the project", Timestamp: at.Add(-2 * time.Second).UnixMilli(), Workspace: h.Project},
		histEntry{Display: "elsewhere", Timestamp: at.Add(time.Hour).UnixMilli(), Workspace: "/home/test/other"},
		histEntry{Display: "a handoff shown shortened", Timestamp: at.Add(2*time.Hour + 3*time.Second).UnixMilli(), Workspace: h.Project},
	)
	p := New(session.Env{}).(*Provider)
	refs, _ := p.Find(context.Background(), session.Query{CWD: h.Project})
	var ids []string
	for _, r := range refs {
		ids = append(ids, r.ID[:8])
		if r.CWD != h.Project {
			t.Errorf("%s cwd %q", r.ID, r.CWD)
		}
	}
	if strings.Join(ids, ",") != "33333333,11111111" {
		t.Fatalf("refs for the project %v (newest first; unknown directory excluded)", ids)
	}
	// A pane's run bounds the time: a conversation without a known directory
	// may be its own.
	refs, _ = p.Find(context.Background(), session.Query{CWD: h.Project, Since: at.Add(150 * time.Minute)})
	if len(refs) != 1 || refs[0].ID[:8] != "44444444" || refs[0].CWD != "" {
		t.Fatalf("time-bounded find %+v", refs)
	}
}

// imux launches agy with the handoff as its first prompt: it is recognized
// as a chained handoff, so the original request survives the next switch.
func TestChainedHandoffAndPreview(t *testing.T) {
	h := testutil.Isolate(t)
	root := filepath.Join(h.Home, ".gemini", "antigravity-cli")
	doc := session.Marker(session.Claude, session.Antigravity, testutil.FrozenTime) +
		"\n\nYou are continuing...\n\n## Original request\nadd a rate limiter\n\n## Conversation (condensed, oldest first)\n**User:** add a rate limiter\n"
	path := conv(t, root, "55555555-0000-4000-8000-000000000005", []map[string]any{
		userInput(doc, testutil.FrozenTime),
		{"source": "MODEL", "type": "PLANNER_RESPONSE", "created_at": ts(testutil.FrozenTime), "content": "Continuing with the limiter."},
	}, "")
	p := New(session.Env{}).(*Provider)
	ref := session.Ref{Agent: session.Antigravity, Path: path}
	s, err := p.Load(context.Background(), ref, session.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Entries[0].Kind != session.KindHandoff || s.Chained == nil || s.Chained.OriginalRequest != "add a rate limiter" {
		t.Fatalf("chained %+v / %+v", s.Entries[0], s.Chained)
	}
	if got, _ := p.Preview(context.Background(), ref); got != "add a rate limiter" {
		t.Fatalf("preview %q", got)
	}
}
