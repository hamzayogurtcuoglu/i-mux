package webview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
)

// liveJS reloads the page when the server says so: "reload" after a file
// it uses changed, "show" to show another page, and "hello" from a server
// that restarted. (Not "open": EventSource fires its own "open" when it
// connects. And an event without data is never dispatched.)
const liveJS = `(() => {
  if (window.__imuxLive) return;
  window.__imuxLive = true;
  let id = null;
  const here = () => location.pathname + location.search;
  const q = "?page=" + encodeURIComponent(here()) + "&title=" + encodeURIComponent(document.title || "");
  const es = new EventSource("/__imux/events" + q);
  es.addEventListener("hello", (e) => { if (id && id !== e.data) location.reload(); id = e.data; });
  es.addEventListener("reload", () => location.reload());
  es.addEventListener("show", (e) => {
    if (!e.data) return;
    if (e.data === here()) location.reload(); else location.replace(e.data);
  });
})();
`

// Server serves Root's files with live reload (§9.3 "Live preview in the
// browser").
type Server struct {
	Name  string        // the folder as the record has it (Status)
	Root  string        // the folder, symbolic links resolved
	Token string        // what the CLI's requests carry (TokenHeader)
	ID    string        // this run's id: a page that sees another reloads
	Poll  time.Duration // how often served files are checked
	Idle  time.Duration // exit after this long with no page and no request

	mu     sync.Mutex
	pages  map[*page]bool
	served map[string]time.Time // files served and folders listed: their modification times
	last   time.Time            // the last request
	done   chan struct{}
}

type page struct {
	path, title string
	at          time.Time
	ch          chan event
}

type event struct{ name, data string }

// maxServed bounds the files watched for changes.
const maxServed = 4000

func (s *Server) init() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pages == nil {
		s.pages, s.served, s.done = map[*page]bool{}, map[string]time.Time{}, make(chan struct{})
		s.last = time.Now()
	}
}

// localHost reports a Host a browser on this machine (or a tunnel that
// forwards to it) sends; a web site's DNS rebinding sends its own name.
func localHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.Trim(host, "[]"))
	switch {
	case host == "localhost", host == "127.0.0.1", host == "::1", strings.HasSuffix(host, ".localhost"):
		return true
	case strings.HasSuffix(host, ".devtunnels.ms"), strings.HasSuffix(host, ".app.github.dev"):
		return true // VS Code's port forwarding
	}
	return false
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.init()
	if !localHost(r.Host) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.mu.Lock()
	s.last = time.Now()
	s.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	switch r.URL.Path {
	case "/__imux/live.js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write([]byte(liveJS))
	case "/__imux/events":
		s.events(w, r)
	case "/__imux/ping", "/__imux/open":
		if r.Header.Get(TokenHeader) != s.Token || s.Token == "" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		st := Status{Root: s.Name, PID: os.Getpid()}
		if st.Root == "" {
			st.Root = s.Root
		}
		if r.URL.Path == "/__imux/open" {
			if r.Method != http.MethodPost {
				http.Error(w, "POST only", http.StatusMethodNotAllowed)
				return
			}
			st.Shown, st.Title = s.show(r.URL.Query().Get("path"))
		}
		st.Pages = s.count()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(st)
	default:
		s.file(w, r)
	}
}

// events is a page's EventSource: hello first, then what the server
// sends it, and a comment now and then so that proxies keep it open.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no streaming", http.StatusInternalServerError)
		return
	}
	q := r.URL.Query()
	p := &page{path: q.Get("page"), title: q.Get("title"), at: time.Now(), ch: make(chan event, 8)}
	s.mu.Lock()
	s.pages[p] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pages, p)
		s.last = time.Now()
		s.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	fmt.Fprintf(w, "retry: 1000\n\nevent: hello\ndata: %s\n\n", s.ID)
	fl.Flush()
	tick := time.NewTicker(25 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.done:
			return
		case ev := <-p.ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.name, ev.data)
			fl.Flush()
		case <-tick.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

func (s *Server) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pages)
}

// show makes the page opened last show path, and returns its title; not
// shown with no page open.
func (s *Server) show(p string) (bool, string) {
	if !strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\r\n") {
		return false, ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var last *page
	for pg := range s.pages {
		if last == nil || pg.at.After(last.at) {
			last = pg
		}
	}
	if last == nil {
		return false, ""
	}
	select {
	case last.ch <- event{"show", p}:
	default:
	}
	return true, last.title
}

func (s *Server) broadcast(ev event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for pg := range s.pages {
		select {
		case pg.ch <- ev:
		default:
		}
	}
}

func (s *Server) watch(p string, mod time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.served) >= maxServed {
		clear(s.served)
	}
	s.served[p] = mod
}

// file serves a file of Root: none whose path has a part starting with "."
// and none outside Root (after links); a folder its index.html, else a
// list; a page with the live reload script.
func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	clean := path.Clean("/" + r.URL.Path)
	for _, part := range strings.Split(clean, "/") {
		if strings.HasPrefix(part, ".") {
			http.NotFound(w, r)
			return
		}
	}
	real, err := filepath.EvalSymlinks(filepath.Join(s.Root, filepath.FromSlash(clean)))
	if err != nil || !within(s.Root, real) {
		http.NotFound(w, r)
		return
	}
	fi, err := os.Stat(real)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if fi.IsDir() {
		if !strings.HasSuffix(r.URL.Path, "/") {
			u := *r.URL
			u.Path += "/"
			http.Redirect(w, r, u.String(), http.StatusFound)
			return
		}
		s.watch(real, fi.ModTime())
		index := filepath.Join(real, "index.html")
		if ifi, err := os.Stat(index); err == nil && ifi.Mode().IsRegular() {
			s.page(w, r, index, ifi)
			return
		}
		s.list(w, real, clean)
		return
	}
	s.watch(real, fi.ModTime())
	if IsHTML(real) {
		s.page(w, r, real, fi)
		return
	}
	f, err := os.Open(real)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
}

// within reports whether p is root or in it.
func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && filepath.IsLocal(rel)
}

// page serves an HTML file with the live reload script; its charset is the
// page's own when it names one, else UTF-8.
func (s *Server) page(w http.ResponseWriter, r *http.Request, file string, fi os.FileInfo) {
	b, err := os.ReadFile(file)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.watch(file, fi.ModTime())
	ct := "text/html"
	if strings.EqualFold(filepath.Ext(file), ".xhtml") {
		ct = "application/xhtml+xml"
	}
	if !strings.Contains(strings.ToLower(string(b[:min(len(b), 1024)])), "charset") {
		ct += "; charset=utf-8"
	}
	w.Header().Set("Content-Type", ct)
	b = inject(b)
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(b)
}

// list is a folder without index.html: its folders, then its files.
func (s *Server) list(w http.ResponseWriter, dir, at string) {
	entries, _ := os.ReadDir(dir)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].IsDir() && !entries[j].IsDir() })
	var b strings.Builder
	title := html.EscapeString(at)
	fmt.Fprintf(&b, `<!doctype html><html><head><meta charset="utf-8"><title>%s</title><style>
body{font:15px/1.6 system-ui,sans-serif;margin:2em auto;max-width:56em;padding:0 1em;color:#ddd;background:#1e1e1e}
a{color:#4fc1ff;text-decoration:none}a:hover{text-decoration:underline}h1{font-size:1.2em;font-weight:600}
ul{list-style:none;padding:0}li{padding:.15em 0}.d{color:#e2c08d}</style></head><body><h1>%s</h1><ul>`, title, title)
	if at != "/" {
		b.WriteString(`<li><a class="d" href="../">..</a></li>`)
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		href := (&url.URL{Path: name}).EscapedPath()
		class := ""
		if e.IsDir() {
			href, name, class = href+"/", name+"/", ` class="d"`
		}
		fmt.Fprintf(&b, `<li><a%s href="%s">%s</a></li>`, class, href, html.EscapeString(name))
	}
	b.WriteString("</ul></body></html>")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(inject([]byte(b.String())))
}

// scan reports whether a watched file changed since it was served (or
// listed), and remembers the new times.
func (s *Server) scan() bool {
	s.mu.Lock()
	files := make(map[string]time.Time, len(s.served))
	for p, t := range s.served {
		files[p] = t
	}
	s.mu.Unlock()
	changed := false
	for p, t := range files {
		var mod time.Time
		if fi, err := os.Stat(p); err == nil {
			mod = fi.ModTime()
		}
		if !mod.Equal(t) {
			changed = true
			s.mu.Lock()
			s.served[p] = mod
			s.mu.Unlock()
		}
	}
	return changed
}

// loop reloads the open pages when a file they use changed (once it stayed
// so for a moment, at most once a second), and stops the server when it
// has been idle.
func (s *Server) loop(ctx context.Context, stop func()) {
	t := time.NewTicker(s.Poll)
	defer t.Stop()
	var reloaded time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		n := s.count()
		if n > 0 && s.scan() {
			time.Sleep(250 * time.Millisecond)
			s.scan()
			if wait := time.Second - time.Since(reloaded); wait > 0 {
				time.Sleep(wait)
			}
			reloaded = time.Now()
			s.broadcast(event{"reload", "changed"})
		}
		s.mu.Lock()
		idle := n == 0 && s.Idle > 0 && time.Since(s.last) > s.Idle
		s.mu.Unlock()
		if idle {
			stop()
			return
		}
	}
}

// Serve runs root's server until it is idle or stopped (SIGTERM): on
// 127.0.0.1, at PortFor(root) or the next free port, recorded for Find.
func Serve(ctx context.Context, root string) error {
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	if fi, err := os.Stat(real); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s is not a folder", root)
	}
	ln, err := listen(PortFor(root))
	if err != nil {
		return err
	}
	s := &Server{Name: root, Root: real, Token: randomHex(16), ID: randomHex(6), Poll: 500 * time.Millisecond, Idle: 30 * time.Minute}
	if v, err := time.ParseDuration(os.Getenv("IMUX_WEB_IDLE")); err == nil {
		s.Idle = v // tests
	}
	s.init()
	info := &Info{Root: root, Port: ln.Addr().(*net.TCPAddr).Port, PID: os.Getpid(), Token: s.Token}
	b, _ := json.Marshal(info)
	rec := recordFile(root)
	if err := paths.WriteFile(rec, b); err != nil {
		_ = ln.Close()
		return err
	}
	defer func() {
		if i := readRecord(rec); i != nil && i.PID == os.Getpid() {
			_ = os.Remove(rec)
		}
	}()
	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer cancel()
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second}
	go s.loop(ctx, cancel)
	go func() {
		<-ctx.Done()
		close(s.done) // ends the pages' event streams
		sctx, c := context.WithTimeout(context.Background(), 2*time.Second)
		defer c()
		_ = srv.Shutdown(sctx)
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// listen takes port on 127.0.0.1, else one of the next 49, else any.
func listen(port int) (net.Listener, error) {
	for p := port; p < port+50 && p < 65536; p++ {
		if ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p)); err == nil {
			return ln, nil
		}
	}
	return net.Listen("tcp", "127.0.0.1:0")
}

func init() {
	// Types the system may not know (a minimal container).
	_ = mime.AddExtensionType(".mjs", "text/javascript")
	_ = mime.AddExtensionType(".svg", "image/svg+xml")
	_ = mime.AddExtensionType(".webp", "image/webp")
}
