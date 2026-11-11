// Package watch is the limit watcher daemon (§10): a singleton process that
// polls agent panes for usage-limit messages while a client is attached,
// sleeps while none is, and exits when imux's tmux server is gone.
package watch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/selfexec"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// LogMax is the size at which watch.log is rotated (one .1 file is kept).
const LogMax = 1 << 20

// lockPid opens watch.pid and tries to take its lock without blocking.
func lockPid() (*os.File, bool, error) {
	if err := paths.EnsureDir(paths.Home()); err != nil {
		return nil, false, err
	}
	f, err := os.OpenFile(paths.WatchPid(), os.O_RDWR|os.O_CREATE, paths.FileMode)
	if err != nil {
		return nil, false, err
	}
	ok, err := sysx.Lock(f, false)
	if err != nil {
		f.Close()
		return nil, false, err
	}
	return f, ok, nil
}

func readPid(f *os.File) int {
	b := make([]byte, 32)
	n, _ := f.ReadAt(b, pidOffset)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(b[:n])))
	return pid
}

// Status reports whether the watcher runs, and its pid. The flock is the
// truth: a stale pid file of a dead watcher is not running.
func Status() (bool, int) {
	f, locked, err := lockPid()
	if err != nil {
		return false, 0
	}
	defer f.Close()
	if locked {
		return false, 0 // closing releases the lock
	}
	return true, readPid(f)
}

// Ensure starts the watcher, or wakes it with SIGUSR1 when it runs.
func Ensure(self string) error {
	if running, pid := Status(); running {
		if pid > 0 {
			_ = sysx.Wake(pid)
		}
		return nil
	}
	return start(self)
}

// start launches `imux __watch run` detached (new session, stdio to
// /dev/null, stderr to watch.log).
func start(self string) error {
	logf, err := openLog()
	if err != nil {
		return err
	}
	defer logf.Close()
	return sysx.StartDetached(func() *exec.Cmd {
		cmd := exec.Command(self, "__watch", "run")
		cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, logf
		return cmd
	})
}

// Stop sends SIGTERM to a running watcher and waits up to 2 s for it.
func Stop() bool {
	running, pid := Status()
	if !running || pid <= 0 {
		return false
	}
	_ = sysx.Signal(pid, sysx.SIGTERM)
	for i := 0; i < 40; i++ {
		if r, _ := Status(); !r {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// openLog opens watch.log for appending, rotating it first when it is full.
func openLog() (*os.File, error) {
	if err := paths.EnsureDir(paths.Home()); err != nil {
		return nil, err
	}
	p := paths.WatchLog()
	if fi, err := os.Stat(p); err == nil && fi.Size() >= LogMax {
		_ = os.Rename(p, p+".1")
	}
	return os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, paths.FileMode)
}

// rotatingLog is watch.log, rotated at LogMax. It holds lifecycle events
// and errors only, never screen text.
type rotatingLog struct {
	mu   sync.Mutex
	f    *os.File
	size int64
}

func newRotatingLog() (*rotatingLog, error) {
	f, err := openLog()
	if err != nil {
		return nil, err
	}
	fi, _ := f.Stat()
	r := &rotatingLog{f: f}
	if fi != nil {
		r.size = fi.Size()
	}
	return r, nil
}

func (r *rotatingLog) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size+int64(len(b)) > LogMax {
		r.f.Close()
		_ = os.Rename(paths.WatchLog(), paths.WatchLog()+".1")
		f, err := os.OpenFile(paths.WatchLog(), os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_TRUNC, paths.FileMode)
		if err != nil {
			return 0, err
		}
		r.f, r.size = f, 0
	}
	n, err := r.f.Write(b)
	r.size += int64(n)
	return n, err
}

// Ticker is one poll of the server. attached reports whether a (non
// control-mode) client is attached; err wrapping tmux.ErrNoServer ends the
// daemon.
type Ticker interface {
	Tick(ctx context.Context) (attached bool, err error)
}

// Daemon is a running watcher.
type Daemon struct {
	T            *tmux.Client
	Tick         Ticker
	PollInterval time.Duration
	IdleInterval time.Duration
	Log          *log.Logger
	Wake         <-chan os.Signal // SIGUSR1
	Stop         <-chan os.Signal // SIGTERM / SIGINT / SIGHUP
	// Upgraded reports that the imux binary was replaced; Loop then
	// returns ErrUpgraded.
	Upgraded func() bool
}

// Loop runs until the server is gone or a stop signal arrives.
func (d *Daemon) Loop(ctx context.Context) error {
	for {
		if d.Upgraded != nil && d.Upgraded() {
			return ErrUpgraded
		}
		attached, err := d.Tick.Tick(ctx)
		if errors.Is(err, tmux.ErrNoServer) {
			d.Log.Printf("tmux server gone; exiting")
			return nil
		}
		if err != nil {
			d.Log.Printf("tick: %v", err)
		}
		wait := d.PollInterval
		if !attached {
			wait = d.IdleInterval
		}
		select {
		case <-ctx.Done():
			return nil
		case s := <-d.Stop:
			d.Log.Printf("%v; exiting", s)
			return nil
		case <-d.Wake:
		case <-time.After(wait):
		}
	}
}

// ErrUpgraded ends Loop when the imux binary was replaced.
var ErrUpgraded = errors.New("imux binary replaced")

// Run is `imux __watch run`: take the singleton lock (exit quietly when
// another watcher holds it), write the pid, tune the runtime for a small
// footprint and loop with tick.
func Run(newTicker func(logger *log.Logger) (Ticker, *tmux.Client, time.Duration, time.Duration, error)) error {
	f, locked, err := lockPid()
	if err != nil {
		return err
	}
	if !locked {
		f.Close()
		return nil // another watcher runs
	}
	defer f.Close()
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), pidOffset); err != nil {
		return err
	}
	runtime.GOMAXPROCS(1)
	debug.SetGCPercent(50)
	debug.SetMemoryLimit(12 << 20)

	rl, err := newRotatingLog()
	if err != nil {
		return err
	}
	logger := log.New(io.Writer(rl), fmt.Sprintf("watch[%d] ", os.Getpid()), log.LstdFlags)
	tk, tc, poll, idle, err := newTicker(logger)
	if err != nil {
		logger.Printf("start: %v", err)
		return err
	}
	wake := make(chan os.Signal, 1)
	sysx.Notify(wake, sysx.SIGUSR1)
	stop := make(chan os.Signal, 1)
	sysx.Notify(stop, sysx.SIGTERM, sysx.SIGINT, sysx.SIGHUP)
	logger.Printf("started (poll %v, idle %v)", poll, idle)
	stamp := selfexec.Take()
	cfgMod := config.ModTime(paths.Config())
	changed := func() bool { return stamp.Changed() || !config.ModTime(paths.Config()).Equal(cfgMod) }
	d := &Daemon{T: tc, Tick: tk, PollInterval: poll, IdleInterval: idle, Log: logger, Wake: wake, Stop: stop, Upgraded: changed}
	err = d.Loop(context.Background())
	if errors.Is(err, ErrUpgraded) {
		// A new imux binary: restart with it (the lock goes with this
		// process's file; the new one takes it again).
		logger.Printf("imux binary or config changed; restarting with it")
		f.Close()
		if !sysx.ExecKeepsLocks {
			// Windows cannot replace the process: start the new
			// watcher on its own and end this one.
			return start(stamp.Path)
		}
		return stamp.Exec(os.Args)
	}
	return err
}
