package auto

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
)

// Status is how a run ended.
type Status string

// Run states.
const (
	StatusOK      Status = "ok"      // it reported something
	StatusQuiet   Status = "quiet"   // nothing new (IMUX_NOTHING_NEW)
	StatusFailed  Status = "failed"  // the CLI exited with an error
	StatusTimeout Status = "timeout" // killed after the schedule's timeout
	StatusStopped Status = "stopped" // stopped by hand
	StatusSkipped Status = "skipped" // not started: the CLI is missing or limited, the definition gone
)

// Run is one run of a schedule.
type Run struct {
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Status  Status    `json:"status"`
	Exit    int       `json:"exit,omitempty"`
	Summary string    `json:"summary,omitempty"` // the report's (or error's) first line
	Report  string    `json:"report,omitempty"`  // the report, for the next run (MaxReport)
	Log     string    `json:"log,omitempty"`     // its log's file name in Dir
	CLI     string    `json:"cli,omitempty"`
	Model   string    `json:"model,omitempty"`
	Session string    `json:"session,omitempty"` // the CLI's session id when i-mux chose it (Claude)
	Manual  bool      `json:"manual,omitempty"`  // started by hand
}

// Duration is how long the run took.
func (r Run) Duration() time.Duration { return r.End.Sub(r.Start) }

// Mark is the status's glyph in the sidebar and the lists.
func (s Status) Mark() string {
	switch s {
	case StatusOK:
		return "✓"
	case StatusQuiet:
		return "·"
	case StatusFailed:
		return "✗"
	case StatusTimeout:
		return "◷"
	case StatusStopped:
		return "■"
	case StatusSkipped:
		return "↷"
	}
	return "?"
}

// Failed reports a run that went wrong (an error exit or a timeout).
func (s Status) Failed() bool { return s == StatusFailed || s == StatusTimeout }

// MaxRuns runs (and their logs) are kept per schedule.
const MaxRuns = 20

// MaxReport bounds the report kept for the next run's prompt.
const MaxReport = 4000

// Dir is a schedule's folder of runs.
func Dir(id string) string { return filepath.Join(paths.Home(), "auto", id) }

func runsFile(id string) string { return filepath.Join(Dir(id), "runs.json") }

// Runs are a schedule's runs, the newest first.
func Runs(id string) []Run {
	b, err := os.ReadFile(runsFile(id))
	if err != nil {
		return nil
	}
	var rs []Run
	if json.Unmarshal(b, &rs) != nil {
		return nil
	}
	return rs
}

// Last is a schedule's last run (nil before the first).
func Last(id string) *Run {
	rs := Runs(id)
	if len(rs) == 0 {
		return nil
	}
	return &rs[0]
}

// LastStart is when the last run started (zero before the first).
func LastStart(id string) time.Time {
	if r := Last(id); r != nil {
		return r.Start
	}
	return time.Time{}
}

// addRun records r first and drops the runs (and logs) past MaxRuns. The
// caller holds the run lock.
func addRun(id string, r Run) error {
	if err := paths.EnsureDir(Dir(id)); err != nil {
		return err
	}
	r.Report = cut(r.Report, MaxReport)
	rs := append([]Run{r}, Runs(id)...)
	for len(rs) > MaxRuns {
		old := rs[len(rs)-1]
		if old.Log != "" {
			_ = os.Remove(filepath.Join(Dir(id), old.Log))
		}
		rs = rs[:len(rs)-1]
	}
	b, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return err
	}
	return paths.WriteFile(runsFile(id), append(b, '\n'))
}

// Current is the run going on.
type Current struct {
	Start time.Time `json:"start"`
	PGID  int       `json:"pgid,omitempty"`
	Log   string    `json:"log,omitempty"`
	CLI   string    `json:"cli,omitempty"`
	Model string    `json:"model,omitempty"`
}

func currentFile(id string) string { return filepath.Join(Dir(id), "current.json") }
func lockFile(id string) string    { return filepath.Join(Dir(id), "run.lock") }
func stopFile(id string) string    { return filepath.Join(Dir(id), "stop") }

// ErrRunning is returned when the schedule already runs.
var ErrRunning = errors.New("already running")

// lockTries is how often lock tries a lock that is held, lockWait apart:
// Running (a sidebar drawing) holds it for a moment, a run for minutes.
const (
	lockTries = 8
	lockWait  = 25 * time.Millisecond
)

// lock takes the schedule's run lock (ErrRunning when another run holds it).
func lock(id string) (*os.File, error) {
	if err := paths.EnsureDir(Dir(id)); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(lockFile(id), os.O_RDWR|os.O_CREATE, paths.FileMode)
	if err != nil {
		return nil, err
	}
	for try := 1; ; try++ {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) || try == lockTries {
			f.Close()
			if errors.Is(err, syscall.EWOULDBLOCK) {
				return nil, ErrRunning
			}
			return nil, err
		}
		time.Sleep(lockWait)
	}
}

// Running reports whether a run of the schedule goes on, and what it is.
func Running(id string) (Current, bool) {
	f, err := os.OpenFile(lockFile(id), os.O_RDWR, 0)
	if err != nil {
		return Current{}, false
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		return Current{}, false
	}
	var c Current
	if b, err := os.ReadFile(currentFile(id)); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c, true
}

func writeCurrent(id string, c Current) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return paths.WriteFile(currentFile(id), b)
}

// Stop ends the schedule's run: its process group gets SIGTERM, then
// SIGKILL when it is still there after two seconds. It reports whether one
// ran.
func Stop(id string) bool {
	c, ok := Running(id)
	if !ok || c.PGID <= 1 {
		return false
	}
	_ = os.WriteFile(stopFile(id), nil, paths.FileMode)
	_ = syscall.Kill(-c.PGID, syscall.SIGTERM)
	for range 20 {
		time.Sleep(100 * time.Millisecond)
		if _, still := Running(id); !still {
			return true
		}
	}
	_ = syscall.Kill(-c.PGID, syscall.SIGKILL)
	return true
}
