// Package textfmt formats times and sizes for the UI.
package textfmt

import (
	"fmt"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

// Ago renders t relative to now: "just now", "12 min ago", "2 h ago",
// "yesterday", "3 days ago", or a date for older times.
func Ago(now, t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return i18n.T("time.just_now")
	case d < time.Hour:
		return i18n.T("time.min_ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return i18n.T("time.hours_ago", int(d.Hours()))
	}
	ny, nd := now.Local().Year(), now.Local().YearDay()
	ty, td := t.Local().Year(), t.Local().YearDay()
	days := (ny-ty)*365 + nd - td
	switch {
	case days <= 1:
		return i18n.T("time.yesterday")
	case days < 7:
		return i18n.T("time.days_ago", days)
	case ty == ny:
		return t.Local().Format("Jan 2")
	default:
		return t.Local().Format("Jan 2 2006")
	}
}

// Size renders a byte count: "812 B", "4.2 KB", "63.9 MB".
func Size(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1024*1024*1024))
}

// Clock renders t as "15:04" when it is today (relative to now), else
// "Jan 2 15:04".
func Clock(now, t time.Time) string {
	lt, ln := t.Local(), now.Local()
	if lt.Year() == ln.Year() && lt.YearDay() == ln.YearDay() {
		return lt.Format("15:04")
	}
	return lt.Format("Jan 2 15:04")
}
