package phone

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

// Stream is a client of the workspace whose terminal is the phone (D105):
// the engine's output goes to the writer it was started with.
type Stream interface {
	Input(b []byte) error
	Resize(cols, rows int) error
	Close() error
	Done() <-chan struct{}
}

// Engine is what the server needs of the workspace; tests replace it.
type Engine interface {
	// Attach starts a stream client named name, cols × rows, on the
	// session the desktop shows (else the newest); its output goes to out.
	Attach(ctx context.Context, name string, cols, rows int, out io.Writer) (Stream, error)
	// DesktopSize is the size of the desktop's most recently used client,
	// 0, 0 when there is none.
	DesktopSize(ctx context.Context) (cols, rows int)
}

// Size limits for a phone's terminal.
const (
	MinCols, MaxCols = 20, 400
	MinRows, MaxRows = 5, 200
	// MaxInput is the most a phone sends at once (a paste).
	MaxInput = 16 << 10
)

//go:embed web
var webFS embed.FS

// FontFile is the Nerd Font file the phone draws with (D94's bundled copy,
// else the installed one); the server's start sets it. "" serves none.
var FontFile = ""

// Server is the phone's side of imux: the app, pairing and the sealed
// stream of the workspace.
type Server struct {
	Store  *Store
	Engine Engine
	Now    func() time.Time

	guard   Guard
	mu      sync.Mutex
	streams map[string]Stream // device → its stream
	Done    chan struct{}     // closed when the server stops
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Handler routes the app's files and the API.
func (s *Server) Handler() http.Handler {
	if s.Done == nil {
		s.Done = make(chan struct{})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/pair", s.pair)
	mux.HandleFunc("GET /api/term", s.term)
	mux.HandleFunc("POST /api/cmd", s.cmd)
	mux.HandleFunc("GET /font.ttf", s.font)
	mux.HandleFunc("GET /", s.static)
	return headers(mux)
}

// headers keeps the app to its own files, out of frames and caches, and
// never sends a referrer. Styles may be inline: xterm.js sets its own.
func headers(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		hd.Set("X-Content-Type-Options", "nosniff")
		hd.Set("Referrer-Policy", "no-referrer")
		hd.Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	gz := false
	b, err := fs.ReadFile(webFS, "web/"+name)
	if err != nil {
		// A large file is kept compressed and served as it is (every
		// browser the app runs in takes gzip).
		if b, err = fs.ReadFile(webFS, "web/"+name+".gz"); err != nil {
			http.NotFound(w, r)
			return
		}
		gz = true
	}
	switch {
	case strings.HasPrefix(name, "LICENSE"):
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	case strings.HasSuffix(name, ".webmanifest"):
		w.Header().Set("Content-Type", "application/manifest+json")
	case strings.HasSuffix(name, ".mjs"), strings.HasSuffix(name, ".js"):
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	default:
		if ct := mime.TypeByExtension(path.Ext(name)); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
	}
	if gz {
		w.Header().Set("Content-Encoding", "gzip")
	}
	_, _ = w.Write(b)
}

// font is the Nerd Font, so the sidebar's icons draw on the phone as on the
// desktop (D105).
func (s *Server) font(w http.ResponseWriter, r *http.Request) {
	if FontFile == "" {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(FontFile)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "font/ttf")
	w.Header().Set("Cache-Control", "max-age=604800")
	_, _ = io.Copy(w, f)
}

// pair takes the QR code's token: the device it pairs, whose key the phone
// already has from the code (it never crosses the network).
func (s *Server) pair(w http.ResponseWriter, r *http.Request) {
	var req struct{ Token, Name string }
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	d, err := s.Store.Pair(req.Token, req.Name, s.now())
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"id": d.ID})
}

// device is the paired device id and its key, or an error for the reply.
func (s *Server) device(id string) (*Device, Key, int) {
	d := s.Store.Device(id)
	if d == nil {
		return nil, Key{}, http.StatusUnauthorized
	}
	k, err := ParseKey(d.Key)
	if err != nil {
		return nil, Key{}, http.StatusUnauthorized
	}
	return d, k, 0
}

// Cmd is a message from the phone.
type Cmd struct {
	TS   int64  `json:"ts"`
	Op   string `json:"op"`             // term, input, resize
	Data string `json:"data,omitempty"` // input: the bytes, base64
	Cols int    `json:"cols,omitempty"` // term, resize
	Rows int    `json:"rows,omitempty"`
}

func clampSize(cols, rows int) (int, int) {
	return min(max(cols, MinCols), MaxCols), min(max(rows, MinRows), MaxRows)
}

func (s *Server) cmd(w http.ResponseWriter, r *http.Request) {
	var req struct{ D, M string }
	if err := json.NewDecoder(io.LimitReader(r.Body, 4*MaxInput)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	d, k, code := s.device(req.D)
	if d == nil {
		http.Error(w, "not paired", code)
		return
	}
	var c Cmd
	if err := OpenChecked(k, d.ID, req.M, &s.guard, s.now(), &c); err != nil {
		http.Error(w, "refused", http.StatusUnauthorized)
		return
	}
	s.mu.Lock()
	st := s.streams[d.ID]
	s.mu.Unlock()
	if st == nil {
		http.Error(w, "no stream", http.StatusConflict)
		return
	}
	switch c.Op {
	case "input":
		if d.ReadOnly {
			http.Error(w, "this phone only watches", http.StatusForbidden)
			return
		}
		b, err := base64.StdEncoding.DecodeString(c.Data)
		if err != nil || len(b) > MaxInput {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := st.Input(b); err != nil {
			http.Error(w, "the stream ended", http.StatusGone)
			return
		}
	case "resize":
		if d.ReadOnly {
			// A watching phone keeps the desktop's size: it never resizes
			// the windows.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		cols, rows := clampSize(c.Cols, c.Rows)
		if err := st.Resize(cols, rows); err != nil {
			http.Error(w, "the stream ended", http.StatusGone)
			return
		}
	default:
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sse writes sealed Server-Sent Events, one at a time.
type sse struct {
	mu  sync.Mutex
	w   http.ResponseWriter
	fl  http.Flusher
	key Key
	id  string
}

func (e *sse) event(name string, plain []byte) error {
	sealed, err := Seal(e.key, e.id, plain)
	if err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if name != "" {
		if _, err := fmt.Fprintf(e.w, "event: %s\n", name); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(e.w, "data: %s\n\n", sealed); err != nil {
		return err
	}
	e.fl.Flush()
	return nil
}

// Write is the engine's output: one sealed event per write.
func (e *sse) Write(p []byte) (int, error) {
	if err := e.event("", p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (e *sse) beat() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := io.WriteString(e.w, ": beat\n\n"); err != nil {
		return err
	}
	e.fl.Flush()
	return nil
}

// ClientName is the stream client's name for a device.
func ClientName(id string) string {
	if len(id) > 8 {
		id = id[:8]
	}
	return "phone-" + id
}

// term is the phone's view of the workspace (D105): a stream client of the
// phone's size (a watching phone: the desktop's), its output sealed as
// events. A device has one stream; a new one ends the old. The request
// proves the key with a sealed {"op":"term"} in a.
func (s *Server) term(w http.ResponseWriter, r *http.Request) {
	d, k, code := s.device(r.URL.Query().Get("d"))
	if d == nil {
		http.Error(w, "not paired", code)
		return
	}
	var c Cmd
	if err := OpenChecked(k, d.ID, r.URL.Query().Get("a"), &s.guard, s.now(), &c); err != nil || c.Op != "term" {
		http.Error(w, "refused", http.StatusUnauthorized)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no streaming", http.StatusInternalServerError)
		return
	}
	ctx := r.Context()
	cols, rows := clampSize(c.Cols, c.Rows)
	if d.ReadOnly {
		if dc, dr := s.Engine.DesktopSize(ctx); dc > 0 && dr > 0 {
			cols, rows = dc, dr
		}
	}
	s.Store.Touch(d.ID, s.now())
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("X-Accel-Buffering", "no")
	out := &sse{w: w, fl: fl, key: k, id: d.ID}
	hello, _ := json.Marshal(map[string]any{"read_only": d.ReadOnly, "cols": cols, "rows": rows, "name": d.Name})
	if out.event("hello", hello) != nil {
		return
	}
	// The old stream ends before the new one attaches: one client a phone.
	s.mu.Lock()
	old := s.streams[d.ID]
	delete(s.streams, d.ID)
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
		select {
		case <-old.Done():
		case <-time.After(2 * time.Second):
		}
	}
	st, err := s.Engine.Attach(ctx, ClientName(d.ID), cols, rows, out)
	if err != nil {
		msg, _ := json.Marshal(map[string]string{"error": err.Error()})
		_ = out.event("failed", msg)
		return
	}
	s.mu.Lock()
	if s.streams == nil {
		s.streams = map[string]Stream{}
	}
	s.streams[d.ID] = st
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.streams[d.ID] == st {
			delete(s.streams, d.ID)
		}
		s.mu.Unlock()
		_ = st.Close()
	}()
	beat := time.NewTicker(15 * time.Second)
	defer beat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.Done:
			return
		case <-st.Done():
			_ = out.event("end", []byte(`{}`))
			return
		case <-beat.C:
			if out.beat() != nil {
				return
			}
		}
	}
}
