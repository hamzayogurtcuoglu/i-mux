//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// workThenAsk makes the fake agent work 13 s, stop, then ask.
const workThenAsk = "i=0; while [ $i -lt 26 ]; do echo working $i; sleep 0.5; i=$((i+1)); done; sleep 6; echo 'Do you want to proceed? (y/n)'\n"

// TestNotifications (T32.3): an agent that worked and stopped, then asks,
// gives one "finished" and one "needs you" notification, through the
// configured command (every platform).
func TestNotifications(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "notified")
	cmd := filepath.Join(dir, "fake-notifier")
	testutil.WriteFile(t, cmd, []byte("#!/bin/sh\nprintf '%s|%s\\n' \"$1\" \"$2\" >> '"+log+"'\n"))
	if err := os.Chmod(cmd, 0o755); err != nil {
		t.Fatal(err)
	}
	h := newH(t, "[notify]\nenabled = true\nsound = false\ncommand = \""+cmd+"\"\n")
	h.control("claude", workThenAsk)
	h.startSession("claude")
	h.run("claude", 1)
	read := func() string { b, _ := os.ReadFile(log); return string(b) }
	h.waitFor("a finished notification", 40*time.Second, func() bool {
		return strings.Contains(read(), "Claude finished · proj|proj · tab 0")
	})
	h.waitFor("a needs-you notification", 30*time.Second, func() bool {
		return strings.Contains(read(), "Claude needs you · proj|proj · tab 0")
	})
	time.Sleep(2 * time.Second)
	if n := strings.Count(read(), "\n"); n != 2 {
		t.Fatalf("%d notifications:\n%s", n, read())
	}
}

// TestNotificationsInTheTerminal (T32.3): with no desktop (no notify-send,
// no D-Bus: a server, SSH) the notification reaches the terminal as OSC 777.
// Linux only: on macOS it would be a real notification on the screen.
func TestNotificationsInTheTerminal(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the desktop's notifications are used here")
	}
	for _, c := range []string{"notify-send", "gdbus"} {
		if _, err := exec.LookPath(c); err == nil {
			t.Skip("a desktop notification service is installed: " + c)
		}
	}
	h := newH(t, "[notify]\nenabled = true\n")
	h.control("claude", workThenAsk)
	h.startSession("claude")
	h.run("claude", 1)
	c, _ := h.attach(120, 30)
	c.Write("\x1b[O") // the terminal is not focused: the user is elsewhere
	h.waitFor("OSC 777 on the terminal", 40*time.Second, func() bool {
		return strings.Contains(c.Output(), "\x1b]777;notify;Claude finished · proj;proj · tab 0\a")
	})
}
