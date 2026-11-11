package session

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMarkerRoundTrip(t *testing.T) {
	at := time.Date(2026, 11, 5, 14, 32, 0, 0, time.Local)
	m := Marker(Claude, Antigravity, at)
	if m != "[i-mux handoff · Claude Code → Antigravity CLI · 2026-11-05 14:32]" {
		t.Fatalf("marker %q", m)
	}
	doc := m + "\n\nYou are continuing...\n\n## Original request\nfix the bug\nplease\n\n" +
		"## Earlier history (carried over from a previous handoff)\nold stuff\n\n" +
		"## Current task list\n- [ ] x\n\n" +
		"## Conversation (condensed, oldest first)\n**User:** hi\n**Assistant:** hello\n\n" +
		"## Where it stopped\nLast user request: hi\n"
	if !IsHandoff("\n  " + doc) {
		t.Fatal("IsHandoff false")
	}
	c := ParseChained(doc)
	if c == nil || c.From != Claude || c.To != Antigravity || !c.At.Equal(at) {
		t.Fatalf("parsed %+v", c)
	}
	if c.OriginalRequest != "fix the bug\nplease" {
		t.Fatalf("original %q", c.OriginalRequest)
	}
	if !strings.Contains(c.History, "old stuff") || !strings.Contains(c.History, "**Assistant:** hello") ||
		strings.Contains(c.History, "task list") {
		t.Fatalf("history %q", c.History)
	}
	if ParseChained("hello") != nil {
		t.Fatal("non-handoff parsed")
	}
}

func TestParseChainedGarbage(t *testing.T) {
	for _, s := range []string{MarkerPrefix, MarkerPrefix + "]", MarkerPrefix + " a → · ]\n## \n##", MarkerPrefix + "\n## Original request"} {
		c := ParseChained(s)
		if c == nil {
			t.Fatalf("%q: nil", s)
		}
	}
}

func TestTodoStatus(t *testing.T) {
	cases := map[string]TodoStatus{"completed": TodoDone, "in_progress": TodoInProgress, "pending": TodoPending, "blocked": TodoBlocked, "done": TodoDone, "weird": TodoPending}
	for in, want := range cases {
		if got := ParseTodoStatus(in); got != want {
			t.Errorf("%s: %v want %v", in, got, want)
		}
	}
}

type fakeProvider struct{ id AgentID }

func (f fakeProvider) Agent() AgentID                                           { return f.id }
func (f fakeProvider) Find(context.Context, Query) ([]Ref, error)               { return nil, nil }
func (f fakeProvider) Load(context.Context, Ref, LoadOptions) (*Session, error) { return nil, nil }

func TestRegistryOrder(t *testing.T) {
	Register(Codex, func(Env) Provider { return fakeProvider{Codex} })
	Register(Claude, func(Env) Provider { return fakeProvider{Claude} })
	ps := Providers(Env{})
	var ids []AgentID
	for _, p := range ps {
		ids = append(ids, p.Agent())
	}
	if len(ids) < 2 || ids[0] != Claude || ids[len(ids)-1] != Codex {
		t.Fatalf("order %v", ids)
	}
	if ProviderFor(Codex, Env{}) == nil || ProviderFor("vim", Env{}) != nil {
		t.Fatal("ProviderFor")
	}
}

func TestEnvResolve(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CODEX_HOME", "/x/codex")
	t.Setenv("COPILOT_HOME", "")
	e := Env{}
	in := func(rel string) string { return filepath.Join("/home/test", rel) } // backslashes on Windows
	if e.Resolve(Claude) != in(".claude") || e.Resolve(Codex) != "/x/codex" ||
		e.Resolve(Antigravity) != in(".gemini/antigravity-cli") || e.Resolve(Copilot) != in(".copilot") {
		t.Fatal("resolve wrong")
	}
	if (Env{ClaudeHome: "/c"}).Resolve(Claude) != "/c" {
		t.Fatal("override")
	}
}

func TestFilterSince(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s := &Session{Entries: []Entry{{At: t0}, {At: t0.Add(time.Hour)}, {}}, Omitted: 4}
	s.FilterSince(t0.Add(time.Minute))
	if len(s.Entries) != 2 || s.Omitted != 0 {
		t.Fatalf("%+v", s)
	}
}

func TestLimitLike(t *testing.T) {
	if !(APIError{Code: "rate_limit"}).LimitLike() || (APIError{Code: "authentication_failed"}).LimitLike() {
		t.Fatal("LimitLike")
	}
}

// A user's request may contain its own markdown headings ("## 1. Product
// vision"). Read back as a chained handoff, the original request must stay
// whole: only the handoff's own section headings split the document.
func TestParseChainedKeepsHeadingsInsideSections(t *testing.T) {
	req := "Write the plan.\n\n## 1. Product vision\n\nA tmux workspace.\n\n## 2. Key decisions\n\n- Go + tmux"
	doc := Marker(Claude, Antigravity, time.Date(2026, 11, 5, 14, 32, 0, 0, time.Local)) + "\n\nYou are continuing...\n\n" +
		"## Original request\n" + req + "\n\n" +
		"## Files touched\nEdited: a.go\n\n" +
		"## Conversation (condensed, oldest first; 3 earlier entries omitted)\n**User:** see ## 3. Open questions below\n## 3. Open questions\n- none\n\n" +
		"## Where it stopped\nLast user request: x\n\n## What to do now\ncontinue\n"
	c := ParseChained(doc)
	if c == nil {
		t.Fatal("not recognized")
	}
	if c.OriginalRequest != req {
		t.Fatalf("original request cut at a heading of its own:\n%q", c.OriginalRequest)
	}
	if !strings.Contains(c.History, "## 3. Open questions\n- none") || strings.Contains(c.History, "Edited: a.go") {
		t.Fatalf("history %q", c.History)
	}
}
