//go:build e2e

package e2e

import (
	"bufio"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestLivePreview (T55.2, T58.1): a click on index.html asks how to open
// it; in its own window serves the project and opens the page in the
// browser (a fake that writes down the URL); the
// page has the live reload script, its event stream hears "reload" when
// the file changes, and a click on another page shows it in that page
// instead of opening a new one.
func TestLivePreview(t *testing.T) {
	opened := filepath.Join(t.TempDir(), "opened")
	h := newH(t, sidebarConfig)
	t.Setenv("IMUX_BROWSE_COMMAND", "echo \"$1\" >> '"+opened+"'")
	index := filepath.Join(h.Homes.Project, "index.html")
	testutil.WriteFile(t, index, []byte("<html><head><title>Index</title></head><body><h1>index</h1></body></html>\n"))
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "about.html"), []byte("<p>about</p>\n"))
	_, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(160, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	t.Cleanup(func() { _, _, _ = h.imux("web", "stop") })

	h.openView(c, bar, "📁", "explorer")
	time.Sleep(500 * time.Millisecond)
	x, y := h.rowOf(bar, "index.html")
	mark := c.Mark()
	c.Click(0, x, y)
	// It asks how to open it (D51): in its own window.
	c.waitScreenAfter(h, mark, "In its own window, live", 5*time.Second)
	c.waitScreenAfter(h, mark, "In a browser tab, live", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("w")
	var u string
	h.waitFor("the browser", 10*time.Second, func() bool {
		b, _ := os.ReadFile(opened)
		u = strings.TrimSpace(string(b))
		return u != ""
	})
	if !strings.HasPrefix(u, "http://localhost:") || !strings.HasSuffix(u, "/index.html") {
		t.Fatalf("opened %q", u)
	}
	c.waitScreenAfter(h, mark, "index.html is open in your browser", 5*time.Second)

	// The page as the browser gets it: with the live reload script.
	resp, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "<h1>index</h1>") || !strings.Contains(string(body), `<script src="/__imux/live.js"></script>`) {
		t.Fatalf("page %q", body)
	}
	// Its event stream, as live.js opens it.
	base := strings.TrimSuffix(u, "/index.html")
	es, err := http.Get(base + "/__imux/events?page=%2Findex.html&title=Index")
	if err != nil {
		t.Fatal(err)
	}
	defer es.Body.Close()
	events := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(es.Body)
		ev := ""
		for sc.Scan() {
			switch l := sc.Text(); {
			case strings.HasPrefix(l, "event: "):
				ev = strings.TrimPrefix(l, "event: ")
			case strings.HasPrefix(l, "data: "):
				events <- ev + " " + strings.TrimPrefix(l, "data: ")
			}
		}
	}()
	next := func(want string) {
		t.Helper()
		select {
		case got := <-events:
			if !strings.HasPrefix(got, want) {
				t.Fatalf("event %q, want %q", got, want)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("no %q event", want)
		}
	}
	next("hello ")

	// An agent rewrites the page: it reloads.
	testutil.WriteFile(t, index, []byte("<body><h1>new</h1></body>\n"))
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(index, later, later); err != nil {
		t.Fatal(err)
	}
	next("reload")

	// Another page: the open page shows it, no new browser.
	time.Sleep(500 * time.Millisecond)
	x, y = h.rowOf(bar, "about.html")
	mark = c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "In its own window, live", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	c.Write("w")
	next("show /about.html")
	c.waitScreenAfter(h, mark, "about.html is in its preview window", 5*time.Second)
	if b, _ := os.ReadFile(opened); strings.Count(string(b), "\n") != 1 {
		t.Errorf("the browser opened again: %q", b)
	}
	if out, _, _ := h.imux("web"); !strings.Contains(out, base+"/ (1 open)") {
		t.Errorf("imux web: %q", out)
	}
}
