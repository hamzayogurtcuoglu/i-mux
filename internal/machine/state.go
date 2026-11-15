package machine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// Report is what a machine's `imux __remote-state` prints.
type Report struct {
	Version  string    `json:"version"`
	Projects []Project `json:"projects"`
}

// Project is one of a machine's projects.
type Project struct {
	Session string `json:"session"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Branch  string `json:"branch,omitempty"`
	Panes   []Pane `json:"panes"`
}

// Pane is one of a project's agent panes.
type Pane struct {
	ID    string `json:"id"`
	Agent string `json:"agent"`
	Name  string `json:"name,omitempty"`
	State string `json:"state"`           // working, idle, blocked, done… (the sidebar's)
	Since int64  `json:"since,omitempty"` // unix seconds of the state
}

// Limits on what a machine sends (D73).
const (
	MaxReport   = 1 << 20
	MaxProjects = 200
	MaxPanes    = 50
	MaxText     = 80
	Timeout     = 15 * time.Second
)

// Clean is a text a machine sent, safe to show: no control characters (no
// escape sequence reaches the terminal), at most MaxText runes.
func Clean(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r >= 0x80 && r < 0xa0 || r == 0x2028 || r == 0x2029 {
			continue
		}
		if n == MaxText {
			b.WriteRune('…')
			break
		}
		b.WriteRune(r)
		n++
	}
	return b.String()
}

var states = map[string]bool{"working": true, "idle": true, "blocked": true, "done": true, "shell": true, "exited": true}

// Parse reads a machine's report as untrusted input: limited in size and
// counts, ids that do not match their pattern dropped, unknown agents and
// states blanked, every text cleaned.
func Parse(b []byte) (*Report, error) {
	if len(b) > MaxReport {
		return nil, errors.New("the machine's report is too large")
	}
	var r Report
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, errors.New("the machine's i-mux sent no report it can read (update i-mux there)")
	}
	out := &Report{Version: Clean(r.Version)}
	for _, p := range r.Projects {
		if len(out.Projects) == MaxProjects {
			break
		}
		if !ValidSession(p.Session) {
			continue
		}
		q := Project{Session: p.Session, Name: Clean(p.Name), Path: Clean(p.Path), Branch: Clean(p.Branch)}
		if q.Name == "" {
			q.Name = p.Session
		}
		for _, x := range p.Panes {
			if len(q.Panes) == MaxPanes {
				break
			}
			if !ValidPane(x.ID) {
				continue
			}
			y := Pane{ID: x.ID, Name: Clean(x.Name), Since: x.Since}
			if session.Known(x.Agent) {
				y.Agent = x.Agent
			}
			if states[x.State] {
				y.State = x.State
			}
			q.Panes = append(q.Panes, y)
		}
		out.Projects = append(out.Projects, q)
	}
	return out, nil
}

// Status of a machine's connection.
const (
	StatusOK         = "ok"
	StatusConnecting = "connecting"
	StatusOffline    = "offline" // SSH failed: Error says why
	StatusAuth       = "auth"    // only an interactive sign-in would do (a password, a host key)
	StatusNoImux     = "noimux"  // reached, without i-mux there
)

// State is what the watcher last heard from a machine, as the sidebar reads
// it.
type State struct {
	Status   string    `json:"status"`
	Error    string    `json:"error,omitempty"`
	At       time.Time `json:"at"`
	Version  string    `json:"version,omitempty"`
	Projects []Project `json:"projects,omitempty"`
}

// CacheFile is where a machine's State is kept.
func CacheFile(name string) string { return filepath.Join(paths.Home(), "machines", name+".json") }

// ReadState reads a machine's State (Connecting when there is none yet).
func ReadState(name string) State {
	b, err := os.ReadFile(CacheFile(name))
	if err != nil {
		return State{Status: StatusConnecting}
	}
	var s State
	if json.Unmarshal(b, &s) != nil {
		return State{Status: StatusConnecting}
	}
	return s
}

// WriteState keeps a machine's State (0600).
func WriteState(name string, s State) error {
	if err := paths.EnsureDir(filepath.Dir(CacheFile(name))); err != nil {
		return err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return paths.WriteFile(CacheFile(name), b)
}

// Fetch asks m's i-mux for its report, in the background (never asking the
// user anything), within Timeout.
func Fetch(ctx context.Context, m Machine) (*Report, State) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	_ = os.MkdirAll(ControlDir(), 0o700) // else ssh shares no connection
	cmd := exec.CommandContext(ctx, SSHBin(), Args(m, Background, "__remote-state")...)
	// A master connection kept in the background may hold the pipes open:
	// the answer is in once ssh itself exits.
	cmd.WaitDelay = time.Second
	var out bytes.Buffer
	var errb limited
	cmd.Stdout, cmd.Stderr = &limitWriter{w: &out, n: MaxReport + 1}, &errb
	cmd.Stdin = nil
	err := cmd.Run()
	if err == nil {
		r, perr := Parse(out.Bytes())
		if perr != nil {
			return nil, State{Status: StatusOffline, Error: perr.Error()}
		}
		return r, State{Status: StatusOK, Version: r.Version, Projects: r.Projects}
	}
	return nil, failed(err, errb.String(), ctx.Err())
}

// failed is the State of an ssh that failed: why, in SSH's last line.
func failed(err error, stderr string, ctxErr error) State {
	last := ""
	for _, l := range strings.Split(strings.TrimSpace(stderr), "\n") {
		if l = Clean(strings.TrimSpace(l)); l != "" {
			last = l
		}
	}
	low := strings.ToLower(stderr)
	var ee *exec.ExitError
	switch {
	case ctxErr != nil:
		return State{Status: StatusOffline, Error: "timed out"}
	case strings.Contains(low, "permission denied") || strings.Contains(low, "host key verification failed") ||
		strings.Contains(low, "authenticity of host") || strings.Contains(low, "no matching host key"):
		return State{Status: StatusAuth, Error: last}
	case errors.As(err, &ee) && ee.ExitCode() == 127:
		return State{Status: StatusNoImux, Error: last}
	}
	if last == "" {
		last = Clean(err.Error())
	}
	return State{Status: StatusOffline, Error: last}
}

// limited keeps the last 4 KB written to it (ssh's errors).
type limited struct{ b []byte }

func (l *limited) Write(p []byte) (int, error) {
	l.b = append(l.b, p...)
	if len(l.b) > 4096 {
		l.b = l.b[len(l.b)-4096:]
	}
	return len(p), nil
}

func (l *limited) String() string { return string(l.b) }

// limitWriter stops keeping after n bytes (the rest is dropped: Parse then
// refuses a report that long).
type limitWriter struct {
	w io.Writer
	n int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if l.n <= 0 {
		return len(p), nil
	}
	k := min(len(p), l.n)
	l.n -= k
	if _, err := l.w.Write(p[:k]); err != nil {
		return 0, err
	}
	return len(p), nil
}
