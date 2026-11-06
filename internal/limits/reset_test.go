package limits

import (
	"testing"
	"time"
)

func TestParseReset(t *testing.T) {
	ist, _ := time.LoadLocation("Europe/Istanbul")
	ny, _ := time.LoadLocation("America/New_York")
	now := time.Date(2026, 11, 5, 14, 32, 0, 0, ist) // Thursday afternoon in Istanbul
	cases := []struct {
		name string
		text string
		now  time.Time
		loc  *time.Location
		want time.Time
	}{
		{"claude bullet", "5-hour limit reached ∙ resets 3pm", now, ist, time.Date(2026, 11, 5, 15, 0, 0, 0, ist)},
		{"minutes + zone", "Usage limit reached ∙ resets 3:45pm (Europe/Istanbul)", now, time.UTC, time.Date(2026, 11, 5, 15, 45, 0, 0, ist)},
		{"other zone", "resets 9am (America/New_York)", now, ist, time.Date(2026, 11, 5, 9, 0, 0, 0, ny)},
		{"24h", "Your limit will reset at 15:00", now, ist, time.Date(2026, 11, 5, 15, 0, 0, 0, ist)},
		{"rollover past midnight", "resets 1am", now, ist, time.Date(2026, 11, 6, 1, 0, 0, 0, ist)},
		{"earlier today → tomorrow", "resets 2pm", now, ist, time.Date(2026, 11, 6, 14, 0, 0, 0, ist)},
		{"12am", "resets 12am", now, ist, time.Date(2026, 11, 6, 0, 0, 0, 0, ist)},
		{"12pm", "resets 12pm", now, ist, time.Date(2026, 11, 6, 12, 0, 0, 0, ist)},
		{"legacy epoch", "Claude AI usage limit reached|1762354800", now, ist, time.Unix(1762354800, 0)},
		{"dated weekly", "Weekly limit reached ∙ resets Nov 8, 9am", now, ist, time.Date(2026, 11, 8, 9, 0, 0, 0, ist)},
		{"dated next year", "resets Jan 2, 10:30am", now, ist, time.Date(2027, 1, 2, 10, 30, 0, 0, ist)},
		{"try again", "You've hit your usage limit. Try again in 2 days 3 hours.", now, ist, now.Add(51 * time.Hour)},
		{"try again minutes", "try again in 45 minutes", now, ist, now.Add(45 * time.Minute)},
		{"try again compact", "Try again in 1h 30m", now, ist, now.Add(90 * time.Minute)},
		// DST: New York falls back on 2026-11-01 at 02:00.
		{"dst fall back", "resets 3am (America/New_York)", time.Date(2026, 10, 31, 23, 0, 0, 0, ny), ist, time.Date(2026, 11, 1, 3, 0, 0, 0, ny)},
		{"unparseable", "5-hour limit reached", now, ist, time.Time{}},
		{"bad hour", "resets 13pm", now, ist, time.Time{}},
		{"bad minutes", "resets 3:75pm", now, ist, time.Time{}},
	}
	for _, c := range cases {
		got := ParseReset(c.text, c.now, c.loc)
		if !got.Equal(c.want) {
			t.Errorf("%s: ParseReset(%q) = %v, want %v", c.name, c.text, got, c.want)
		}
	}
	// The DST case: 3am on the fall-back day is 4 hours after 23:00 the
	// evening before (an extra hour), not 4 hours of wall clock difference.
	d := ParseReset("resets 3am (America/New_York)", time.Date(2026, 10, 31, 23, 0, 0, 0, ny), nil)
	if got := d.Sub(time.Date(2026, 10, 31, 23, 0, 0, 0, ny)); got != 5*time.Hour {
		t.Errorf("DST elapsed %v, want 5h", got)
	}
}

func FuzzParseReset(f *testing.F) {
	for _, s := range []string{"resets 3pm", "resets Nov 8, 9am", "|1762354800", "try again in 2 days 3 hours", "resets 3:45pm (Europe/Istanbul)", "reset at 99:99"} {
		f.Add(s)
	}
	now := time.Date(2026, 11, 5, 14, 32, 0, 0, time.UTC)
	f.Fuzz(func(t *testing.T, s string) {
		r := ParseReset(s, now, time.UTC)
		if !r.IsZero() && (r.Year() < 2001 || r.Year() > 2300) {
			// Only the epoch form can land far away; others stay near now.
			if !epochRe.MatchString(s) {
				t.Fatalf("implausible reset %v for %q", r, s)
			}
		}
	})
}
