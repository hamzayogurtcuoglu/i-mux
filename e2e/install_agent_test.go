//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestInstallMissingAgent (T49.1): a click on an agent that is not installed
// in "Continue this chat in" opens a popup with the command that installs
// it; Install now runs it (here: copying the fake Copilot into place), and
// Enter then continues the chat in it.
func TestInstallMissingAgent(t *testing.T) {
	later := filepath.Join(stagedFakes, "fake-copilot-installed")
	t.Cleanup(func() { _ = os.Remove(later) })
	install := "cp '" + filepath.Join(stagedFakes, "fake-copilot") + "' '" + later + "'"
	h := newH(t, sidebarConfig+fmt.Sprintf("[agents.copilot]\nbin = %q\ninstall = %q\n", later, install))
	agentPane, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	x, y := h.rowOf(bar, "Copilot")
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Copilot is not installed", 5*time.Second)
	c.waitScreenAfter(h, mark, "$ cp ", 2*time.Second)
	if _, err := os.Stat(later); err == nil {
		t.Fatal("installed before Install now")
	}
	c.Write("\r") // Install now
	c.waitScreenAfter(h, mark, "Copilot is installed", 10*time.Second)
	c.Write("\r") // continue the chat in it
	h.run("copilot", 1)
	h.waitFor("the pane runs copilot", 10*time.Second, func() bool { return h.paneOpt(agentPane, "#{@imux_agent}") == "copilot" })
	if strings.Contains(stripANSI(c.Output()[mark:]), "is not installed:") {
		t.Error("the old status-line hint showed too")
	}
}
