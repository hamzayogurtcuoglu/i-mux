//go:build e2e

package e2e

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

func (h *H) watchers() []string {
	out, _ := exec.Command("pgrep", "-f", "^"+regexp.QuoteMeta(h.Bin)+" __watch run").Output()
	return strings.Fields(string(out))
}

// TestWatcherLifecycle (T3.3): imux starts one watcher; another ensure wakes
// it instead of starting a second; killing the server ends it within a tick.
func TestWatcherLifecycle(t *testing.T) {
	h := newH(t, "")
	h.startSession("claude")
	h.waitFor("watcher running", 5*time.Second, func() bool { return len(h.watchers()) == 1 })
	status := h.mustImux("__watch", "status")
	pid := h.watchers()[0]
	if !strings.Contains(status, "running (pid "+pid+")") {
		t.Fatalf("status %q, pid %s", status, pid)
	}
	for i := 0; i < 3; i++ {
		h.mustImux("__watch", "ensure")
		h.mustImux("-d")
	}
	time.Sleep(300 * time.Millisecond)
	if w := h.watchers(); len(w) != 1 || w[0] != pid {
		t.Fatalf("watchers after repeated ensure: %v (was %s)", w, pid)
	}
	// The client-attached hook wakes it too.
	h.attach(100, 30)

	h.mustTmux("kill-server")
	h.waitFor("watcher exit after the server is gone", 2*time.Second, func() bool { return len(h.watchers()) == 0 })
	if out := h.mustImux("__watch", "status"); !strings.Contains(out, "not running") {
		t.Fatalf("status after exit: %q", out)
	}
	log := h.readFile(h.Homes.Imux + "/watch.log")
	if !strings.Contains(log, "started") || !strings.Contains(log, "server gone") {
		t.Fatalf("watch.log:\n%s", log)
	}
}

// TestSidebarRevivesWatcher (T3.10, D86): a watcher that dies with the server
// living on comes back on its own. A client that stayed attached never runs
// client-attached again, so a sidebar checks the watcher's lock and starts
// it when it finds it gone.
func TestSidebarRevivesWatcher(t *testing.T) {
	t.Setenv("IMUX_WATCH_CHECK", "300ms")
	h := newH(t, sidebarConfig)
	h.startWithSidebar()
	h.waitFor("watcher running", 5*time.Second, func() bool { return len(h.watchers()) == 1 })
	old := h.watchers()[0]
	if err := exec.Command("kill", "-9", old).Run(); err != nil {
		t.Fatal(err)
	}
	h.waitFor("sidebar revives the watcher", 10*time.Second, func() bool {
		w := h.watchers()
		return len(w) == 1 && w[0] != old
	})
	// The process is visible before it holds the lock and writes its pid, so
	// wait for the status to name it (M91), not only for pgrep to see it.
	var status string
	h.waitFor("status names the new watcher", 10*time.Second, func() bool {
		status = h.mustImux("__watch", "status")
		w := h.watchers()
		return len(w) == 1 && strings.Contains(status, "running (pid "+w[0]+")")
	})
}
