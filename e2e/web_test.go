//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// webPanes lists the panes of pane's window that show a web page.
func (h *H) webPanes(pane string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(h.mustTmux("list-panes", "-t", pane, "-F", "#{pane_id}\t#{@imux_web}")), "\n") {
		if f := strings.SplitN(l, "\t", 2); len(f) == 2 && f[1] != "" {
			out = append(out, f[0])
		}
	}
	return out
}

// TestWebPreview (T52.2): with the preview's browser there ([web] browser,
// a fake), a click on an HTML file opens it in a pane beside the agent, in
// true color; a click on another loads it in the same pane, which Agents
// shows with ⊕; "+" → Web page… asks for a page and opens it there too.
func TestWebPreview(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "fake-browser")
	testutil.WriteFile(t, fake, []byte("#!/bin/sh\necho \"BROWSER $* COLORTERM=$COLORTERM\"\nexec sleep 600\n"))
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatal(err)
	}
	h := newH(t, sidebarConfig+"[web]\nbrowser = \""+fake+"\"\n")
	index := filepath.Join(h.Homes.Project, "index.html")
	about := filepath.Join(h.Homes.Project, "about.html")
	testutil.WriteFile(t, index, []byte("<h1>hi</h1>\n"))
	testutil.WriteFile(t, about, []byte("<p>about</p>\n"))
	agent, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(160, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)

	h.openView(c, bar, "📁", "explorer")
	time.Sleep(500 * time.Millisecond)
	x, y := h.rowOf(bar, "index.html")
	c.Click(0, x, y)
	var web string
	h.waitFor("a web pane", 10*time.Second, func() bool {
		ps := h.webPanes(agent)
		if len(ps) == 1 {
			web = ps[0]
		}
		return web != ""
	})
	if got := h.paneOpt(web, "#{@imux_web}|#{@imux_name}|#{pane_active}"); got != "file://"+index+"|index.html|1" {
		t.Errorf("web pane: %s", got)
	}
	wl, _ := strconv.Atoi(h.paneOpt(web, "#{pane_left}"))
	al, _ := strconv.Atoi(h.paneOpt(agent, "#{pane_left}"))
	if wl <= al {
		t.Errorf("the page (column %d) is not right of the agent (%d)", wl, al)
	}
	h.waitFor("the browser on index.html", 10*time.Second, func() bool {
		return strings.Contains(h.capture(web), "BROWSER file://"+index+" COLORTERM=truecolor")
	})

	time.Sleep(500 * time.Millisecond)
	x, y = h.rowOf(bar, "about.html")
	c.Click(0, x, y)
	h.waitFor("about.html in the same pane", 10*time.Second, func() bool {
		return strings.Contains(h.capture(web), "BROWSER file://"+about)
	})
	if ps := h.webPanes(agent); len(ps) != 1 || ps[0] != web {
		t.Fatalf("web panes %q, want %s", ps, web)
	}

	h.openView(c, bar, "🤖", "agents")
	h.rowOf(bar, "⊕  about.html")

	x, y = h.plusOf(bar, "AGENTS")
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Web page…", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("w")
	c.waitScreenAfter(h, mark, "http://localhost:3000", 5*time.Second)
	c.Write("\x15" + "8123\r") // Ctrl+U clears the page it starts with
	h.waitFor("the typed page", 10*time.Second, func() bool {
		return strings.Contains(h.capture(web), "BROWSER http://localhost:8123")
	})
	if got := h.paneOpt(web, "#{@imux_name}"); got != "localhost:8123" {
		t.Errorf("name %q", got)
	}
	// Ctrl+C ends the browser, and its pane closes.
	h.mustTmux("send-keys", "-t", web, "C-c")
	h.waitFor("the web pane to close", 10*time.Second, func() bool { return len(h.webPanes(agent)) == 0 })
}

// TestWebPreviewNotInstalled (T52.2): without the browser, a click on an
// HTML file opens the popup that offers to install it; e edits the file.
func TestWebPreviewNotInstalled(t *testing.T) {
	ed := filepath.Join(t.TempDir(), "fake-editor")
	testutil.WriteFile(t, ed, []byte("#!/bin/sh\necho \"EDIT $*\"\nread x\n"))
	if err := os.Chmod(ed, 0o755); err != nil {
		t.Fatal(err)
	}
	h := newH(t, "editor = \""+ed+"\"\n"+sidebarConfig)
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "index.html"), []byte("<h1>hi</h1>\n"))
	agent, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(160, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	h.openView(c, bar, "📁", "explorer")
	time.Sleep(500 * time.Millisecond)
	x, y := h.rowOf(bar, "index.html")
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Web preview is not installed", 5*time.Second)
	c.waitScreenAfter(h, mark, "[ Edit ]", 5*time.Second)
	mark = c.Mark()
	c.Write("e")
	c.waitScreenAfter(h, mark, "EDIT ", 5*time.Second)
	c.waitScreenAfter(h, mark, "index.html", 5*time.Second)
	c.Write("\r")
	if ps := h.webPanes(agent); len(ps) != 0 {
		t.Errorf("a web pane without a browser: %q", ps)
	}
}
