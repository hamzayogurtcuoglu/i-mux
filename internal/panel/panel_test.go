package panel

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/gemini"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func geminiChat(t *testing.T, h *testutil.Homes, project, id string, mtime time.Time, msgs int) {
	t.Helper()
	ts := mtime.UTC().Format(time.RFC3339)
	body := fmt.Sprintf(`{"sessionId":%q,"startTime":%q,"lastUpdated":%q,"messages":[`, id, ts, ts)
	for i := 0; i < msgs; i++ {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"id":"u%d","timestamp":%q,"type":"user","content":"q"},{"id":"g%d","timestamp":%q,"type":"gemini","content":"a","tokens":{"input":100,"output":10,"cached":5}}`, i, ts, i, ts)
	}
	body += "]}"
	p := filepath.Join(h.Gemini, "tmp", gemini.ProjectHash(project), "chats", "session-"+mtime.UTC().Format("2006-01-02T15-04")+"-"+id[:8]+".json")
	testutil.WriteFile(t, p, []byte(body))
	testutil.Touch(t, p, mtime)
}

func TestGeminiToday(t *testing.T) {
	h := testutil.Isolate(t)
	loc := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = loc })
	now := testutil.FrozenTime // 14:32 UTC
	geminiChat(t, h, h.Project, "aaaaaaaa-1", now.Add(-time.Hour), 2)
	geminiChat(t, h, "/home/test/other", "bbbbbbbb-2", now.Add(-10*time.Hour), 3) // today, other project
	geminiChat(t, h, h.Project, "cccccccc-3", now.Add(-20*time.Hour), 7)          // yesterday
	d := GeminiToday(context.Background(), sources.New(config.Default()), now)
	if d.Chats != 2 || d.Requests != 5 || d.Input != 500 || d.Output != 50 || d.CacheRead != 25 {
		t.Fatalf("today %+v", d)
	}
	if !d.Date.Equal(time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date %v", d.Date)
	}
}

func copilotSession(t *testing.T, h *testutil.Homes, id string, mtime time.Time, totals ...float64) {
	t.Helper()
	dir := filepath.Join(h.Copilot, "session-state", id)
	testutil.WriteFile(t, filepath.Join(dir, "workspace.yaml"), []byte("id: "+id+"\ncwd: "+h.Project+"\n"))
	body := `{"type":"session.start","id":"e0","timestamp":"2026-11-05T08:00:00.000Z","data":{"context":{"cwd":"` + h.Project + `"}}}` + "\n"
	for i, v := range totals {
		typ := "session.usage_checkpoint"
		if i == len(totals)-1 {
			typ = "session.shutdown"
		}
		body += fmt.Sprintf(`{"type":%q,"id":"e%d","timestamp":"2026-11-05T09:00:0%d.000Z","data":{"totalPremiumRequests":%g}}`+"\n", typ, i+1, i, v)
	}
	p := filepath.Join(dir, "events.jsonl")
	testutil.WriteFile(t, p, []byte(body))
	testutil.Touch(t, p, mtime)
}

func TestCopilotMonth(t *testing.T) {
	h := testutil.Isolate(t)
	loc := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = loc })
	now := testutil.FrozenTime                                                                // Nov 5
	copilotSession(t, h, "11111111-aaaa-4000-8000-000000000001", now.Add(-time.Hour), 2, 3.5) // latest total 3.5
	copilotSession(t, h, "11111111-aaaa-4000-8000-000000000002", now.Add(-72*time.Hour), 1)   // Nov 2
	copilotSession(t, h, "11111111-aaaa-4000-8000-000000000003", now.AddDate(0, -1, 0), 40)   // October
	m := CopilotMonth(context.Background(), sources.New(config.Default()), now)
	if m.Sessions != 2 || m.PremiumRequests != 4.5 || !m.Start.Equal(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("month %+v", m)
	}
}
