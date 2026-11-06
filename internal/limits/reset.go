package limits

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // IANA zones named in limit messages ("(Europe/Istanbul)")
)

var (
	epochRe = regexp.MustCompile(`\|(\d{10})\b`)
	datedRe = regexp.MustCompile(`(?i)(?:resets?(?: at)?|until)\s+([a-z]{3,9})\.?\s+(\d{1,2}),?\s+(?:at\s+)?(\d{1,2})(?::(\d{2}))?\s*(am|pm)?(?:\s*\(([A-Za-z_]+/[A-Za-z_]+(?:/[A-Za-z_]+)?)\))?`)
	clockRe = regexp.MustCompile(`(?i)resets?\s+(?:at\s+)?(\d{1,2})(?::(\d{2}))?\s*(am|pm)?(?:\s*\(([A-Za-z_]+/[A-Za-z_]+(?:/[A-Za-z_]+)?)\))?`)
	tryRe   = regexp.MustCompile(`(?i)try again in\s+((?:\d+\s*(?:seconds?|secs?|minutes?|mins?|hours?|hrs?|days?|h|m|s|d)[\s,]*(?:and\s+)?)+)`)
	partRe  = regexp.MustCompile(`(?i)(\d+)\s*(seconds?|secs?|minutes?|mins?|hours?|hrs?|days?|h|m|s|d)`)
)

var months = map[string]time.Month{
	"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
	"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
}

// ParseReset extracts when a limit resets from a limit message (§10.1).
// The first form that matches wins:
//
//   - Claude's legacy "…|<epoch>" (exact);
//   - "resets Nov 8, 9am" (dated weekly resets);
//   - "resets 3pm", "resets 3:45pm (Europe/Istanbul)", "reset at 15:00": the
//     next such time after now, in the named zone or loc;
//   - "try again in 2 days 3 hours".
//
// It returns the zero time when nothing is recognized.
func ParseReset(text string, now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	if m := epochRe.FindStringSubmatch(text); m != nil {
		if sec, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			return time.Unix(sec, 0)
		}
	}
	if m := datedRe.FindStringSubmatch(text); m != nil {
		if mon, ok := months[strings.ToLower(m[1])[:3]]; ok {
			day, _ := strconv.Atoi(m[2])
			h, min, ok := clock(m[3], m[4], m[5])
			if ok && day >= 1 && day <= 31 {
				l := zone(m[6], loc)
				n := now.In(l)
				t := time.Date(n.Year(), mon, day, h, min, 0, 0, l)
				if t.Before(now.Add(-24 * time.Hour)) {
					t = time.Date(n.Year()+1, mon, day, h, min, 0, 0, l)
				}
				return t
			}
		}
	}
	if m := clockRe.FindStringSubmatch(text); m != nil {
		if h, min, ok := clock(m[1], m[2], m[3]); ok {
			l := zone(m[4], loc)
			n := now.In(l)
			t := time.Date(n.Year(), n.Month(), n.Day(), h, min, 0, 0, l)
			if !t.After(now) {
				t = time.Date(n.Year(), n.Month(), n.Day()+1, h, min, 0, 0, l)
			}
			return t
		}
	}
	if m := tryRe.FindStringSubmatch(text); m != nil {
		var d time.Duration
		for _, p := range partRe.FindAllStringSubmatch(m[1], -1) {
			n, err := strconv.Atoi(p[1])
			if err != nil || n > 1000 {
				continue
			}
			d += time.Duration(n) * unit(p[2])
		}
		if d > 0 {
			return now.Add(d)
		}
	}
	return time.Time{}
}

func clock(hs, ms, ampm string) (int, int, bool) {
	h, err := strconv.Atoi(hs)
	if err != nil {
		return 0, 0, false
	}
	min := 0
	if ms != "" {
		if min, err = strconv.Atoi(ms); err != nil || min > 59 {
			return 0, 0, false
		}
	}
	switch strings.ToLower(ampm) {
	case "am":
		if h < 1 || h > 12 {
			return 0, 0, false
		}
		if h == 12 {
			h = 0
		}
	case "pm":
		if h < 1 || h > 12 {
			return 0, 0, false
		}
		if h != 12 {
			h += 12
		}
	default:
		if h > 23 {
			return 0, 0, false
		}
	}
	return h, min, true
}

func zone(name string, def *time.Location) *time.Location {
	if name == "" {
		return def
	}
	if l, err := time.LoadLocation(name); err == nil {
		return l
	}
	return def
}

func unit(u string) time.Duration {
	switch strings.ToLower(u)[0] {
	case 's':
		return time.Second
	case 'm':
		return time.Minute
	case 'h':
		return time.Hour
	}
	return 24 * time.Hour
}
