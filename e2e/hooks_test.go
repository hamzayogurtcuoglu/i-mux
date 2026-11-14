//go:build e2e

package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestAgentHooks (M76, D71): Claude starts with i-mux's hooks in its
// settings file; what they say is the pane's state at once: a permission
// prompt is Needs you, the end of a turn is Done; a hook from a pane that
// runs another agent changes nothing; it prints nothing.
func TestAgentHooks(t *testing.T) {
	h := newH(t, sidebarConfig+"[hooks]\nenabled = true\n")
	agent, bar := h.startWithSidebar()
	argv := strings.Join(h.run("claude", 1), "\n")
	var settings string
	for _, a := range strings.Split(argv, "\n") {
		if f, ok := strings.CutPrefix(a, "--settings="); ok {
			settings = f
		}
	}
	if b, _ := os.ReadFile(settings); !strings.Contains(string(b), `__hook claude`) || !strings.Contains(string(b), `"Notification"`) {
		t.Fatalf("claude's settings %q: %s", settings, b)
	}
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)

	hook := func(pane, event string) string {
		t.Helper()
		cmd := exec.Command(h.Bin, "__hook", "claude")
		cmd.Env = append(os.Environ(), "IMUX_PANE="+pane)
		cmd.Stdin = strings.NewReader(event)
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("__hook: %v %s", err, out.String())
		}
		return out.String()
	}
	state := func() string { return h.paneOpt(agent, "#{@imux_state}") }

	if out := hook(agent, `{"hook_event_name":"UserPromptSubmit","session_id":"s1"}`); out != "" {
		t.Fatalf("the hook printed %q", out)
	}
	h.waitFor("working", 5*time.Second, func() bool { return state() == "working" })
	hook(agent, `{"hook_event_name":"Notification","notification_type":"permission_prompt","message":"Claude needs your permission to use Bash"}`)
	h.waitFor("Needs you", 5*time.Second, func() bool { return state() == "blocked" })
	h.waitFor("Needs you in the sidebar", 5*time.Second, func() bool { return strings.Contains(h.screen(bar), "Needs you") })
	hook(agent, `{"hook_event_name":"UserPromptSubmit"}`)
	h.waitFor("working again", 5*time.Second, func() bool { return state() == "working" })
	time.Sleep(MinWorkForTest)
	hook(agent, `{"hook_event_name":"Stop"}`)
	h.waitFor("done", 5*time.Second, func() bool {
		return state() == "idle" && h.paneOpt(agent, "#{@imux_done_at}") != ""
	})

	// A pane without IMUX_PANE's agent: nothing.
	hook(bar, `{"hook_event_name":"Notification","notification_type":"permission_prompt"}`)
	time.Sleep(500 * time.Millisecond)
	if s := h.paneOpt(bar, "#{@imux_hook}"); s != "" {
		t.Errorf("the sidebar's pane took a hook: %q", s)
	}
}

// MinWorkForTest is long enough for the watcher to see the work.
const MinWorkForTest = 300 * time.Millisecond
