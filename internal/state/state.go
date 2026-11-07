// Package state reads and updates ~/.imux/state.json: detected limits, the
// agent binary cache and one-time notices.
//
// Writers serialize with flock(state.lock) and replace the file atomically
// (temp file, fsync, rename), so readers never see a partial file and
// concurrent writers never lose each other's updates. Unknown top-level
// fields survive a read-modify-write.
package state

import (
	"encoding/json"
	"errors"
	"os"
	"syscall"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
)

// SchemaVersion is the current state.json version.
const SchemaVersion = 1

// Limit is a detected usage limit of one agent.
type Limit struct {
	At        time.Time `json:"at"`
	Text      string    `json:"text"`
	ResetAt   time.Time `json:"reset_at,omitzero"`
	ExpiresAt time.Time `json:"expires_at"`
	Source    string    `json:"source"` // "screen" | "session"
	Pane      string    `json:"pane,omitempty"`
	Severity  string    `json:"severity"` // "limit" | "warn"
}

// Active reports whether the limit is still in effect at now.
func (l *Limit) Active(now time.Time) bool {
	return l != nil && now.Before(l.ExpiresAt)
}

// Bin caches where an agent binary lives and the PATH it needs.
type Bin struct {
	Path      string    `json:"path"`
	PathEnv   string    `json:"path_env"`
	Version   string    `json:"version,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// State is the content of state.json.
type State struct {
	Version int               `json:"version"`
	Limits  map[string]*Limit `json:"limits,omitempty"`
	Bins    map[string]*Bin   `json:"bins,omitempty"`
	Notices map[string]bool   `json:"notices,omitempty"`
	// BusSends holds recent send timestamps (unix ms) per sender pane, for
	// the bus rate limit.
	BusSends map[string][]int64 `json:"bus_sends,omitempty"`
	// Recent holds the project folders started most recently, newest first.
	Recent []Recent `json:"recent,omitempty"`

	extra map[string]json.RawMessage
}

// known lists the JSON keys of typed fields.
var known = []string{"version", "limits", "bins", "notices", "bus_sends", "recent"}

type plain State // no methods: avoids recursion

// UnmarshalJSON keeps unknown top-level fields.
func (s *State) UnmarshalJSON(b []byte) error {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return err
	}
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*s = State(p)
	for _, k := range known {
		delete(all, k)
	}
	if len(all) > 0 {
		s.extra = all
	}
	return nil
}

// MarshalJSON writes typed fields over the preserved unknown ones.
func (s State) MarshalJSON() ([]byte, error) {
	typed, err := json.Marshal(plain(s))
	if err != nil {
		return nil, err
	}
	if len(s.extra) == 0 {
		return typed, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(typed, &m); err != nil {
		return nil, err
	}
	for k, v := range s.extra {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
	return json.Marshal(m)
}

func newState() *State { return &State{Version: SchemaVersion} }

// Read returns the current state. A missing or unreadable file yields an
// empty state (the file is a cache; it is rebuilt on the next update).
func Read() (*State, error) {
	b, err := os.ReadFile(paths.State())
	if errors.Is(err, os.ErrNotExist) {
		return newState(), nil
	}
	if err != nil {
		return nil, err
	}
	s := newState()
	if err := json.Unmarshal(b, s); err != nil {
		return newState(), nil
	}
	if s.Version == 0 {
		s.Version = SchemaVersion
	}
	return s, nil
}

// Update runs fn on the current state under an exclusive lock and writes the
// result atomically. If fn returns an error, nothing is written.
func Update(fn func(s *State) error) error {
	if err := paths.EnsureDir(paths.Home()); err != nil {
		return err
	}
	lf, err := os.OpenFile(paths.StateLock(), os.O_RDWR|os.O_CREATE, paths.FileMode)
	if err != nil {
		return err
	}
	defer lf.Close()
	if err := flock(lf, syscall.LOCK_EX); err != nil {
		return err
	}
	defer flock(lf, syscall.LOCK_UN) //nolint:errcheck // released on close anyway

	s, err := Read()
	if err != nil {
		return err
	}
	if err := fn(s); err != nil {
		return err
	}
	s.Version = SchemaVersion
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return paths.WriteFile(paths.State(), append(b, '\n'))
}

func flock(f *os.File, how int) error {
	for {
		err := syscall.Flock(int(f.Fd()), how)
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}

// Blocking reports whether the limit stops the agent (a "warn" record is
// only shown in the status bar).
func (l *Limit) Blocking() bool { return l != nil && l.Severity != "warn" }

// ActiveLimit returns the agent's blocking limit if it is still in effect.
func (s *State) ActiveLimit(agent string, now time.Time) *Limit {
	if s == nil {
		return nil
	}
	l := s.Limits[agent]
	if l.Active(now) && l.Blocking() {
		return l
	}
	return nil
}

// ActiveRecord returns the agent's limit or warning if still in effect.
func (s *State) ActiveRecord(agent string, now time.Time) *Limit {
	if s == nil {
		return nil
	}
	if l := s.Limits[agent]; l.Active(now) {
		return l
	}
	return nil
}

// DropExpired removes limits that are no longer in effect and reports whether
// anything changed.
func (s *State) DropExpired(now time.Time) bool {
	changed := false
	for k, l := range s.Limits {
		if !l.Active(now) {
			delete(s.Limits, k)
			changed = true
		}
	}
	return changed
}

// SetLimit records a limit for agent.
func (s *State) SetLimit(agent string, l *Limit) {
	if s.Limits == nil {
		s.Limits = map[string]*Limit{}
	}
	s.Limits[agent] = l
}

// SetBin records the binary cache entry for agent.
func (s *State) SetBin(agent string, b *Bin) {
	if s.Bins == nil {
		s.Bins = map[string]*Bin{}
	}
	s.Bins[agent] = b
}

// SetNotice marks a one-time notice as shown.
func (s *State) SetNotice(key string) {
	if s.Notices == nil {
		s.Notices = map[string]bool{}
	}
	s.Notices[key] = true
}

// Recent is a project folder and when it was last started.
type Recent struct {
	Path string `json:"path"`
	At   int64  `json:"at"` // unix seconds
}

// MaxRecent bounds the recent folders kept.
const MaxRecent = 30

// AddRecent puts dir first in the recent folders.
func (s *State) AddRecent(dir string, at time.Time) {
	out := []Recent{{Path: dir, At: at.Unix()}}
	for _, r := range s.Recent {
		if r.Path != dir && len(out) < MaxRecent {
			out = append(out, r)
		}
	}
	s.Recent = out
}
