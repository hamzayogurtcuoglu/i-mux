//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestConversationNames (T39.2): an agent renamed in the sidebar names its
// conversation; History lists it by that name; after the agent is closed,
// continuing the conversation from History brings the name back; a name
// given in History renames the agent running it.
func TestConversationNames(t *testing.T) {
	h := newH(t, sidebarConfig)
	agentPane, _ := h.startWithSidebar()
	h.run("claude", 1)
	c, client := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	var key string
	h.waitFor("claude's conversation", 10*time.Second, func() bool {
		for _, l := range strings.Split(h.mustImux("sessions", "--agent", "claude"), "\n") {
			if f := strings.Fields(l); len(f) >= 2 && f[0] == "claude" {
				key = "claude:" + f[1]
			}
		}
		return key != ""
	})
	names := func() map[string]string {
		var st struct {
			Names map[string]string `json:"names"`
		}
		b, _ := os.ReadFile(filepath.Join(h.Homes.Imux, "state.json"))
		_ = json.Unmarshal(b, &st)
		return st.Names
	}

	// Renamed in the sidebar (the agent menu's Rename…): its conversation.
	h.mustImux("rename", "--pane", agentPane, "backend fix")
	if n := names()[key]; n != "backend fix" {
		t.Fatalf("conversation %s named %q (%v)", key, n, names())
	}

	// History lists it by its name; its menu's Rename… asks for a new one
	// and the agent takes it (the prompt path, keys like a user's).
	mark := c.Mark()
	h.mustImux("__menu", "--sessions", "--pane", agentPane, "--client", client)
	c.waitScreenAfter(h, mark, "backend fix · ", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("1")
	c.waitScreenAfter(h, mark, "Rename…", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("r")
	c.waitScreenAfter(h, mark, "Name for this conversation:", 5*time.Second)
	c.Write("\x15backend fix 2\r") // Ctrl+U clears the current name
	h.waitFor("renamed from History", 5*time.Second, func() bool { return h.paneOpt(agentPane, "#{@imux_name}") == "backend fix 2" })
	c.waitScreenAfter(h, mark, "backend fix 2 · ", 5*time.Second) // History again
	c.Write("\x1b")
	h.mustImux("rename", "--pane", agentPane, "backend fix")

	// Closed, then continued from History: the agent has its name again.
	h.mustImux("__close-pane", "--pane", agentPane)
	h.waitFor("No agents", 5*time.Second, func() bool { return h.paneOpt(agentPane, "#{@imux_role}") == "empty" })
	if n := h.paneOpt(agentPane, "#{@imux_name}"); n != "" {
		t.Fatalf("the No agents pane is named %q", n)
	}
	h.mustImux("split", "claude", "-h", "--from-session", key, "--pane", agentPane)
	h.run("claude", 2)
	h.waitFor("the name back", 5*time.Second, func() bool { return h.paneOpt(agentPane, "#{@imux_name}") == "backend fix" })

	// Renamed in History (Rename…): the agent running it takes the name.
	h.mustImux("__name-session", key, "api", "work")
	h.waitFor("the agent renamed", 5*time.Second, func() bool { return h.paneOpt(agentPane, "#{@imux_name}") == "api work" })
	if n := names()[key]; n != "api work" {
		t.Fatalf("conversation named %q", n)
	}
}
