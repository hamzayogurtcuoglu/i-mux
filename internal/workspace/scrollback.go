package workspace

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session/antigravity"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// Scrollback fallback (§18): when no session file is found, the pane's own
// scrollback becomes the transcript, marked as reconstructed.
const (
	scrollbackLines = 3000
	scrollbackChunk = 1100 // runes per entry (the builder caps entries at 1.2K)
)

// ReconstructedNote marks a handoff built from terminal scrollback.
const ReconstructedNote = "The record above was reconstructed from terminal scrollback because no session file was found; it may be incomplete."

// ReconstructedWarning is the warning shown to the user.
const ReconstructedWarning = "no session file found; context reconstructed from terminal scrollback"

func init() {
	scrollbackFallback = func(w *WS, ctx context.Context, p *Pane) (*session.Session, error) {
		out, err := w.T.Run(ctx, tmux.Cmd{"capture-pane", "-p", "-J", "-S", "-" + itoa(scrollbackLines), "-t", p.ID})
		if err != nil {
			return nil, err
		}
		id := p.Agent
		l := p.Trail.Last()
		if id == "" && l != nil {
			id = l.Agent
		}
		s := ScrollbackSession(id, p.CWD, out, w.Now())
		if id == session.Antigravity && l != nil && l.Agent == id {
			// Antigravity keeps its plan as a markdown checklist; the
			// conversation itself is only in the scrollback.
			s.Todos = antigravity.Todos(session.Env{}.UserHome(), l.StartTime().Add(-discoverSlack))
		}
		return s, nil
	}
}

// ScrollbackSession turns captured screen text into a session: chunks of
// lines as assistant entries, then a notice that it was reconstructed. Empty
// text gives a session without entries ("nothing recorded").
func ScrollbackSession(id session.AgentID, cwd, text string, at time.Time) *session.Session {
	s := &session.Session{Ref: session.Ref{Agent: id, CWD: cwd}, Warnings: []string{ReconstructedWarning}}
	lines := strings.Split(strings.ReplaceAll(text, "\r", ""), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	// Drop leading and trailing blank lines.
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return s
	}
	var b strings.Builder
	n := 0
	flush := func() {
		if t := strings.TrimSpace(b.String()); t != "" {
			s.Entries = append(s.Entries, session.Entry{Role: session.RoleAssistant, Kind: session.KindMessage, At: at, Text: t})
		}
		b.Reset()
		n = 0
	}
	for _, l := range lines {
		k := utf8.RuneCountInString(l) + 1
		if n > 0 && n+k > scrollbackChunk {
			flush()
		}
		b.WriteString(l)
		b.WriteByte('\n')
		n += k
	}
	flush()
	s.Entries = append(s.Entries, session.Entry{Role: session.RoleNotice, Kind: session.KindMessage, At: at, Text: ReconstructedNote})
	return s
}
