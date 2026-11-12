package auto

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// When is a schedule's time: every Every, every hour on the hour, or every
// day at the times in At. The zero When runs only when started by hand.
type When struct {
	Every  time.Duration
	Hourly bool
	At     []int // minutes after midnight, sorted
}

// MinEvery is the shortest interval: each run is a model call.
const MinEvery = time.Minute

// ErrTooOften is ParseWhen's error for an interval under MinEvery.
var ErrTooOften = errors.New("at most once a minute")

// ParseWhen reads a time as the Scheduler stores and the user types it:
// "" (by hand), "5m", "90s"-style durations ("10 min", "5dk", "2 saat"),
// "hourly" ("saat başı"), or times of the day ("09:00 18:30", "9:30").
func ParseWhen(s string) (When, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", "manual", "elle":
		return When{}, nil
	case "hourly", "@hourly", "saat başı", "saatbaşı", "her saat başı", "every hour":
		return When{Hourly: true}, nil
	}
	if strings.Contains(s, ":") {
		var at []int
		for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' || r == ';' }) {
			h, m, ok := strings.Cut(f, ":")
			hh, err1 := strconv.Atoi(h)
			mm, err2 := strconv.Atoi(m)
			if !ok || err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 || len(m) != 2 {
				return When{}, fmt.Errorf("not a time of day: %q", f)
			}
			at = append(at, hh*60+mm)
		}
		sort.Ints(at)
		uniq := at[:0]
		for i, v := range at {
			if i == 0 || v != at[i-1] {
				uniq = append(uniq, v)
			}
		}
		return When{At: uniq}, nil
	}
	d, err := parseEvery(s)
	if err != nil {
		return When{}, err
	}
	if d < MinEvery {
		return When{}, ErrTooOften
	}
	return When{Every: d}, nil
}

// parseEvery reads an interval: Go's durations, with words and Turkish
// units ("every 10 min", "5 dk", "2 saat", "1 gün").
func parseEvery(s string) (time.Duration, error) {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "every "), "her ")
	s = strings.ReplaceAll(s, " ", "")
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Minute, nil // a bare number: minutes
	}
	for _, u := range [][2]string{
		{"minutes", "m"},
		{"minute", "m"},
		{"mins", "m"},
		{"min", "m"},
		{"dakika", "m"},
		{"dk", "m"},
		{"seconds", "s"},
		{"second", "s"},
		{"saniye", "s"},
		{"sn", "s"},
		{"hours", "h"},
		{"hour", "h"},
		{"saat", "h"},
		{"sa", "h"},
	} {
		s = strings.ReplaceAll(s, u[0], u[1])
	}
	if day, ok := strings.CutSuffix(s, "gün"); ok {
		n, err := strconv.Atoi(day)
		if err != nil {
			return 0, fmt.Errorf("not a time: %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	if day, ok := strings.CutSuffix(s, "d"); ok {
		if n, err := strconv.Atoi(day); err == nil {
			return time.Duration(n) * 24 * time.Hour, nil
		}
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("not a time: %q", s)
	}
	return d, nil
}

// Manual reports a schedule that runs only when started by hand.
func (w When) Manual() bool { return w.Every == 0 && !w.Hourly && len(w.At) == 0 }

// String is the stored form: "5m", "2h", "1h30m", "hourly", "09:00 18:30", "".
func (w When) String() string {
	switch {
	case w.Every > 0:
		return short(w.Every)
	case w.Hourly:
		return "hourly"
	case len(w.At) > 0:
		var p []string
		for _, m := range w.At {
			p = append(p, fmt.Sprintf("%02d:%02d", m/60, m%60))
		}
		return strings.Join(p, " ")
	}
	return ""
}

// English is the time as a run's prompt says it: "every 5m", "every hour
// on the hour", "every day at 09:00 and 18:30"; "" by hand.
func (w When) English() string {
	switch {
	case w.Every > 0:
		return "every " + short(w.Every)
	case w.Hourly:
		return "every hour on the hour"
	case len(w.At) > 0:
		p := strings.Fields(w.String())
		if len(p) == 1 {
			return "every day at " + p[0]
		}
		return "every day at " + strings.Join(p[:len(p)-1], ", ") + " and " + p[len(p)-1]
	}
	return ""
}

// short is a duration without Go's zero units ("5m", not "5m0s").
func short(d time.Duration) string {
	if d%(24*time.Hour) == 0 {
		return strconv.Itoa(int(d/(24*time.Hour))) + "d"
	}
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// Next is the first time after `after` the schedule is due (in after's
// time zone); the zero time for one that runs only by hand. An interval's
// next time is after + Every.
func (w When) Next(after time.Time) time.Time {
	switch {
	case w.Every > 0:
		return after.Add(w.Every)
	case w.Hourly:
		return time.Date(after.Year(), after.Month(), after.Day(), after.Hour()+1, 0, 0, 0, after.Location())
	case len(w.At) > 0:
		for day := 0; day < 2; day++ {
			for _, m := range w.At {
				t := time.Date(after.Year(), after.Month(), after.Day()+day, m/60, m%60, 0, 0, after.Location())
				if t.After(after) {
					return t
				}
			}
		}
	}
	return time.Time{}
}

// Due reports whether a schedule made at created, whose last run started at
// last (zero: never), is due at now; and when it is next due. An interval
// that never ran is due at once; a time of day or the hour is due at its
// first occurrence after the last run, or after the schedule was made: a
// time missed while i-mux was closed runs once when it opens.
func (w When) Due(created, last, now time.Time) (bool, time.Time) {
	if w.Manual() {
		return false, time.Time{}
	}
	if w.Every > 0 && last.IsZero() {
		return true, now
	}
	base := last
	if base.IsZero() {
		base = created
	}
	next := w.Next(base.In(now.Location()))
	return !next.After(now), next
}
