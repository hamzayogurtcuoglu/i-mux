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
