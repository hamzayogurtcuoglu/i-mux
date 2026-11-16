package panel

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/opencode"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// Tests never run the real opencode (it would ask opencode.ai with the
// developer's own key, D80): only a fake given as OpenCodeBin runs.
func init() {
	opencode.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }
}

func goRow(g *opencode.GoUsage) Row {
	return Row{
		Agent: session.OpenCode, Name: "OpenCode", Installed: true, Status: "ok",
		Usage: &Usage{Window: "opencode", InputTokens: 1200, OutputTokens: 80, Sessions: 1, OpenCodeGo: g},
	}
}

// TestOpenCodeGoWindows (D80): an OpenCode Go plan's windows show as bars
// with their reset times, after the status and before the tokens.
func TestOpenCodeGoWindows(t *testing.T) {
	loc := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = loc })
	now := testutil.FrozenTime
	g := &opencode.GoUsage{
		FiveHour: &opencode.GoWindow{UsedPercent: 39, ResetsAt: now.Add(2 * time.Hour)},
		Weekly:   &opencode.GoWindow{UsedPercent: 15, ResetsAt: now.Add(72 * time.Hour)},
		Monthly:  &opencode.GoWindow{UsedPercent: 7, ResetsAt: now.Add(20 * 24 * time.Hour)},
	}
	row := goRow(g)
	goLimited(&row, g, now)
	if row.Status != "ok" || row.Limit != nil {
		t.Fatalf("not limited: %+v", row)
	}
	var b strings.Builder
	if err := (&Report{GeneratedAt: now, Agents: []Row{row}}).Table(&b); err != nil {
		t.Fatal(err)
	}
	out := stripANSI(b.String())
	t.Logf("\n%s", out)
	for _, want := range []string{
		"5-hour    ████████░░░░░░░░░░░░  39%  resets " + now.Add(2*time.Hour).Format("15:04"),
		"weekly    ███░░░░░░░░░░░░░░░░░  15%  resets ",
		"monthly   █░░░░░░░░░░░░░░░░░░░   7%  resets ",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("table lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "model provider's limits") {
		t.Errorf("with Go data the provider note should not show:\n%s", out)
	}
	// JSON has the windows under usage.opencode_go.
	var j bytes.Buffer
	if err := (&Report{GeneratedAt: now, Agents: []Row{row}}).JSON(&j); err != nil {
		t.Fatal(err)
	}
	var back struct {
		Agents []struct {
			Usage struct {
				OpenCodeGo map[string]struct {
					UsedPercent float64 `json:"used_percent"`
					ResetsAt    string  `json:"resets_at"`
				} `json:"opencode_go"`
			} `json:"usage"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(j.Bytes(), &back); err != nil || back.Agents[0].Usage.OpenCodeGo["weekly"].UsedPercent != 15 || back.Agents[0].Usage.OpenCodeGo["five_hour"].ResetsAt == "" {
		t.Fatalf("json %s %v", j.String(), err)
	}
}

// TestOpenCodeGoLimited (D80): a window OpenCode stops at marks OpenCode
// limited until the last such window resets; a limit seen on screen wins;
// a window whose reset has passed does not count.
func TestOpenCodeGoLimited(t *testing.T) {
	now := testutil.FrozenTime
	week, month := now.Add(48*time.Hour), now.Add(10*24*time.Hour)
	g := &opencode.GoUsage{
		FiveHour: &opencode.GoWindow{UsedPercent: 60},
		Weekly:   &opencode.GoWindow{UsedPercent: 100, Limited: true, ResetsAt: week},
		Monthly:  &opencode.GoWindow{UsedPercent: 98, Limited: true, ResetsAt: month},
	}
	row := goRow(g)
	goLimited(&row, g, now)
	if row.Status != "limited" || row.Limit == nil || !row.Limit.ResetAt.Equal(month) || row.Limit.Source != "opencode" {
		t.Fatalf("limited: %+v %+v", row, row.Limit)
	}
	var b strings.Builder
	if err := (&Report{GeneratedAt: now, Agents: []Row{row}}).Table(&b); err != nil {
		t.Fatal(err)
	}
	if out := stripANSI(b.String()); !strings.Contains(out, "monthly   ████████████████████ 100%") {
		t.Errorf("a limited window is drawn full:\n%s", out)
	}
	seen := &state.Limit{At: now, Text: "rate limit exceeded", ResetAt: week, ExpiresAt: week, Source: "screen", Severity: "limit"}
	row = goRow(g)
	row.Limit, row.Status = seen, "limited"
	goLimited(&row, g, now)
	if row.Limit != seen {
		t.Fatalf("the screen's limit should stay: %+v", row.Limit)
	}
	past := &opencode.GoUsage{Weekly: &opencode.GoWindow{UsedPercent: 100, Limited: true, ResetsAt: now.Add(-time.Minute)}}
	row = goRow(past)
	goLimited(&row, past, now)
	if row.Status != "ok" || row.Limit != nil {
		t.Fatalf("a reset window: %+v", row)
	}
}
