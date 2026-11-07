package handoff

import (
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

func TestMessage(t *testing.T) {
	i18n.SetLocale("en")
	at := time.Date(2026, 11, 7, 9, 47, 0, 0, time.Local)
	path := "/Users/me/.imux/handoffs/20261107-094700-claude-to-codex.md"
	m := Message{From: session.Claude, To: session.Codex, At: at, Path: path, Limit: true}
	lines := strings.Split(m.Text(), "\n")
	if len(lines) != 3 || lines[0] != session.Marker(session.Claude, session.Codex, at) || lines[2] != path {
		t.Fatalf("message %q", lines)
	}
	if !strings.HasPrefix(lines[1], "Claude Code hit its usage limit") || !session.IsHandoff(m.Text()) {
		t.Fatalf("limit line %q", lines[1])
	}
	m.Limit = false
	if l := strings.Split(m.Text(), "\n")[1]; !strings.HasPrefix(l, "We continue the Claude Code session") {
		t.Fatalf("manual line %q", l)
	}
	m.Delta = true
	if l := strings.Split(m.Text(), "\n")[1]; !strings.HasPrefix(l, "Welcome back") {
		t.Fatalf("delta line %q", l)
	}
	i18n.SetLocale("tr")
	defer i18n.SetLocale("en")
	m.Delta, m.Limit = false, true
	if l := strings.Split(m.Text(), "\n")[1]; !strings.Contains(l, "limitine takıldı") {
		t.Fatalf("tr line %q", l)
	}
}
