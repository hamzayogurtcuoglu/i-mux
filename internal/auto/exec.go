package auto

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
)

// Execution is a run that holds its schedule's lock.
type Execution struct {
	id   string
	lock *os.File
}

// Begin takes the schedule's run lock (ErrRunning when a run goes on).
func Begin(id string) (*Execution, error) {
	f, err := lock(id)
	if err != nil {
		return nil, err
	}
	_ = os.Remove(stopFile(id))
	return &Execution{id: id, lock: f}, nil
}

// Started records the run going on (the sidebar shows it, Stop ends it).
func (e *Execution) Started(c Current) { _ = writeCurrent(e.id, c) }

// Stopped reports whether Stop ended the run.
func (e *Execution) Stopped() bool {
	_, err := os.Stat(stopFile(e.id))
	return err == nil
}

// Finish records the run and lets the next one start.
func (e *Execution) Finish(r Run) error {
	err := addRun(e.id, r)
	_ = os.Remove(currentFile(e.id))
	_ = os.Remove(stopFile(e.id))
	_ = syscall.Flock(int(e.lock.Fd()), syscall.LOCK_UN)
	e.lock.Close()
	return err
}

// Abort lets the lock go without a run: one that was not due any more.
func (e *Execution) Abort() {
	_ = syscall.Flock(int(e.lock.Fd()), syscall.LOCK_UN)
	e.lock.Close()
}

// LogName is a run's log file name.
func LogName(start time.Time) string { return start.UTC().Format("20060102-150405") + ".log" }

// LogPath is a run's log.
func LogPath(id, name string) string { return filepath.Join(Dir(id), name) }

// Outcome is how the process ended.
type Outcome struct {
	Stdout   string
	Stderr   string
	Exit     int
	TimedOut bool
	Err      error // it did not start, or it failed without an exit code
}

// maxOutput is how much of stdout and stderr is kept for the result (the
// log has it all).
const maxOutput = 256 << 10

type capped struct {
	buf bytes.Buffer
	max int
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// Execute runs argv in dir with env and stdin empty, its output into the
// log at logPath; its process group is killed after timeout or when ctx
// ends. started gets the process group id once it runs.
func Execute(ctx context.Context, argv, env []string, dir, logPath string, timeout time.Duration, started func(pgid int)) Outcome {
	if err := paths.EnsureDir(filepath.Dir(logPath)); err != nil {
		return Outcome{Err: err}
	}
	logf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, paths.FileMode)
	if err != nil {
		return Outcome{Err: err}
	}
	defer logf.Close()
	shown := make([]string, 0, len(argv))
	for i, a := range argv {
		if i > 0 && (strings.ContainsAny(a, "\n") || len(a) > 120) {
			a = "<prompt>"
		}
		shown = append(shown, a)
	}
	fmt.Fprintf(logf, "# %s\n$ %s\n\n", time.Now().Format("2006-01-02 15:04:05"), strings.Join(shown, " "))
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir, cmd.Env = dir, env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	out, errb := &capped{max: maxOutput}, &capped{max: maxOutput}
	cmd.Stdout, cmd.Stderr = io.MultiWriter(logf, out), io.MultiWriter(logf, errb)
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(logf, "! %v\n", err)
		return Outcome{Err: err}
	}
	if started != nil {
		started(cmd.Process.Pid)
	}
	err = cmd.Wait()
	o := Outcome{Stdout: out.buf.String(), Stderr: errb.buf.String()}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		o.TimedOut = true
	}
	var ee *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &ee):
		o.Exit = ee.ExitCode()
		if o.Exit == 0 {
			o.Exit = -1 // killed by a signal
		}
	default:
		o.Err = err
	}
	fmt.Fprintf(logf, "\n# exit %d", o.Exit)
	if o.TimedOut {
		fmt.Fprintf(logf, " (timed out after %s)", timeout)
	}
	fmt.Fprintln(logf)
	return o
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(\x07|\x1b\\)`)

// Clean is a reply without terminal escapes and \r.
func Clean(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(ansiRE.ReplaceAllString(s, ""), "\r", ""))
}

// mdLead is Markdown a summary line starts with.
var mdLead = regexp.MustCompile(`^(?:#{1,6}\s+|>\s*|[-*•+]\s+|\d+[.)]\s+)+`)

// Summarize is a report's first line (Markdown left out, at most 200
// characters) and whether the run had nothing new.
func Summarize(report string) (summary string, quiet bool) {
	report = Clean(report)
	lines := strings.Split(report, "\n")
	var nonEmpty []string
	for _, l := range lines {
		if t := strings.TrimSpace(l); t != "" {
			nonEmpty = append(nonEmpty, t)
		}
	}
	if len(nonEmpty) == 0 {
		return "", true
	}
	strip := func(s string) string { return strings.Trim(s, "`*_ ") }
	if strip(nonEmpty[0]) == NothingNew || strip(nonEmpty[len(nonEmpty)-1]) == NothingNew {
		return "", true
	}
	first := mdLead.ReplaceAllString(nonEmpty[0], "")
	first = strings.NewReplacer("**", "", "__", "", "`", "").Replace(first)
	return cut(strings.TrimSpace(first), 200), false
}

// FirstLine is the first non-empty line of s (cut to 200 characters).
func FirstLine(s string) string {
	for _, l := range strings.Split(Clean(s), "\n") {
		if t := strings.TrimSpace(l); t != "" {
			return cut(t, 200)
		}
	}
	return ""
}

func cut(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// Launch starts `self __auto exec <id>` on its own (a new session, no
// terminal): for a due schedule, or by hand (manual).
func Launch(self, id string, manual bool) error {
	if !ValidID(id) {
		return errors.New("bad schedule id")
	}
	args := []string{"__auto", "exec", id}
	if manual {
		args = append(args, "--manual")
	}
	cmd := exec.Command(self, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// DueNow are the schedules to start at now: not paused, not running, their
// definition still there, and due (When.Due); recent are the ones started
// lately (a run takes a moment to take its lock).
func DueNow(ss []Schedule, now time.Time, recent map[string]time.Time) []Schedule {
	var out []Schedule
	for _, s := range ss {
		if s.Paused || !ValidID(s.ID) {
			continue
		}
		if t, ok := recent[s.ID]; ok && now.Sub(t) < time.Minute {
			continue
		}
		w := s.Parsed()
		if w.Manual() {
			continue
		}
		if due, _ := w.Due(s.Created, LastStart(s.ID), now); !due {
			continue
		}
		if _, err := os.Stat(s.File); err != nil {
			continue
		}
		if _, running := Running(s.ID); running {
			continue
		}
		out = append(out, s)
	}
	return out
}
