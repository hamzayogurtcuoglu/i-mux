// Package usage records the plan usage the agent CLIs report about
// themselves (ARCHITECTURE D25): Claude Code's status-line input carries
// rate_limits.five_hour and .seven_day with used_percentage and resets_at.
package usage

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Window is one usage window.
type Window struct {
	UsedPercent float64   `json:"used_percent"`
	ResetsAt    time.Time `json:"resets_at,omitzero"`
}

// Snapshot is what an agent last reported.
type Snapshot struct {
	At       time.Time `json:"at"`
	FiveHour *Window   `json:"five_hour,omitempty"`
	Weekly   *Window   `json:"weekly,omitempty"`
}

// statusline is the part of Claude Code's status-line input imux reads.
type statusline struct {
	CWD       string `json:"cwd"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
	RateLimits struct {
		FiveHour *rawWindow `json:"five_hour"`
		SevenDay *rawWindow `json:"seven_day"`
	} `json:"rate_limits"`
}

type rawWindow struct {
	UsedPercentage *float64        `json:"used_percentage"`
	ResetsAt       json.RawMessage `json:"resets_at"`
}

// resetTime reads resets_at: unix seconds (or milliseconds), or an
// RFC 3339 string.
func resetTime(raw json.RawMessage) time.Time {
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}
	}
	var n float64
	if json.Unmarshal(raw, &n) == nil && n > 0 {
		if n > 1e12 {
			return time.UnixMilli(int64(n))
		}
		sec, frac := math.Modf(n)
		return time.Unix(int64(sec), int64(frac*1e9))
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func (w *rawWindow) window() *Window {
	if w == nil || w.UsedPercentage == nil {
		return nil
	}
	return &Window{UsedPercent: *w.UsedPercentage, ResetsAt: resetTime(w.ResetsAt)}
}

// ParseClaudeStatusline reads Claude Code's status-line input. It returns
// the usage (nil when the input has no rate limits) and the session's
// folder.
func ParseClaudeStatusline(b []byte, now time.Time) (*Snapshot, string) {
	var in statusline
	if json.Unmarshal(b, &in) != nil {
		return nil, ""
	}
	dir := in.Workspace.CurrentDir
	if dir == "" {
		dir = in.CWD
	}
	s := &Snapshot{At: now, FiveHour: in.RateLimits.FiveHour.window(), Weekly: in.RateLimits.SevenDay.window()}
	if s.FiveHour == nil && s.Weekly == nil {
		return nil, dir
	}
	return s, dir
}

// Save writes a snapshot (0600) atomically.
func Save(path string, s *Snapshot) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Load reads a snapshot (nil when missing).
func Load(path string) *Snapshot {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var s Snapshot
	if json.Unmarshal(b, &s) != nil {
		return nil
	}
	return &s
}

// Current drops windows that have reset since they were recorded (their
// percentage no longer holds).
func (s *Snapshot) Current(now time.Time) *Snapshot {
	if s == nil {
		return nil
	}
	c := *s
	if c.FiveHour != nil && !c.FiveHour.ResetsAt.IsZero() && now.After(c.FiveHour.ResetsAt) {
		c.FiveHour = &Window{ResetsAt: time.Time{}}
	}
	if c.Weekly != nil && !c.Weekly.ResetsAt.IsZero() && now.After(c.Weekly.ResetsAt) {
		c.Weekly = &Window{ResetsAt: time.Time{}}
	}
	return &c
}

// UserStatusLine finds the status-line command the user configured for
// Claude Code in dir (local project settings, project, then user), or "".
func UserStatusLine(home, dir string) string {
	var files []string
	if dir != "" {
		files = append(files, filepath.Join(dir, ".claude", "settings.local.json"), filepath.Join(dir, ".claude", "settings.json"))
	}
	files = append(files, filepath.Join(home, ".claude", "settings.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var s struct {
			StatusLine *struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"statusLine"`
		}
		if json.Unmarshal(b, &s) == nil && s.StatusLine != nil && s.StatusLine.Type == "command" &&
			strings.TrimSpace(s.StatusLine.Command) != "" && !strings.Contains(s.StatusLine.Command, "__statusline") {
			return s.StatusLine.Command
		}
	}
	return ""
}
