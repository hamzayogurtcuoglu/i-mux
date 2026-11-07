// Package panel gathers the data of the limits panel (`imux limits`, §11)
// from local files only: limits detected on screen (state.json) and usage
// read from the agents' session stores.
package panel

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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

// AntigravityToday sums requests (model replies) and tokens of every
// Antigravity conversation, in any project, modified today in the local time
// zone.
func AntigravityToday(ctx context.Context, f *sources.Finder, now time.Time) Day {
	d := Day{Date: StartOfDay(now)}
	refs := f.Find(ctx, []session.AgentID{session.Antigravity}, session.Query{Since: d.Date, Limit: maxSessions})
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

// CopilotQuota is Copilot's monthly premium-request quota as its session
// files last recorded it (quotaSnapshots.premium_interactions of a
// model.model_call_success event).
type CopilotQuota struct {
	Entitlement float64   `json:"entitlement"` // requests per month; 0 when unlimited
	Used        float64   `json:"used"`
	UsedPercent float64   `json:"used_percent"`
	Unlimited   bool      `json:"unlimited,omitempty"`
	ResetsAt    time.Time `json:"resets_at,omitzero"`
	At          time.Time `json:"at"`
}

// CopilotQuotaNow reads the newest quota snapshot of Copilot's sessions
// (nil when none has one: Copilot writes it after a model reply).
func CopilotQuotaNow(ctx context.Context, f *sources.Finder) *CopilotQuota {
	for _, r := range f.Find(ctx, []session.AgentID{session.Copilot}, session.Query{Limit: 20}) {
		if q := copilotQuotaIn(r.Path); q != nil {
			return q
		}
	}
	return nil
}

// copilotQuotaIn returns the last quota snapshot of a session (its folder,
// or its events.jsonl).
func copilotQuotaIn(path string) *CopilotQuota {
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		path = filepath.Join(path, "events.jsonl")
	}
	fh, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer fh.Close()
	var last []byte
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 64*1024), 16<<20)
	for sc.Scan() {
		if bytes.Contains(sc.Bytes(), []byte(`"quotaSnapshots"`)) {
			last = append(last[:0], sc.Bytes()...)
		}
	}
	return ParseCopilotQuota(last)
}

// ParseCopilotQuota reads the premium_interactions snapshot of one event.
func ParseCopilotQuota(line []byte) *CopilotQuota {
	var ev struct {
		Timestamp time.Time `json:"timestamp"`
		Data      struct {
			QuotaSnapshots map[string]struct {
				Unlimited   bool     `json:"isUnlimitedEntitlement"`
				Entitlement float64  `json:"entitlementRequests"`
				Used        float64  `json:"usedRequests"`
				Remaining   *float64 `json:"remainingPercentage"`
				ResetDate   string   `json:"resetDate"`
			} `json:"quotaSnapshots"`
		} `json:"data"`
	}
	if len(line) == 0 || json.Unmarshal(line, &ev) != nil {
		return nil
	}
	p, ok := ev.Data.QuotaSnapshots["premium_interactions"]
	if !ok {
		return nil
	}
	q := &CopilotQuota{Unlimited: p.Unlimited, Used: p.Used, At: ev.Timestamp}
	if !p.Unlimited && p.Entitlement > 0 {
		q.Entitlement = p.Entitlement
		q.UsedPercent = p.Used / p.Entitlement * 100
	}
	if p.Remaining != nil && !p.Unlimited {
		q.UsedPercent = 100 - *p.Remaining
	}
	if t, err := time.Parse(time.RFC3339, p.ResetDate); err == nil {
		q.ResetsAt = t
	}
	return q
}
