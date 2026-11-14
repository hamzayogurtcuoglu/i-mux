package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/theme"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/clean"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/limits"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sources"
	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/textfmt"
	"github.com/hamzayogurtcuoglu/i-mux/internal/usage"
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
	CopilotQuota    *CopilotQuota              `json:"copilot_quota,omitempty"`
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
				RateLimits: claudeLimits(now),
			}
		case session.Antigravity:
			d := AntigravityToday(ctx, f, now)
			row.Usage = &Usage{
				Window: "today", From: d.Date, Replies: d.Requests, InputTokens: d.Input, OutputTokens: d.Output,
				CacheReadTokens: d.CacheRead, Sessions: d.Chats,
			}
		case session.Copilot:
			m := CopilotMonth(ctx, f, now)
			row.Usage = &Usage{
				Window: "month", From: m.Start, PremiumRequests: m.PremiumRequests, Sessions: m.Sessions,
				CopilotQuota: CopilotQuotaNow(ctx, f),
			}
		case session.Codex:
			row.Usage = &Usage{Window: "latest", RateLimits: CodexLimits(ctx, f)}
		case session.OpenCode:
			row.Usage = &Usage{Window: "opencode", From: now.Add(-5 * time.Hour)}
			if p, ok := f.Provider(session.OpenCode).(interface {
				Tokens(context.Context, time.Time) (int64, int64, int, error)
			}); ok {
				if in, out, n, err := p.Tokens(ctx, now.Add(-5*time.Hour)); err == nil {
					row.Usage.InputTokens, row.Usage.OutputTokens, row.Usage.Sessions = in, out, n
				}
			}
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

// claudeLimits is what Claude's status line last reported (D25), with
// windows that have reset since dropped to 0%.
func claudeLimits(now time.Time) *session.RateLimitSnapshot {
	u := usage.Load(paths.ClaudeUsage()).Current(now)
	if u == nil {
		return nil
	}
	rl := &session.RateLimitSnapshot{At: u.At}
	if w := u.FiveHour; w != nil {
		rl.PrimaryUsedPercent, rl.PrimaryWindowMinutes, rl.PrimaryResetsAt = w.UsedPercent, 300, w.ResetsAt
	}
	if w := u.Weekly; w != nil {
		rl.SecondaryUsedPercent, rl.SecondaryWindowMins, rl.SecondaryResetsAt = w.UsedPercent, 10080, w.ResetsAt
	}
	return rl
}

// Table writes the human-readable panel: a block per agent with a bar per
// usage window (§9.3 "Sidebar v5", limits panel).
func (r *Report) Table(w io.Writer) error {
	now := r.GeneratedAt
	var b strings.Builder
	for i, row := range r.Agents {
		if i > 0 {
			b.WriteString("\n")
		}
		name := agent.MustGet(row.Agent).Short
		fmt.Fprintf(&b, " %s%-12s%s %s\n", bold, name, reset, statusText(row, now))
		if !row.Installed {
			continue
		}
		u := row.Usage
		rl := (*session.RateLimitSnapshot)(nil)
		if u != nil {
			rl = u.RateLimits
		}
		var cq *CopilotQuota
		if u != nil {
			cq = u.CopilotQuota
		}
		switch {
		case cq != nil && cq.Unlimited:
			fmt.Fprintf(&b, "   %s%s%s\n", dim, i18n.T("limits.copilot_unlimited"), reset)
		case cq != nil:
			b.WriteString(windowLine(i18n.T("limits.win_month"), cq.UsedPercent, cq.ResetsAt, now))
			line := i18n.T("limits.copilot_requests", trimFloat(cq.Used), trimFloat(cq.Entitlement))
			if !cq.At.IsZero() {
				line += " · " + i18n.T("limits.as_of", textfmt.Ago(now, cq.At))
			}
			fmt.Fprintf(&b, "   %s%s%s\n", dim, line, reset)
		case rl != nil:
			if rl.PrimaryWindowMinutes > 0 || rl.PrimaryUsedPercent > 0 {
				b.WriteString(windowLine(windowName(rl.PrimaryWindowMinutes, "5h"), rl.PrimaryUsedPercent, rl.PrimaryResetsAt, now))
			}
			if rl.SecondaryWindowMins > 0 || rl.SecondaryUsedPercent > 0 {
				b.WriteString(windowLine(windowName(rl.SecondaryWindowMins, "week"), rl.SecondaryUsedPercent, rl.SecondaryResetsAt, now))
			}
		case row.Agent == session.Claude:
			fmt.Fprintf(&b, "   %s%s%s\n", dim, i18n.T("limits.pct_later_claude"), reset)
		case row.Agent == session.Codex:
			fmt.Fprintf(&b, "   %s%s%s\n", dim, i18n.T("limits.pct_later_codex"), reset)
		case row.Agent == session.Copilot:
			fmt.Fprintf(&b, "   %s%s%s\n", dim, i18n.T("limits.pct_later_copilot"), reset)
		case row.Agent == session.OpenCode:
			fmt.Fprintf(&b, "   %s%s%s\n", dim, i18n.T("limits.opencode"), reset)
		default:
			fmt.Fprintf(&b, "   %s%s%s\n", dim, i18n.T("limits.no_pct", name), reset)
		}
		if t := usageText(row, now); t != "" && cq == nil {
			fmt.Fprintf(&b, "   %s%s%s\n", dim, t, reset)
		}
		if row.Limit != nil && row.Limit.Text != "" {
			fmt.Fprintf(&b, "   %s%s%s\n", dim, i18n.T("limits.seen", clean.OneLine(row.Limit.Text, 70), textfmt.Ago(now, row.Limit.At)), reset)
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

const (
	bold  = "\x1b[1m"
	dim   = "\x1b[2m"
	reset = "\x1b[0m"
)

// windowName names a usage window by its length.
func windowName(minutes int, fallback string) string {
	switch {
	case minutes == 0 && fallback == "5h", minutes > 0 && minutes <= 6*60:
		return i18n.T("limits.win_5h")
	case minutes == 0, minutes >= 6*24*60:
		return i18n.T("limits.win_week")
	}
	return i18n.T("limits.win_hours", minutes/60)
}

// windowLine is "   5-hour   ███████░░░  72%   resets 15:00".
func windowLine(name string, pct float64, resets, now time.Time) string {
	const width = 20
	p := min(max(pct, 0), 100)
	full := int(p/100*width + 0.5)
	s := theme.S()
	color := s.Green
	switch {
	case p >= 90:
		color = s.Red
	case p >= 70:
		color = s.Yellow
	}
	bar := color + strings.Repeat("█", full) + reset + dim + strings.Repeat("░", width-full) + reset
	when := ""
	if !resets.IsZero() {
		when = "  " + dim + i18n.T("limits.resets", textfmt.Clock(now, resets)) + reset
	}
	return fmt.Sprintf("   %-9s %s %3.0f%%%s\n", name, bar, p, when)
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
		return theme.S().Red + "◆ " + i18n.T("limits.limited_until", textfmt.Clock(now, t)) + reset
	case row.Status == "warn":
		return theme.S().Yellow + "◐ " + i18n.T("limits.approaching") + reset
	}
	return theme.S().Green + "●" + reset + " " + i18n.T("limits.ok")
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
		return "" // Codex: its windows are drawn as bars
	case "opencode":
		return i18n.T("limits.opencode_tokens", short(u.InputTokens), short(u.OutputTokens), u.Sessions)
	}
	return ""
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

// trimFloat prints 1.6 as "1.6" and 200 as "200".
func trimFloat(f float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.1f", f), "0"), ".")
}
