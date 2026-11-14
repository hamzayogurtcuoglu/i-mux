package auto

import (
	"strconv"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

// Label is the time as the UI says it: "every 5 min", "hourly", "daily
// 09:00 18:30", "by hand".
func (w When) Label() string {
	switch {
	case w.Every > 0:
		var p []string
		d := w.Every
		if n := d / (24 * time.Hour); n > 0 {
			p = append(p, strconv.Itoa(int(n))+" "+i18n.T("auto.unit_d"))
			d -= n * 24 * time.Hour
		}
		if n := d / time.Hour; n > 0 {
			p = append(p, strconv.Itoa(int(n))+" "+i18n.T("auto.unit_h"))
			d -= n * time.Hour
		}
		if n := d / time.Minute; n > 0 {
			p = append(p, strconv.Itoa(int(n))+" "+i18n.T("auto.unit_m"))
			d -= n * time.Minute
		}
		if n := d / time.Second; n > 0 { // "90s" runs every 1 min 30 s
			p = append(p, strconv.Itoa(int(n))+" "+i18n.T("auto.unit_s"))
		}
		return i18n.T("auto.every", strings.Join(p, " "))
	case w.Hourly:
		return i18n.T("auto.hourly")
	case len(w.At) > 0:
		return i18n.T("auto.daily", w.String())
	}
	return i18n.T("auto.by_hand")
}

// Text is what a run says in a list: its summary, or what its status means.
func (r Run) Text() string {
	switch {
	case r.Status == StatusQuiet:
		return i18n.T("auto.quiet")
	case r.Summary != "":
		return r.Summary
	case r.Status == StatusStopped:
		return i18n.T("auto.stopped")
	case r.Status == StatusTimeout:
		return i18n.T("auto.timed_out")
	case r.Status == StatusFailed:
		return i18n.T("auto.failed", r.Exit)
	}
	return ""
}

// Took is how long something took: "12s", "3m", "1h 5m".
func Took(d time.Duration) string {
	switch {
	case d < 0:
		return "?"
	case d < time.Minute:
		return strconv.Itoa(int(d/time.Second)) + "s"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + "m"
	}
	return strconv.Itoa(int(d/time.Hour)) + "h " + strconv.Itoa(int(d%time.Hour/time.Minute)) + "m"
}
