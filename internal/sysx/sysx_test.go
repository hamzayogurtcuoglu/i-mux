package sysx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// A lock held through one open keeps another from taking it, but not from
// reading the file: watch.pid holds the watcher's pid under its lock.
func TestLockKeepsOthersOutButNotFromReading(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.lock")
	if err := os.WriteFile(p, []byte("1234"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := os.OpenFile(p, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := os.OpenFile(p, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	if ok, err := Lock(a, false); !ok || err != nil {
		t.Fatalf("first lock: %v %v", ok, err)
	}
	if ok, err := Lock(b, false); ok || err != nil {
		t.Fatalf("second lock while held: %v %v, want false and no error", ok, err)
	}
	buf := make([]byte, 4)
	if n, err := b.ReadAt(buf, 0); n != 4 || string(buf) != "1234" {
		t.Fatalf("read while locked: %q %v", buf[:n], err)
	}
	if err := Unlock(a); err != nil {
		t.Fatal(err)
	}
	if ok, err := Lock(b, false); !ok || err != nil {
		t.Fatalf("lock after unlock: %v %v", ok, err)
	}
	_ = Unlock(b)
}

// Wake reaches a process that asked for SIGUSR1 (the sidebar's redraw, the
// watcher's wake-up), until it stops asking.
func TestWakeReachesNotify(t *testing.T) {
	c := make(chan os.Signal, 1)
	Notify(c, SIGUSR1)
	defer Stop(c)
	if err := Wake(os.Getpid()); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-c:
		if s != SIGUSR1 {
			t.Fatalf("got %v", s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no SIGUSR1")
	}
}

func TestAlive(t *testing.T) {
	if !Alive(os.Getpid()) {
		t.Fatal("this process is not alive")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(self, "-test.run=^$")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if Alive(cmd.Process.Pid) {
		t.Fatal("an ended process is alive")
	}
}
