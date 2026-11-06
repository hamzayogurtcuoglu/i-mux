// Package panel gathers the data of the limits panel (`imux limits`, §11)
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

// Month is usage over a calendar month.
type Month struct {
	Start           time.Time // local first day of the month
	PremiumRequests float64
	Sessions        int
}

// StartOfMonth returns local midnight of the first day of t's month.
func StartOfMonth(t time.Time) time.Time {
	l := t.Local()
	return time.Date(l.Year(), l.Month(), 1, 0, 0, 0, 0, time.Local)
}

// CopilotMonth sums premium requests this calendar month: per session the
// latest session.shutdown / session.usage_checkpoint total, for sessions
// whose last event falls in this month.
func CopilotMonth(ctx context.Context, f *sources.Finder, now time.Time) Month {
	m := Month{Start: StartOfMonth(now)}
	refs := f.Find(ctx, []session.AgentID{session.Copilot}, session.Query{Since: m.Start, Limit: maxSessions})
	for _, r := range refs {
		if r.UpdatedAt.Before(m.Start) {
			continue
		}
		s, err := f.Load(ctx, r, time.Time{})
		if err != nil {
			continue
		}
		m.Sessions++
		m.PremiumRequests += s.Usage.PremiumRequests
	}
	return m
}

// CodexLimits returns the newest rate-limit snapshot Codex wrote (nil when
// no session has one: older Codex versions do not write it).
func CodexLimits(ctx context.Context, f *sources.Finder) *session.RateLimitSnapshot {
	refs := f.Find(ctx, []session.AgentID{session.Codex}, session.Query{Limit: 20})
	for _, r := range refs {
		s, err := f.Load(ctx, r, time.Time{})
		if err != nil {
			continue
		}
		if rl := s.Usage.RateLimits; rl != nil {
			return rl
		}
	}
	return nil
}
