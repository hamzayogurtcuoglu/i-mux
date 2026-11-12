package autoview

import (
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/auto"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
)

func plain(ls []string) string {
	var out []string
	for _, l := range ls {
		out = append(out, strings.TrimRight(sidebar.Plain(l), " "))
	}
	return strings.Join(out, "\n")
}

// TestLines (T66.6): the agent's words wrapped, a heading without its
// hashes, each tool on a line with its result's first line under it (a
// failed one in red) and a blank line between steps, errors; every line
// fits.
func TestLines(t *testing.T) {
	i18n.SetLocale("en")
	o := auto.ParseOutput(`{"type":"system","subtype":"init","session_id":"s","model":"claude-sonnet-5-5"}
{"type":"assistant","message":{"content":[{"type":"text","text":"I will compare the newest tag of the simulator with the one the pipeline last built."},{"type":"tool_use","name":"Bash","input":{"command":"git ls-remote --tags https://example.com/org/sim.git | sort -V | tail -n 1"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","content":"abc refs/tags/v2.3\nmore\nlines","is_error":false}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/p/last.txt"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","content":"no such file","is_error":true}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":"## v2.3 is out\nThe pipeline started."}]}}
{"type":"result","subtype":"success","is_error":false,"result":"## v2.3 is out\nThe pipeline started."}
`, "/p")
	ls := Lines(o, 40)
	got := plain(ls)
	want := strings.Join([]string{
		" I will compare the newest tag of the",
		" simulator with the one the pipeline",
		" last built.",
		"",
		" ● Bash git ls-remote --tags https://ex…",
		"   ⎿ abc refs/tags/v2.3  +2 lines",
		"",
		" ● Read last.txt",
		"   ⎿ no such file",
		"",
		" v2.3 is out",
		" The pipeline started.",
	}, "\n")
	if got != want {
		t.Errorf("lines:\n%s\nwant\n%s", got, want)
	}
	for _, l := range ls {
		if n := sidebar.DisplayWidth(sidebar.Plain(l)); n > 40 {
			t.Errorf("%d columns: %q", n, sidebar.Plain(l))
		}
	}
	if !strings.Contains(ls[8], red) || !strings.Contains(ls[10], bold) {
		t.Errorf("colors: %q %q", ls[8], ls[10])
	}
	// A text CLI's lines as they are; a warning among events dim; errors.
	if got := plain(Lines(auto.ParseOutput("Done.\nAll good.\n", ""), 40)); got != " Done.\n All good." {
		t.Errorf("text CLI %q", got)
	}
	e := Lines(auto.ParseOutput("warning: x\n"+`{"type":"session.error","data":{"message":"Not logged in"}}`+"\n", ""), 40)
	if plain(e) != " warning: x\n\n ✗ Not logged in" || !strings.Contains(e[0], faint) {
		t.Errorf("errors %q", e)
	}
}

// TestMarkdown (T66.6): **bold** and `code` set off without their markers
// (one not closed stays), list items with a bullet and their lines under
// their text, code blocks indented; a tool's tabs, escapes and control
// characters never reach the terminal.
func TestMarkdown(t *testing.T) {
	i18n.SetLocale("en")
	text := "Started **v2.3** with `gh workflow run`, a * star and a ` tick.\n\n" +
		"- the first item, which is long enough to wrap\n* second\n2. third\n\n```sh\ngh run list\n```\n> quoted\n---"
	o := &auto.Output{Items: []auto.Item{
		{Kind: auto.ItemText, Text: text},
		{Kind: auto.ItemTool, Name: "Bash", Text: "printf 'a\tb'"},
		{Kind: auto.ItemResult, Text: "abc\trefs/tags/v2.3\x1b[31m red\x1b[0m\r\x07"},
	}}
	ls := Lines(o, 36)
	want := strings.Join([]string{
		" Started v2.3 with gh workflow run,",
		" a * star and a ` tick.",
		"",
		" • the first item, which is long",
		"   enough to wrap",
		" • second",
		" 2. third",
		"",
		"   gh run list",
		" │ quoted",
		" " + strings.Repeat("─", 34),
		"",
		" ● Bash printf 'a   b'",
		"   ⎿ abc refs/tags/v2.3 red",
	}, "\n")
	if got := plain(ls); got != want {
		t.Errorf("lines:\n%s\nwant\n%s", got, want)
	}
	if !strings.Contains(ls[0], bold+"v2.3") || !strings.Contains(ls[0], code+"gh workflow run") || !strings.Contains(ls[8], code) {
		t.Errorf("styles: %q %q", ls[0], ls[8])
	}
	for _, l := range ls {
		if strings.ContainsAny(sidebar.Plain(l), "\t\r\x07") || strings.Contains(l, "\x1b[31m") {
			t.Errorf("unsafe line %q", l)
		}
		if n := sidebar.DisplayWidth(sidebar.Plain(l)); n > 36 {
			t.Errorf("%d columns: %q", n, sidebar.Plain(l))
		}
	}
}

// TestHeader (T66.6): running with its spinner and time; starting; how it
// ended, with how long it took; one that did not start.
func TestHeader(t *testing.T) {
	i18n.SetLocale("en")
	now := time.Date(2026, 11, 12, 12, 5, 0, 0, time.Local)
	r := Run{Name: "sim-tag", CLI: session.Claude, Model: "sonnet", Command: "claude -p --output-format stream-json <prompt>", Start: now.Add(-72 * time.Second)}
	h := Header(r, now, 80)
	if len(h) != 3 || plain(h[:2]) != " ⠋ sim-tag · Claude · sonnet   running · 1m\n $ claude -p --output-format stream-json <prompt>" {
		t.Errorf("running %q", plain(h))
	}
	r.Start, r.Starting, r.Command = time.Time{}, true, ""
	if got := plain(Header(r, now, 80)[:1]); got != " ⠋ sim-tag · Claude · sonnet   starting…" {
		t.Errorf("starting %q", got)
	}
	r.Starting = false
	for _, c := range []struct {
		res  auto.Run
		want string
	}{
		{auto.Run{Start: now.Add(-3 * time.Minute), End: now.Add(-time.Minute), Status: auto.StatusOK, Summary: "v2.3"}, " ✓ sim-tag · Claude · sonnet   12:02 · took 2m  done"},
		{auto.Run{Start: now.Add(-3 * time.Minute), End: now.Add(-3*time.Minute + 5*time.Second), Status: auto.StatusFailed, Exit: 1, Summary: "Not logged in"}, " ✗ sim-tag · Claude · sonnet   12:02 · took 5s  Not logged in"},
		{auto.Run{Start: now.Add(-time.Minute), End: now, Status: auto.StatusQuiet}, " · sim-tag · Claude · sonnet   12:04 · took 1m  nothing new"},
		{auto.Run{Start: now, Status: auto.StatusSkipped, Summary: "Claude is not installed"}, " ↷ sim-tag · Claude · sonnet   12:05  Claude is not installed"},
	} {
		res := c.res
		r.Result = &res
		if got := plain(Header(r, now, 80)[:1]); got != c.want {
			t.Errorf("ended:\n got %q\nwant %q", got, c.want)
		}
	}
	r.Result, r.GaveUp = nil, true
	if got := plain(Header(r, now, 80)[:1]); got != " ↷ sim-tag · Claude · sonnet   did not start" {
		t.Errorf("gave up %q", got)
	}
	if Hint(true) == Hint(false) || !strings.Contains(Hint(true), "s stop") {
		t.Error("hints")
	}
}
