package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// agyChat writes an Antigravity conversation of project with msgs replies of
// 100 in / 10 out / 5 cache-read tokens each, last written at mtime.
func agyChat(t *testing.T, h *testutil.Homes, project, id string, mtime time.Time, msgs int) {
	t.Helper()
	var turns []testutil.AgyTurn
	for i := 0; i < msgs; i++ {
		turns = append(turns, testutil.AgyTurn{At: mtime, User: fmt.Sprintf("q%d", i), Reply: "a", Input: 100, Output: 10, Cache: 5})
	}
	testutil.WriteAntigravity(t, h, id, project, mtime, turns...)
}

func TestAntigravityToday(t *testing.T) {
	h := testutil.Isolate(t)
	loc := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = loc })
	now := testutil.FrozenTime // 14:32 UTC
	agyChat(t, h, h.Project, "aaaaaaaa-1", now.Add(-time.Hour), 2)
	agyChat(t, h, "/home/test/other", "bbbbbbbb-2", now.Add(-10*time.Hour), 3) // today, other project
	agyChat(t, h, h.Project, "cccccccc-3", now.Add(-20*time.Hour), 7)          // yesterday
	d := AntigravityToday(context.Background(), sources.New(config.Default()), now)
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

func TestCodexLimits(t *testing.T) {
	h := testutil.Isolate(t)
	f := sources.New(config.Default())
	write := func(id, extra string, mtime time.Time) {
		p := filepath.Join(h.Codex, "sessions", "2026", "11", "05", "rollout-2026-11-05T08-00-00-"+id+".jsonl")
		testutil.WriteFile(t, p, []byte(`{"timestamp":"2026-11-05T08:00:00.000Z","type":"session_meta","payload":{"id":"`+id+`","cwd":"`+h.Project+`"}}`+"\n"+extra))
		testutil.Touch(t, p, mtime)
	}
	write("cccccccc-0000-4000-8000-000000000001", "", testutil.FrozenTime)
	if CodexLimits(context.Background(), f) != nil {
		t.Fatal("no rate_limits should give nil")
	}
	write("cccccccc-0000-4000-8000-000000000002", `{"timestamp":"2026-11-05T08:00:10.000Z","type":"event_msg","payload":{"type":"token_count","info":null,"rate_limits":{"primary":{"used_percent":42.5,"window_minutes":300,"resets_in_seconds":3600}}}}`+"\n", testutil.FrozenTime.Add(-time.Hour))
	rl := CodexLimits(context.Background(), f)
	if rl == nil || rl.PrimaryUsedPercent != 42.5 || rl.PrimaryResetsAt.IsZero() {
		t.Fatalf("rate limits %+v", rl)
	}
}

func TestReportTableAndJSON(t *testing.T) {
	h := testutil.Isolate(t)
	loc := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = loc })
	now := testutil.FrozenTime
	agyChat(t, h, h.Project, "aaaaaaaa-1", now.Add(-time.Hour), 2)
	reset := now.Add(28 * time.Minute)
	st := &state.State{Limits: map[string]*state.Limit{
		"claude": {At: now.Add(-12 * time.Minute), Text: "5-hour limit reached ∙ resets 3pm", ResetAt: reset, ExpiresAt: reset, Source: "screen", Pane: "%1", Severity: "limit"},
		"codex":  {At: now, ExpiresAt: now.Add(time.Hour), Severity: "warn", Text: "approaching"},
	}}
	installed := func(id session.AgentID) bool { return id != "copilot" }
	rep := Collect(context.Background(), config.Default(), st, installed, now)
	var b strings.Builder
	if err := rep.Table(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{
		"AGENT", "Claude", "⛔ limited until 15:00", `"5-hour limit reached ∙ resets 3pm" (seen 12 min ago)`,
		"Antigravity", "● ok", "today: 2 requests · 200 in / 20 out · 10 cache read",
		"Copilot", "not installed", "this month: 0 premium requests",
		"Codex", "◐ approaching a limit", "rate limits: n/a (update Codex)",
		"CodexBar (brew install --cask codexbar)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table lacks %q:\n%s", want, out)
		}
	}
	b.Reset()
	if err := rep.JSON(&b); err != nil {
		t.Fatal(err)
	}
	var back struct {
		Agents []struct {
			Agent  string `json:"agent"`
			Status string `json:"status"`
			Limit  *struct {
				Severity string `json:"severity"`
			} `json:"limit"`
			Usage struct {
				Window  string `json:"window"`
				Replies int    `json:"replies"`
			} `json:"usage"`
		} `json:"agents"`
	}
	if err := json.Unmarshal([]byte(b.String()), &back); err != nil {
		t.Fatal(err)
	}
	if len(back.Agents) != 4 || back.Agents[0].Agent != "claude" || back.Agents[0].Status != "limited" || back.Agents[0].Limit.Severity != "limit" ||
		back.Agents[1].Agent != "antigravity" || back.Agents[1].Usage.Window != "today" || back.Agents[1].Usage.Replies != 2 ||
		back.Agents[3].Status != "warn" {
		t.Fatalf("json %+v\n%s", back, b.String())
	}
	if short(812) != "812" || short(34500) != "34.5K" || short(2_000_000) != "2M" {
		t.Fatal("short")
	}
}
