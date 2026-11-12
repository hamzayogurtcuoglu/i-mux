package auto

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
)

// Access is what a scheduled run may do.
const (
	AccessFull = "full" // run commands and change files without asking (the default: nobody is there)
	AccessRead = "read" // only read
)

// Schedule is one entry of schedules.toml: what runs, with which CLI and
// model, and when.
type Schedule struct {
	ID      string    `toml:"id"`
	Project string    `toml:"project"`
	Kind    Kind      `toml:"kind"`
	Name    string    `toml:"name"`
	File    string    `toml:"file"`
	CLI     string    `toml:"cli"`
	Model   string    `toml:"model"`
	When    string    `toml:"when"`
	Access  string    `toml:"access"`
	Paused  bool      `toml:"paused"`
	Created time.Time `toml:"created"`
	Timeout string    `toml:"timeout,omitempty"`
}

// ReadOnly reports a schedule that may only read.
func (s Schedule) ReadOnly() bool { return s.Access == AccessRead }

// Parsed is the schedule's time (by hand when it does not parse).
func (s Schedule) Parsed() When {
	w, err := ParseWhen(s.When)
	if err != nil {
		return When{}
	}
	return w
}

// DefaultTimeout ends a run that takes longer.
const DefaultTimeout = 20 * time.Minute

// RunTimeout is the schedule's timeout.
func (s Schedule) RunTimeout() time.Duration {
	if d, err := time.ParseDuration(s.Timeout); err == nil && d > 0 {
		return d
	}
	return DefaultTimeout
}

type scheduleFile struct {
	Schedule []Schedule `toml:"schedule"`
}

// Path is schedules.toml.
func Path() string { return filepath.Join(paths.Home(), "schedules.toml") }

func lockPath() string { return filepath.Join(paths.Home(), "schedules.lock") }

const header = "# i-mux's Scheduler (the Schedule view writes this file; you may edit it).\n" +
	"# when: 5m, 2h, hourly, \"09:00 18:30\", or \"\" (only by hand); access: full or read.\n\n"

// Load reads schedules.toml (none when it is missing).
func Load() ([]Schedule, error) {
	b, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var f scheduleFile
	if err := toml.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return f.Schedule, nil
}

// Update runs fn on the schedules under schedules.lock and writes what it
// returns (atomically); an error from fn writes nothing.
func Update(fn func([]Schedule) ([]Schedule, error)) error {
	if err := paths.EnsureDir(paths.Home()); err != nil {
		return err
	}
	lf, err := os.OpenFile(lockPath(), os.O_RDWR|os.O_CREATE, paths.FileMode)
	if err != nil {
		return err
	}
	defer lf.Close()
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lf.Fd()), syscall.LOCK_UN) //nolint:errcheck // released on close anyway
	ss, err := Load()
	if err != nil {
		return err
	}
	ss, err = fn(ss)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.WriteString(header)
	if err := toml.NewEncoder(&buf).Encode(scheduleFile{Schedule: ss}); err != nil {
		return err
	}
	return paths.WriteFile(Path(), buf.Bytes())
}

// Get returns the schedule with id.
func Get(ss []Schedule, id string) (Schedule, bool) {
	for _, s := range ss {
		if s.ID == id {
			return s, true
		}
	}
	return Schedule{}, false
}

// Change applies fn to the schedule with id (made from def when missing:
// fn sees a new entry with Created set) and saves.
func Change(id string, def *Def, project string, now time.Time, fn func(*Schedule)) (Schedule, error) {
	var out Schedule
	err := Update(func(ss []Schedule) ([]Schedule, error) {
		for i := range ss {
			if ss[i].ID == id {
				fn(&ss[i])
				out = ss[i]
				return ss, nil
			}
		}
		if def == nil {
			return nil, errors.New("no such schedule")
		}
		s := Schedule{
			ID: id, Project: project, Kind: def.Kind, Name: def.Name, File: def.File,
			Access: AccessFull, Created: now.UTC().Truncate(time.Second),
		}
		fn(&s)
		out = s
		return append(ss, s), nil
	})
	return out, err
}

// Remove deletes the schedule with id and its runs.
func Remove(id string) error {
	err := Update(func(ss []Schedule) ([]Schedule, error) {
		out := ss[:0]
		for _, s := range ss {
			if s.ID != id {
				out = append(out, s)
			}
		}
		return out, nil
	})
	if err != nil {
		return err
	}
	return os.RemoveAll(Dir(id))
}
