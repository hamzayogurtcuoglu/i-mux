package webview

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func TestTarget(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"index.html", "a b.html"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("<p>hi"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for in, want := range map[string][2]string{
		"":                       {"", ""},
		"  https://go.dev/doc  ": {"", "https://go.dev/doc"},
		"localhost:3000":         {"", "http://localhost:3000"},
		"3000":                   {"", "http://localhost:3000"},
		":8080/x":                {"", "http://localhost:8080/x"},
		"3000/api?q=1":           {"", "http://localhost:3000/api?q=1"},
		"127.0.0.1:8000/a":       {"", "http://127.0.0.1:8000/a"},
		"[::1]:3000":             {"", "http://[::1]:3000"},
		"app.localhost:5173":     {"", "http://app.localhost:5173"},
		"go.dev/doc":             {"", "https://go.dev/doc"},
		"about:blank":            {"", "about:blank"},
		"index.html":             {filepath.Join(dir, "index.html"), ""},
		"a b.html":               {filepath.Join(dir, "a b.html"), ""},
		fileURL(dir) + "/a.html": {filepath.Join(dir, "a.html"), ""},
		"missing.html":           {"", "https://missing.html"},
	} {
		if f, u := Target(in, dir); f != want[0] || u != want[1] {
			t.Errorf("Target(%q) = %q %q, want %q", in, f, u, want)
		}
	}
	for u, want := range map[string]string{"http://localhost:3000/": "localhost:3000", "https://go.dev/doc/": "go.dev/doc", "about:blank": "about:blank"} {
		if got := Label(u); got != want {
			t.Errorf("Label(%q) = %q, want %q", u, got, want)
		}
	}
	for name, want := range map[string][2]bool{
		"index.html": {true, true}, "A.HTM": {true, true}, "logo.svg": {false, true}, "README.md": {false, false},
	} {
		if got := [2]bool{IsHTML(name), Previewable(name)}; got != want {
			t.Errorf("%s: %v, want %v", name, got, want)
		}
	}
}

func TestRootAndPort(t *testing.T) {
	proj := t.TempDir()
	sub := filepath.Join(proj, "artifacts", "run 1")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	page := filepath.Join(sub, "index.html")
	testutil.WriteFile(t, page, []byte("x"))
	out := filepath.Join(t.TempDir(), "x.html")
	if RootFor(page, proj) != proj || RootFor(out, proj) != filepath.Dir(out) || RootFor(sub, "") != sub {
		t.Error("roots")
	}
	if p := URLPath(proj, page); p != "/artifacts/run%201/index.html" {
		t.Errorf("path %q", p)
	}
	if URLPath(proj, sub) != "/artifacts/run%201/" || URLPath(proj, proj) != "/" {
		t.Error("folder paths")
	}
	if p := PortFor(proj); p < Base || p >= Base+Span || p != PortFor(proj) {
		t.Errorf("port %d", p)
	}
	if string(inject([]byte("<HTML><BODY>x</BODY></HTML>"))) != `<HTML><BODY>x<script src="/__imux/live.js"></script></BODY></HTML>` {
		t.Error("injected before </BODY>")
	}
}

// site is a folder to serve: pages, data, secrets and a link out.
func site(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range map[string]string{
		"index.html":    "<html><head><title>Run 1</title></head><body><h1>hi</h1></body></html>",
		"sub/page.htm":  "<p>no body",
		"data.json":     `{"a":1}`,
		".env":          "SECRET=1",
		"docs/a b.txt":  "a",
		"docs/.hidden":  "h",
		"latin1.html":   `<meta charset="windows-1254"><p>x`,
		"assets/app.js": "console.log(1)",
	} {
		testutil.WriteFile(t, filepath.Join(root, filepath.FromSlash(name)), []byte(body))
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	testutil.WriteFile(t, outside, []byte("outside"))
	if err := os.Symlink(outside, filepath.Join(root, "out.txt")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symbolic links need Developer Mode on Windows:", err)
		}
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	return real
}

func get(t *testing.T, srv *httptest.Server, path, host string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if host != "" {
		req.Host = host
	}
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// TestServeFiles: pages get the live reload script, nothing is cached, a
// folder lists its files; dot paths, links out, other hosts and calls
// without the token are refused.
func TestServeFiles(t *testing.T) {
	root := site(t)
	s := &Server{Root: root, Token: "tok", ID: "run1"}
	srv := httptest.NewServer(s)
	defer srv.Close()

	resp, body := get(t, srv, "/", "")
	if resp.StatusCode != 200 || !strings.Contains(body, `<h1>hi</h1><script src="/__imux/live.js"></script></body>`) ||
		resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("index: %d %v %q", resp.StatusCode, resp.Header, body)
	}
	if _, body := get(t, srv, "/sub/page.htm", "localhost:9"); !strings.HasSuffix(body, `<p>no body<script src="/__imux/live.js"></script>`) {
		t.Errorf("no </body>: %q", body)
	}
	if resp, _ := get(t, srv, "/latin1.html", ""); resp.Header.Get("Content-Type") != "text/html" {
		t.Errorf("a page's own charset: %q", resp.Header.Get("Content-Type"))
	}
	if resp, body := get(t, srv, "/data.json", ""); body != `{"a":1}` || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Errorf("data: %q %q", resp.Header.Get("Content-Type"), body)
	}
	if resp, _ := get(t, srv, "/docs", ""); resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/docs/" {
		t.Errorf("folder without /: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if _, body := get(t, srv, "/docs/", ""); !strings.Contains(body, `href="a%20b.txt"`) || strings.Contains(body, ".hidden") || !strings.Contains(body, "live.js") {
		t.Errorf("listing %q", body)
	}
	if _, body := get(t, srv, "/__imux/live.js", ""); !strings.Contains(body, "EventSource") {
		t.Errorf("live.js %q", body)
	}
	for _, c := range []struct{ path, host string }{
		{"/.env", ""}, {"/docs/.hidden", ""}, {"/out.txt", ""}, {"/../etc/passwd", ""}, {"/missing", ""},
	} {
		if resp, _ := get(t, srv, c.path, c.host); resp.StatusCode != http.StatusNotFound {
			t.Errorf("%s: %d", c.path, resp.StatusCode)
		}
	}
	for _, host := range []string{"evil.example:80", "localhost.evil.example"} {
		if resp, _ := get(t, srv, "/", host); resp.StatusCode != http.StatusForbidden {
			t.Errorf("host %s: %d", host, resp.StatusCode)
		}
	}
	if resp, _ := get(t, srv, "/__imux/ping", ""); resp.StatusCode != http.StatusForbidden {
		t.Errorf("ping without the token: %d", resp.StatusCode)
	}
}

// TestLiveReload: an open page hears hello, then reload when a file it
// loaded changes; Show sends the page opened last to another page. Every
// event has data (a browser drops one without), and none is "open" (the
// name of EventSource's own event).
func TestLiveReload(t *testing.T) {
	root := site(t)
	s := &Server{Root: root, Token: "tok", ID: "run1", Poll: 50 * time.Millisecond}
	srv := httptest.NewServer(s)
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.loop(ctx, cancel)

	resp, err := http.Get(srv.URL + "/__imux/events?page=%2F&title=Run%201")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	events := make(chan string, 16)
	go func() {
		sc := bufio.NewScanner(resp.Body)
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
			if got != want {
				t.Fatalf("event %q, want %q", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no %q", want)
		}
	}
	next("hello run1")
	get(t, srv, "/", "") // the page loads index.html: it is watched
	later := time.Now().Add(time.Hour)
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<body>new</body>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(root, "index.html"), later, later); err != nil {
		t.Fatal(err)
	}
	next("reload changed")

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/__imux/open?path=%2Fsub%2Fpage.htm", nil)
	req.Header.Set(TokenHeader, "tok")
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var st Status
	_ = json.NewDecoder(r.Body).Decode(&st)
	r.Body.Close()
	if !st.Shown || st.Title != "Run 1" || st.Pages != 1 || st.Root != root {
		t.Fatalf("show: %+v", st)
	}
	next("show /sub/page.htm")
}

// TestServeAndFind: Serve records its port; Find and All find it, Stop
// (here: the context) ends it and removes the record; an idle server stops.
func TestServeAndFind(t *testing.T) {
	testutil.Isolate(t)
	root := site(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, root) }()
	var info *Info
	deadline := time.Now().Add(5 * time.Second)
	for info == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		info = Find(context.Background(), root)
	}
	if info == nil || info.PID != os.Getpid() || len(All(context.Background())) != 1 {
		t.Fatalf("not found: %+v", info)
	}
	if st, err := info.Show(context.Background(), "/"); err != nil || st.Shown {
		t.Errorf("show with no page: %+v %v", st, err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("did not stop")
	}
	if Find(context.Background(), root) != nil {
		t.Error("found after it stopped")
	}
	if _, err := os.Stat(recordFile(root)); !os.IsNotExist(err) {
		t.Errorf("record left: %v", err)
	}

	t.Setenv("IMUX_WEB_IDLE", "100ms")
	go func() { done <- Serve(context.Background(), root) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("an idle server kept running")
	}
}

// The script listens to the events the server sends, and not to "open".
func TestLiveJS(t *testing.T) {
	for _, ev := range []string{`"hello"`, `"reload"`, `"show"`} {
		if !strings.Contains(liveJS, "addEventListener("+ev) {
			t.Errorf("live.js does not listen to %s", ev)
		}
	}
	if strings.Contains(liveJS, `addEventListener("open"`) {
		t.Error(`live.js listens to "open", which EventSource fires when it connects`)
	}
}

// fileURL is dir as a file:// URL (file:///C:/... on Windows).
func fileURL(dir string) string {
	p := filepath.ToSlash(dir)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return "file://" + p
}
