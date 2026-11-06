package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/clean"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/limits"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/textfmt"
)

// Report is the limits panel. Its JSON form (`imux limits --json`) is
// documented in docs/LIMITS.md; field names are stable.
type Report struct {
	GeneratedAt time.Time `json:"generated_at"`
	Agents      []Row     `json:"agents"`
}

// Row is one agent.
type Row struct {
	Agent     session.AgentID `json:"agent"`
	Name      string          `json:"name"`
	Installed bool            `json:"installed"`
	// Status is "limited", "warn" or "ok" (from limits detected on screen).
	Status string       `json:"status"`
	Limit  *state.Limit `json:"limit,omitempty"`
	Usage  *Usage       `json:"usage,omitempty"`
}

// Usage is the agent's local usage. Window says what it covers: "5h"
// (Claude), "today" (Antigravity), "month" (Copilot) or "latest" (Codex).
type Usage struct {
	Window          string                     `json:"window"`
	From            time.Time                  `json:"from,omitzero"`
	Replies         int                        `json:"replies,omitempty"`
	InputTokens     int64                      `json:"input_tokens,omitempty"`
	OutputTokens    int64                      `json:"output_tokens,omitempty"`
	CacheReadTokens int64                      `json:"cache_read_tokens,omitempty"`
	CacheWrite      int64                      `json:"cache_write_tokens,omitempty"`
	PremiumRequests float64                    `json:"premium_requests,omitempty"`
	Sessions        int                        `json:"sessions,omitempty"`
	RateLimits      *session.RateLimitSnapshot `json:"rate_limits,omitempty"`
}

// Collect builds the report from state.json and the session stores.
func Collect(ctx context.Context, cfg *config.Config, st *state.State, installed func(session.AgentID) bool, now time.Time) *Report {
	f := sources.New(cfg)
	r := &Report{GeneratedAt: now}
	for _, a := range agent.Ordered(cfg) {
		row := Row{Agent: a.ID, Name: a.Name, Installed: installed(a.ID), Status: "ok"}
		if l := st.ActiveRecord(string(a.ID), now); l != nil {
			row.Limit = l
			row.Status = "limited"
			if !l.Blocking() {
				row.Status = "warn"
			}
		}
		switch a.ID {
		case session.Claude:
			w := limits.ClaudeUsage(f.Env, now, 5*time.Hour)
			row.Usage = &Usage{
				Window: "5h", From: w.From, Replies: w.Replies, InputTokens: w.Input, OutputTokens: w.Output,
				CacheReadTokens: w.CacheRead, CacheWrite: w.CacheWrite, Sessions: w.Files,
			}
		case session.Antigravity:
			d := AntigravityToday(ctx, f, now)
			row.Usage = &Usage{
				Window: "today", From: d.Date, Replies: d.Requests, InputTokens: d.Input, OutputTokens: d.Output,
				CacheReadTokens: d.CacheRead, Sessions: d.Chats,
			}
		case session.Copilot:
			m := CopilotMonth(ctx, f, now)
			row.Usage = &Usage{Window: "month", From: m.Start, PremiumRequests: m.PremiumRequests, Sessions: m.Sessions}
		case session.Codex:
			row.Usage = &Usage{Window: "latest", RateLimits: CodexLimits(ctx, f)}
		}
		r.Agents = append(r.Agents, row)
	}
	return r
}

// JSON writes the report as indented JSON.
func (r *Report) JSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// Table writes the human-readable panel.
func (r *Report) Table(w io.Writer) error {
	now := r.GeneratedAt
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, i18n.T("limits.header"))
	for _, row := range r.Agents {
		status := statusText(row, now)
		fmt.Fprintf(tw, "%s\t%s\t%s\n", agent.MustGet(row.Agent).Short, status, usageText(row, now))
		if row.Limit != nil && row.Limit.Text != "" {
			fmt.Fprintf(tw, "\t\t%s\n", i18n.T("limits.seen", clean.OneLine(row.Limit.Text, 70), textfmt.Ago(now, row.Limit.At)))
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, i18n.T("limits.footer"))
	return nil
}

func statusText(row Row, now time.Time) string {
	switch {
	case !row.Installed:
		return i18n.T("limits.not_installed")
	case row.Status == "limited":
		t := row.Limit.ResetAt
		if t.IsZero() {
			t = row.Limit.ExpiresAt
		}
		return "⛔ " + i18n.T("limits.limited_until", textfmt.Clock(now, t))
	case row.Status == "warn":
		return "◐ " + i18n.T("limits.approaching")
	}
	return "● " + i18n.T("limits.ok")
}

func usageText(row Row, now time.Time) string {
	u := row.Usage
	if u == nil {
		return ""
	}
	tokens := func() string {
		s := i18n.T("limits.tokens", short(u.InputTokens), short(u.OutputTokens))
		if u.CacheReadTokens > 0 {
			s += " · " + i18n.T("limits.cache", short(u.CacheReadTokens))
		}
		return s
	}
	switch u.Window {
	case "5h":
		return i18n.T("limits.claude", u.Replies) + " · " + tokens()
	case "today":
		return i18n.T("limits.today", u.Replies) + " · " + tokens()
	case "month":
		return i18n.T("limits.copilot", strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.2f", u.PremiumRequests), "0"), "."))
	case "latest":
		rl := u.RateLimits
		if rl == nil {
			return i18n.T("limits.codex_na")
		}
		s := i18n.T("limits.codex_primary", rl.PrimaryUsedPercent, resetText(now, rl.PrimaryResetsAt))
		if rl.SecondaryUsedPercent > 0 || !rl.SecondaryResetsAt.IsZero() {
			s += " · " + i18n.T("limits.codex_secondary", rl.SecondaryUsedPercent, resetText(now, rl.SecondaryResetsAt))
		}
		return s
	}
	return ""
}

func resetText(now, t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	return textfmt.Clock(now, t)
}

// short renders a token count: 812, 34.5K, 1.2M.
func short(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprint(n)
	case n < 1_000_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1000), ".0") + "K"
	}
	return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1e6), ".0") + "M"
}
