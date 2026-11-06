// Package panel gathers the data of the limits panel (`ajan limits`, §11)
// from local files only: limits detected on screen (state.json) and usage
// read from the agents' session stores.
package panel

import (
	"context"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/limits"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
)

// maxSessions bounds how many sessions one panel row loads.
const maxSessions = 500

// Day is usage over a calendar day.
type Day struct {
	Date     time.Time // local midnight
	Requests int       // model replies
	limits.Tokens
	Chats int
}

// StartOfDay returns local midnight of t's day.
func StartOfDay(t time.Time) time.Time {
	l := t.Local()
	return time.Date(l.Year(), l.Month(), l.Day(), 0, 0, 0, 0, time.Local)
}

// GeminiToday sums requests (Gemini messages) and tokens of every chat, in
// any project, modified today in the local time zone.
func GeminiToday(ctx context.Context, f *sources.Finder, now time.Time) Day {
	d := Day{Date: StartOfDay(now)}
	refs := f.Find(ctx, []session.AgentID{session.Gemini}, session.Query{Since: d.Date, Limit: maxSessions})
	for _, r := range refs {
		if r.UpdatedAt.Before(d.Date) {
			continue
		}
		s, err := f.Load(ctx, r, time.Time{})
		if err != nil {
			continue
		}
		d.Chats++
		d.Requests += s.Usage.Replies
		d.Input += s.Usage.InputTokens
		d.Output += s.Usage.OutputTokens
		d.CacheRead += s.Usage.CacheReadTokens
		d.CacheWrite += s.Usage.CacheWriteTokens
	}
	return d
}
