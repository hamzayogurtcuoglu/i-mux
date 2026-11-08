//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// fakeClipboard writes a [paste] command that prints a new image file on
// each call while dir/has-image exists, and nothing (no image) otherwise.
func fakeClipboard(t *testing.T) (dir, cfg string) {
	t.Helper()
	dir = t.TempDir()
	cmd := filepath.Join(dir, "fake-clipboard")
	testutil.WriteFile(t, cmd, []byte(`#!/bin/sh
[ -f '`+dir+`/has-image' ] || exit 0
n=$(cat '`+dir+`/count' 2>/dev/null || echo 0); n=$((n+1)); echo $n >'`+dir+`/count'
f='`+dir+`/shot '$n'.png'; printf 'png' >"$f"; echo "$f"
`))
	if err := os.Chmod(cmd, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, filepath.Join(dir, "has-image"), nil)
	return dir, "[paste]\nimages = true\ncommand = \"" + cmd + "\"\n"
}

// TestPasteImage (T33.3): Ctrl+V and Alt+V in an agent's pane paste the
// clipboard's image as its path, a new file each time, as a bracketed paste;
// with no image the key reaches the agent; Antigravity gets "@path".
func TestPasteImage(t *testing.T) {
	dir, cfg := fakeClipboard(t)
	h := newH(t, cfg)
	// The fake agent asks for bracketed paste and sees Ctrl+V as a character.
	h.control("claude", "stty -iexten; printf '\\033[?2004h'\n")
	h.control("antigravity", "stty -iexten; printf '\\033[?2004h'\n")
	pane := h.startSession("claude")
	h.run("claude", 1)
	c, _ := h.attach(120, 30)
	c.waitScreen(h, "fake claude ready", 10*time.Second)

	c.Write("\x16") // Ctrl+V
	h.waitFor("first path", 10*time.Second, func() bool { return strings.Contains(h.capture(pane), "shot\\ 1.png") })
	c.Write("\x1bv") // Alt+V
	h.waitFor("second path", 10*time.Second, func() bool { return strings.Contains(h.capture(pane), "shot\\ 2.png") })
	c.Write("\r")
	want := "\x1b[200~" + dir + "/shot\\ 1.png \x1b[201~\x1b[200~" + dir + "/shot\\ 2.png \x1b[201~"
	h.waitFor("both images on the agent's input", 5*time.Second, func() bool {
		return strings.TrimRight(h.stdinOf("claude", 1), "\n") == want
	})

	// No image: Ctrl+V is the agent's.
	if err := os.Remove(filepath.Join(dir, "has-image")); err != nil {
		t.Fatal(err)
	}
	c.Write("\x16")
	time.Sleep(time.Second) // i-mux looked at the clipboard and sent the key on
	c.Write("x\r")
	h.waitFor("Ctrl+V passed through", 5*time.Second, func() bool {
		return strings.HasSuffix(h.stdinOf("claude", 1), "\n\x16x\n")
	})

	// Antigravity mentions files with @.
	testutil.WriteFile(t, filepath.Join(dir, "has-image"), nil)
	h.mustImux("switch", "antigravity", "--pane", pane)
	h.run("antigravity", 1)
	c.waitScreen(h, "fake antigravity ready", 10*time.Second)
	time.Sleep(time.Second) // past the handoff's own paste
	before := h.stdinOf("antigravity", 1)
	c.Write("\x16")
	h.waitFor("third path", 10*time.Second, func() bool { return strings.Contains(h.capture(pane), "shot\\ 3.png") })
	c.Write("\r")
	h.waitFor("@path", 10*time.Second, func() bool {
		return strings.Contains(strings.TrimPrefix(h.stdinOf("antigravity", 1), before), "\x1b[200~@"+dir+"/shot\\ 3.png \x1b[201~")
	})
}

// TestPasteImageNoAgent (T33.3): in a pane with no agent Ctrl+V is the
// pane's, even with an image on the clipboard.
func TestPasteImageNoAgent(t *testing.T) {
	_, cfg := fakeClipboard(t)
	h := newH(t, cfg)
	h.startSession("claude")
	h.run("claude", 1)
	shell := strings.TrimSpace(h.mustTmux("split-window", "-d", "-P", "-F", "#{pane_id}", "sh -c 'stty -iexten; printf ready; cat -v'"))
	c, _ := h.attach(120, 30)
	h.mustTmux("select-pane", "-t", shell)
	h.waitFor("shell ready", 5*time.Second, func() bool { return strings.Contains(h.capture(shell), "ready") })
	c.Write("\x16\r")
	h.waitFor("^V in the plain pane", 5*time.Second, func() bool { return strings.Contains(h.capture(shell), "^V") })
	if strings.Contains(h.capture(shell), "shot") {
		t.Fatal("an image pasted into a pane with no agent")
	}
}

// TestPasteImageButton (T33.3): the sidebar's 📎 pastes into the agent.
func TestPasteImageButton(t *testing.T) {
	dir, cfg := fakeClipboard(t)
	h := newH(t, sidebarConfig+cfg)
	agentPane, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	x, y := h.rowOf(bar, "📎")
	c.Click(0, x, y)
	h.waitFor("the path in the agent's pane", 10*time.Second, func() bool {
		return strings.Contains(h.capture(agentPane), dir+"/shot\\ 1.png")
	})
}
