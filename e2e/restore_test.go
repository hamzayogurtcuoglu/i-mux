//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRestoreProject (T16.4): a project with two tabs and a named pane is
// closed; `imux` in its folder brings both tabs back, the name too, and
// each agent resumes its own conversation.
func TestRestoreProject(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, _ := h.startWithSidebar()
	argv := h.run("claude", 1)
	claudeID := ""
	for i, a := range argv {
		if a == "--session-id" && i+1 < len(argv) {
			claudeID = argv[i+1]
		}
	}
	h.mustImux("rename", "backend fix", "--pane", agentPane)
	h.mustImux("tab", "codex", "--pane", agentPane)
	h.run("codex", 1)
	session := strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", agentPane, "#{session_name}"))

	h.mustImux("close", "--session", session)
	h.waitFor("project closed", 5*time.Second, func() bool {
		_, err := h.tmux("has-session", "-t", "="+session)
		return err != nil
	})

	h.mustImux("-d")
	argv = h.run("claude", 2)
	if strings.Join(argv, " ") != "--resume "+claudeID || claudeID == "" {
		t.Fatalf("claude was not resumed: %q (session %s)", argv, claudeID)
	}
	if argv := h.run("codex", 2); len(argv) < 2 || argv[0] != "resume" {
		t.Fatalf("codex was not resumed: %q", argv)
	}
	var out string
	defer func() {
		if t.Failed() {
			t.Logf("panes: %s", out)
		}
	}()
	h.waitFor("two tabs with sidebars", 10*time.Second, func() bool {
		out, _ = h.tmux("list-panes", "-s", "-t", "="+session, "-F", "#{window_index}:#{@imux_role}:#{@imux_agent}:#{@imux_name}")
		return strings.Count(out, "sidebar") == 2 && strings.Contains(out, "0::claude:backend fix") && strings.Contains(out, "1::codex:")
	})
}

// TestRecentInPicker (T16.4): a closed project is listed under RECENT with
// its tabs and agents; choosing it in the picker restores it and the client
// moves there.
func TestRecentInPicker(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, _ := h.startWithSidebar()
	h.run("claude", 1)
	h.mustImux("tab", "codex", "--pane", agentPane)
	h.run("codex", 1)
	first := strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", agentPane, "#{session_name}"))
	other := filepath.Join(h.Homes.Home, "blog")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	h.mustImux("open", "-d", other)
	h.run("claude", 2)
	c, client := h.attach(140, 40)
	c.waitScreen(h, "Agents", 10*time.Second)
	h.mustImux("close", "--session", first)
	h.waitFor("client on blog", 5*time.Second, func() bool { return strings.HasPrefix(h.clientSession(client), "blog-") })

	var bar string
	h.waitFor("blog sidebar", 5*time.Second, func() bool {
		for _, l := range strings.Split(h.mustTmux("list-panes", "-a", "-F", "#{pane_id} #{@imux_role}"), "\n") {
			if f := strings.Fields(l); len(f) == 2 && f[1] == "sidebar" {
				bar = f[0]
			}
		}
		return bar != ""
	})
	x, y := h.plusOf(bar, "Projects")
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "RECENT", 5*time.Second)
	c.waitScreenAfter(h, mark, "2 tabs · 2 agents", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("proj\r")
	h.run("claude", 3)
	h.run("codex", 2)
	h.waitFor("client on the restored project", 5*time.Second, func() bool { return h.clientSession(client) == first })
}
