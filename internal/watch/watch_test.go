package watch

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

func TestLogRotation(t *testing.T) {
	testutil.Isolate(t)
	r, err := newRotatingLog()
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Repeat("x", 1023) + "\n"
	for i := 0; i < 1100; i++ {
		if _, err := io.WriteString(r, line); err != nil {
			t.Fatal(err)
		}
	}
	cur, err1 := os.Stat(paths.WatchLog())
	old, err2 := os.Stat(paths.WatchLog() + ".1")
	if err1 != nil || err2 != nil {
		t.Fatalf("log files: %v %v", err1, err2)
	}
	if cur.Size() > LogMax || old.Size() > LogMax || cur.Size() == 0 {
		t.Fatalf("sizes: current %d, rotated %d", cur.Size(), old.Size())
	}
	if cur.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", cur.Mode().Perm())
	}
	// A full log is rotated when it is opened, too.
	r.f.Close()
	big := make([]byte, LogMax)
	_ = os.WriteFile(paths.WatchLog(), big, 0o600)
	f, err := openLog()
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if fi, _ := os.Stat(paths.WatchLog()); fi.Size() != 0 {
		t.Fatal("full log not rotated on open")
	}
}

func TestStatusFollowsTheLock(t *testing.T) {
	testutil.Isolate(t)
	if running, _ := Status(); running {
		t.Fatal("nothing should be running")
	}
	// A stale pid file without a lock holder is not running.
	_ = os.MkdirAll(paths.Home(), 0o700)
	_ = os.WriteFile(paths.WatchPid(), []byte("999999\n"), 0o600)
	if running, _ := Status(); running {
		t.Fatal("stale pid file reported as running")
	}
	f, locked, err := lockPid()
	if err != nil || !locked {
		t.Fatalf("lock: %v %v", locked, err)
	}
	defer f.Close()
	_ = f.Truncate(0)
	_, _ = f.WriteAt([]byte("4242\n"), 0)
	if running, pid := Status(); !running || pid != 4242 {
		t.Fatalf("held lock: %v %d", running, pid)
	}
}

type scriptedTicker struct {
	n        atomic.Int32
	attached bool
	failAt   int32
}

func (s *scriptedTicker) calls() int32 { return s.n.Load() }

func (s *scriptedTicker) Tick(context.Context) (bool, error) {
	if s.n.Add(1) == s.failAt {
		return false, &tmux.Error{Kind: tmux.ErrNoServer}
	}
	return s.attached, nil
}

func TestLoopExitsWhenServerGone(t *testing.T) {
	tk := &scriptedTicker{attached: true, failAt: 3}
	d := &Daemon{Tick: tk, PollInterval: time.Millisecond, IdleInterval: time.Hour, Log: log.New(io.Discard, "", 0)}
	done := make(chan error, 1)
	go func() { done <- d.Loop(context.Background()) }()
	select {
	case err := <-done:
		if err != nil || tk.calls() != 3 {
			t.Fatalf("loop: %v after %d ticks", err, tk.calls())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("loop did not exit")
	}
}

func TestLoopWakesAndStops(t *testing.T) {
	tk := &scriptedTicker{attached: false}
	wake := make(chan os.Signal, 1)
	stop := make(chan os.Signal, 1)
	d := &Daemon{Tick: tk, PollInterval: time.Hour, IdleInterval: time.Hour, Log: log.New(io.Discard, "", 0), Wake: wake, Stop: stop}
	done := make(chan error, 1)
	go func() { done <- d.Loop(context.Background()) }()
	time.Sleep(20 * time.Millisecond)
	wake <- syscall.SIGUSR1 // idle sleep is cut short
	deadline := time.Now().Add(time.Second)
	for tk.calls() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if tk.calls() < 2 {
		t.Fatal("SIGUSR1 did not wake the loop")
	}
	stop <- syscall.SIGTERM
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("SIGTERM did not stop the loop")
	}
	if !errors.Is(&tmux.Error{Kind: tmux.ErrNoServer}, tmux.ErrNoServer) {
		t.Fatal("sanity")
	}
	if !Attached("1\n0\n") || Attached("1\n") || Attached("") {
		t.Fatal("Attached")
	}
}

// TestConfigChanged (T71.6): a config saved with an error does not restart
// the watcher (the restarted one would exit on it); saved again, fixed, it
// does; an unchanged file never does.
func TestConfigChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	var logged bytes.Buffer
	logger := log.New(&logged, "", 0)
	save := func(text string, at time.Time) {
		t.Helper()
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, at, at); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	save("locale = \"en\"\n", now.Add(-time.Hour))
	mod := config.ModTime(path)
	if configChanged(path, &mod, logger) {
		t.Fatal("unchanged config restarts")
	}
	save("locale = \"en\n[broken\n", now.Add(-time.Minute))
	if configChanged(path, &mod, logger) {
		t.Fatal("a broken config restarts the watcher")
	}
	if !strings.Contains(logged.String(), "config:") {
		t.Errorf("not logged: %q", logged.String())
	}
	if configChanged(path, &mod, logger) {
		t.Fatal("the broken config is looked at again and again")
	}
	save("locale = \"tr\"\n", now)
	if !configChanged(path, &mod, logger) {
		t.Fatal("a fixed config does not restart the watcher")
	}
}
