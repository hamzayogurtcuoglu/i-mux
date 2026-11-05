package codex

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

const uid = "cccccccc-1111-4222-8333-444444444444"

const rollout = `{"timestamp":"2026-11-05T08:00:00.000Z","type":"session_meta","payload":{"id":"` + uid + `","timestamp":"2026-11-05T08:00:00.000Z","cwd":"/home/test/proj","originator":"codex_cli_rs","cli_version":"0.38.0","instructions":"long instructions"}}
{"timestamp":"2026-11-05T08:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<environment_context>\n  <cwd>/home/test/proj</cwd>\n</environment_context>"}]}}
{"timestamp":"2026-11-05T08:00:01.100Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"<user_instructions>be nice</user_instructions>"}]}}
{"timestamp":"2026-11-05T08:00:02.000Z","type":"turn_context","payload":{"cwd":"/home/test/proj","model":"gpt-5-codex","approval_policy":"on-request"}}
{"timestamp":"2026-11-05T08:00:02.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"add a rate limiter"}]}}
{"timestamp":"2026-11-05T08:00:02.001Z","type":"event_msg","payload":{"type":"user_message","message":"add a rate limiter","kind":"plain"}}
{"timestamp":"2026-11-05T08:00:03.000Z","type":"response_item","payload":{"type":"reasoning","summary":[{"type":"summary_text","text":"secret reasoning"}]}}
{"timestamp":"2026-11-05T08:00:04.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"I'll check the middleware."}]}}
{"timestamp":"2026-11-05T08:00:04.001Z","type":"event_msg","payload":{"type":"agent_message","message":"I'll check the middleware."}}
{"timestamp":"2026-11-05T08:00:05.000Z","type":"response_item","payload":{"type":"function_call","name":"shell","arguments":"{\"command\":[\"bash\",\"-lc\",\"rg limiter\"],\"workdir\":\"/home/test/proj\"}","call_id":"call_1"}}
{"timestamp":"2026-11-05T08:00:06.000Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_1","output":"{\"output\":\"\",\"metadata\":{\"exit_code\":1,\"duration_seconds\":0.1}}"}}
{"timestamp":"2026-11-05T08:00:07.000Z","type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","input":"*** Begin Patch\n*** Add File: src/limiter.ts\n+x\n*** End Patch","call_id":"call_2"}}
{"timestamp":"2026-11-05T08:00:07.500Z","type":"response_item","payload":{"type":"custom_tool_call_output","call_id":"call_2","output":"Success"}}
{"timestamp":"2026-11-05T08:00:08.000Z","type":"response_item","payload":{"type":"function_call","name":"update_plan","arguments":"{\"plan\":[{\"step\":\"write limiter\",\"status\":\"completed\"},{\"step\":\"wire it\",\"status\":\"in_progress\"}]}","call_id":"call_3"}}
{"timestamp":"2026-11-05T08:00:09.000Z","type":"response_item","payload":{"type":"local_shell_call","call_id":"call_4","status":"completed","action":{"type":"exec","command":["npm","test"]}}}
{"timestamp":"2026-11-05T08:00:10.000Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1000,"cached_input_tokens":800,"output_tokens":50,"total_tokens":1050},"model_context_window":272000},"rate_limits":{"primary":{"used_percent":42.5,"window_minutes":300,"resets_in_seconds":3600},"secondary":{"used_percent":10,"window_minutes":10080,"resets_at":1762416000}}}}
{"timestamp":"2026-11-05T08:00:11.000Z","type":"event_msg","payload":{"type":"error","message":"You've hit your usage limit. Try again in 2 hours."}}
{"timestamp":"2026-11-05T08:00:12.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Done ğüş 🎉"}]}}
{"timestamp":"2026-11-05T08:00:13.000Z","type":"mystery","payload":{}}
{oops
`

func write(t *testing.T, h *testutil.Homes, day, name, body string) string {
	t.Helper()
	p := filepath.Join(h.Codex, "sessions", day, name)
	testutil.WriteFile(t, p, []byte(body))
	return p
}

func TestLoad(t *testing.T) {
	h := testutil.Isolate(t)
	p := write(t, h, "2026/11/05", "rollout-2026-11-05T08-00-00-"+uid+".jsonl", rollout)
	s, err := New(session.Env{}).Load(context.Background(), session.Ref{Path: p}, session.LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != uid || s.CWD != "/home/test/proj" || s.Model != "gpt-5-codex" {
		t.Fatalf("meta %+v %s", s.Ref, s.Model)
	}
	var texts []string
	for _, e := range s.Entries {
		texts = append(texts, e.Text)
	}
	if strings.Join(texts, "|") != "add a rate limiter|I'll check the middleware.|Done ğüş 🎉" {
		t.Fatalf("entries %q (event_msg duplicates or context leaked)", texts)
	}
	tools := s.Entries[1].Tools
	if len(tools) != 4 {
		t.Fatalf("tools %+v", tools)
	}
	if argv, ok := tools[0].Args["command"].([]any); !ok || len(argv) != 3 || argv[2] != "rg limiter" {
		t.Fatalf("decoded args %+v", tools[0].Args)
	}
	if !tools[0].Failed || tools[1].Failed {
		t.Fatal("failure mapping")
	}
	if tools[1].Name != "apply_patch" || !strings.Contains(tools[1].Args["input"].(string), "Add File") {
		t.Fatalf("custom tool %+v", tools[1])
	}
	if tools[3].Name != "shell" {
		t.Fatalf("local shell %+v", tools[3])
	}
	if len(s.Todos) != 2 || s.Todos[0].Status != session.TodoDone {
		t.Fatalf("todos %+v", s.Todos)
	}
	rl := s.Usage.RateLimits
	if rl == nil || rl.PrimaryUsedPercent != 42.5 || !rl.PrimaryResetsAt.Equal(time.Date(2026, 11, 5, 9, 0, 10, 0, time.UTC)) ||
		rl.SecondaryResetsAt.Unix() != 1762416000 {
		t.Fatalf("rate limits %+v", rl)
	}
	if s.Usage.InputTokens != 1000 || s.Usage.Replies != 2 {
		t.Fatalf("usage %+v", s.Usage)
	}
	if len(s.Errors) != 1 || !s.Errors[0].LimitLike() {
		t.Fatalf("errors %+v", s.Errors)
	}
	w := strings.Join(s.Warnings, ";")
	if !strings.Contains(w, "1 malformed") || !strings.Contains(w, "1 unknown line") {
		t.Fatalf("warnings %v", s.Warnings)
	}
	for _, e := range s.Entries {
		if strings.Contains(e.Text, "secret reasoning") {
			t.Fatal("reasoning leaked")
		}
	}
}

func TestRateLimitsAbsent(t *testing.T) {
	h := testutil.Isolate(t)
	body := `{"timestamp":"2026-11-05T08:00:00.000Z","type":"session_meta","payload":{"id":"` + uid + `","cwd":"/home/test/proj"}}
{"timestamp":"2026-11-05T08:00:10.000Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":5}}}}
`
	p := write(t, h, "2026/11/05", "rollout-2026-11-05T08-00-00-"+uid+".jsonl", body)
	s, err := New(session.Env{}).Load(context.Background(), session.Ref{Path: p}, session.LoadOptions{})
	if err != nil || s.Usage.RateLimits != nil || s.Usage.InputTokens != 5 {
		t.Fatalf("%v %+v", err, s.Usage)
	}
}

func TestFind(t *testing.T) {
	h := testutil.Isolate(t)
	meta := func(id, cwd string) string {
		return `{"timestamp":"2026-11-01T00:00:00Z","type":"session_meta","payload":{"id":"` + id + `","cwd":"` + cwd + `"}}` + "\n"
	}
	ids := []string{
		"aaaaaaaa-0000-4000-8000-000000000001",
		"aaaaaaaa-0000-4000-8000-000000000002",
		"aaaaaaaa-0000-4000-8000-000000000003",
		"aaaaaaaa-0000-4000-8000-000000000004",
	}
	p1 := write(t, h, "2026/10/30", "rollout-2026-10-30T10-00-00-"+ids[0]+".jsonl", meta(ids[0], "/home/test/proj"))
	p2 := write(t, h, "2026/11/04", "rollout-2026-11-04T10-00-00-"+ids[1]+".jsonl", meta(ids[1], "/home/test/proj"))
	p3 := write(t, h, "2026/11/05", "rollout-2026-11-05T10-00-00-"+ids[2]+".jsonl", meta(ids[2], "/home/test/proj"))
	write(t, h, "2026/11/05", "rollout-2026-11-05T11-00-00-"+ids[3]+".jsonl", meta(ids[3], "/home/test/other"))
	testutil.Touch(t, p1, time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC))
	testutil.Touch(t, p2, time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC))
	testutil.Touch(t, p3, time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC))
	pr := New(session.Env{})
	refs, err := pr.Find(context.Background(), session.Query{CWD: "/home/test/proj"})
	if err != nil || len(refs) != 3 || refs[0].ID != ids[2] {
		t.Fatalf("%v %+v", err, refs)
	}
	refs, _ = pr.Find(context.Background(), session.Query{CWD: "/home/test/proj", Since: time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)})
	if len(refs) != 2 {
		t.Fatalf("since: %+v", refs)
	}
	refs, _ = pr.Find(context.Background(), session.Query{ID: ids[0]})
	if len(refs) != 1 || refs[0].Path != p1 {
		t.Fatalf("by id: %+v", refs)
	}
	refs, _ = pr.Find(context.Background(), session.Query{Limit: 2})
	if len(refs) != 2 {
		t.Fatalf("limit: %+v", refs)
	}
}

func TestPreview(t *testing.T) {
	h := testutil.Isolate(t)
	p := write(t, h, "2026/11/05", "rollout-2026-11-05T08-00-00-"+uid+".jsonl", rollout)
	got, err := New(session.Env{}).(*Provider).Preview(context.Background(), session.Ref{Path: p})
	if err != nil || got != "add a rate limiter" {
		t.Fatalf("%q %v", got, err)
	}
}

func FuzzCodexLoad(f *testing.F) {
	f.Add([]byte(rollout))
	f.Add([]byte(`{"type":"response_item","payload":{"type":"function_call","arguments":"{\"plan\":[1,{\"step\":2}]}","name":"update_plan"}}`))
	dir := f.TempDir()
	p := filepath.Join(dir, "rollout-x.jsonl")
	f.Fuzz(func(t *testing.T, data []byte) {
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Skip()
		}
		_, _ = New(session.Env{Home: dir}).Load(context.Background(), session.Ref{Path: p}, session.LoadOptions{HeadBytes: 64, TailBytes: 64})
		_, _ = New(session.Env{Home: dir}).Load(context.Background(), session.Ref{Path: p}, session.LoadOptions{})
	})
}
