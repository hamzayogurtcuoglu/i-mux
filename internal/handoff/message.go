package handoff

import (
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// Message is the short first message that points the new agent to a saved
// handoff file (§8.2b), so the user does not see the whole document:
//
//	[i-mux handoff · Claude Code → Codex CLI · 2026-11-07 09:47]
//	Claude Code hit its usage limit, so we continue here. First read …
//	/Users/me/.imux/handoffs/20261107-094700-claude-to-codex.md
//
// The marker keeps it recognizable as a handoff (chained handoffs follow the
// path on the last line, session.ParseChained); the sentence is in the UI
// language, because the user reads it and the agent answers in it.
type Message struct {
	From, To session.AgentID
	At       time.Time
	Path     string
	Limit    bool // the source agent hit its usage limit
	Delta    bool // the target resumed natively: what happened elsewhere
}

// Text renders the message.
func (m Message) Text() string {
	var line string
	switch {
	case m.Delta:
		line = i18n.T("handoff.msg.delta", session.DisplayName(m.From))
	case m.Limit:
		line = i18n.T("handoff.msg.limit", session.DisplayName(m.From))
	default:
		line = i18n.T("handoff.msg.manual", session.DisplayName(m.From))
	}
	return session.Marker(m.From, m.To, m.At) + "\n" + line + "\n" + m.Path
}
