package phone

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

// Agent is an agent pane as the phone lists it.
type Agent struct {
	Pane  string `json:"pane"`
	CLI   string `json:"cli"`
	Name  string `json:"name,omitempty"`
	State string `json:"state,omitempty"` // working, idle or blocked (the watcher's)
	Since int64  `json:"since,omitempty"` // unix seconds of the state's change
}

// Project is a project (an imux session) and its agents.
type Project struct {
	Name   string  `json:"name"`
	Branch string  `json:"branch,omitempty"`
	Agents []Agent `json:"agents"`
}

// Screen is a pane's visible screen, with its colors (SGR escapes).
type Screen struct {
	Text   string `json:"text"`
	Width  int    `json:"w"`
	Height int    `json:"h"`
}

// Engine is what the server needs of the workspace; tests replace it.
type Engine interface {
	Projects(ctx context.Context) ([]Project, error)
	// Screen is pane's screen with lines of its scrollback above it (0:
	// what is visible).
	Screen(ctx context.Context, pane string, lines int) (Screen, error)
	// Send types text (literally) and then keys (tmux key names) into pane.
	Send(ctx context.Context, pane, text string, keys []string) error
}

// Keys the phone may send, by the names its buttons use.
var Keys = map[string]string{
	"Enter": "Enter", "Escape": "Escape", "Tab": "Tab", "BSpace": "BSpace", "Space": "Space",
	"Up": "Up", "Down": "Down", "Left": "Left", "Right": "Right", "C-c": "C-c",
	"1": "1", "2": "2", "3": "3", "4": "4", "5": "5", "6": "6", "7": "7", "8": "8", "9": "9",
	"y": "y", "n": "n", "BTab": "BTab",
}

// MaxLines is the most scrollback the phone may ask for (D110).
const MaxLines = 500

// MaxText is the longest text the phone may type at once.
const MaxText = 4000

//go:embed web
var webFS embed.FS

// Server is the phone's side of imux: the app, pairing and the sealed API.
type Server struct {
	Store  *Store
	Engine Engine
	// Look is the desktop's theme, sent in hello (D110); nil for none.
	Look func() Look
	Now  func() time.Time
	// Every is how often the watched screen is looked at; Agents how often
	// the agents are (tests shorten them).
	Every, AgentsEvery time.Duration

	guard Guard
	mu    sync.Mutex
	watch map[string]watch         // device → the pane it watches
	poke  map[string]chan struct{} // device → its streams' wake-up
	Done  chan struct{}            // closed when the server stops
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
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("POST /api/cmd", s.cmd)
	mux.HandleFunc("GET /", s.static)
	return headers(mux)
}

// headers keeps the app to its own files, out of frames and caches, and
// never sends a referrer (the pairing key is in the fragment, which no
// browser sends; this keeps the address out of logs too).
func headers(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hd := w.Header()
		hd.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
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
	if strings.HasPrefix(name, "LICENSE") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
	b, err := fs.ReadFile(webFS, "web/"+name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if ct := mime.TypeByExtension(path.Ext(name)); ct != "" && w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", ct)
	}
	if strings.HasSuffix(name, ".webmanifest") {
		w.Header().Set("Content-Type", "application/manifest+json")
	}
	_, _ = w.Write(b)
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
	TS   int64    `json:"ts"`
	Op   string   `json:"op"`             // events, watch, send
	Pane string   `json:"pane,omitempty"` // watch, send
	Text string   `json:"text,omitempty"` // send: typed first
	Keys []string `json:"keys,omitempty"` // send: then these keys
	// Lines is watch's scrollback above the screen (D110), at most MaxLines.
	Lines int `json:"lines,omitempty"`
}

// watch is what a device watches: a pane, with lines of scrollback.
type watch struct {
	pane  string
	lines int
}

func (s *Server) cmd(w http.ResponseWriter, r *http.Request) {
	var req struct{ D, M string }
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
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
	s.Store.Touch(d.ID, s.now())
	ctx := r.Context()
	if c.Pane != "" && !s.isAgent(ctx, c.Pane) {
		http.Error(w, "no such agent", http.StatusNotFound)
		return
	}
	switch c.Op {
	case "watch":
		s.mu.Lock()
		if s.watch == nil {
			s.watch = map[string]watch{}
		}
		s.watch[d.ID] = watch{pane: c.Pane, lines: min(max(c.Lines, 0), MaxLines)}
		s.mu.Unlock()
		s.wake(d.ID)
	case "send":
		if d.ReadOnly {
			http.Error(w, "this phone only watches", http.StatusForbidden)
			return
		}
		if c.Pane == "" || len(c.Text) > MaxText {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var keys []string
		for _, k := range c.Keys {
			tk, ok := Keys[k]
			if !ok {
				http.Error(w, "unknown key", http.StatusBadRequest)
				return
			}
			keys = append(keys, tk)
		}
		if err := s.Engine.Send(ctx, c.Pane, c.Text, keys); err != nil {
			http.Error(w, "could not send", http.StatusBadGateway)
			return
		}
		s.wake(d.ID)
	default:
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// isAgent reports whether pane is an agent pane the phone may see.
func (s *Server) isAgent(ctx context.Context, pane string) bool {
	ps, err := s.Engine.Projects(ctx)
	if err != nil {
		return false
	}
	for _, p := range ps {
		for _, a := range p.Agents {
			if a.Pane == pane {
				return true
			}
		}
	}
	return false
}

func (s *Server) wake(id string) {
	s.mu.Lock()
	ch := s.poke[id]
	s.mu.Unlock()
	if ch != nil {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// events streams the device's sealed updates (Server-Sent Events): hello,
// the agents when they change, the watched pane's screen when it changes.
// The request proves the key with a sealed {"op":"events"} in a.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	d, k, code := s.device(r.URL.Query().Get("d"))
	if d == nil {
		http.Error(w, "not paired", code)
		return
	}
	var c Cmd
	if err := OpenChecked(k, d.ID, r.URL.Query().Get("a"), &s.guard, s.now(), &c); err != nil || c.Op != "events" {
		http.Error(w, "refused", http.StatusUnauthorized)
		return
	}
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no streaming", http.StatusInternalServerError)
		return
	}
	s.Store.Touch(d.ID, s.now())
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("X-Accel-Buffering", "no")
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	if s.poke == nil {
		s.poke = map[string]chan struct{}{}
	}
	s.poke[d.ID] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.poke[d.ID] == ch {
			delete(s.poke, d.ID)
		}
		s.mu.Unlock()
	}()

	send := func(v any) bool {
		b, err := json.Marshal(v)
		if err != nil {
			return false
		}
		sealed, err := Seal(k, d.ID, b)
		if err != nil {
			return false
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", sealed); err != nil {
			return false
		}
		fl.Flush()
		return true
	}
	hello := map[string]any{"t": "hello", "read_only": d.ReadOnly, "name": d.Name}
	if s.Look != nil {
		hello["theme"] = s.Look()
	}
	if !send(hello) {
		return
	}
	every, agentsEvery := s.Every, s.AgentsEvery
	if every <= 0 {
		every = 250 * time.Millisecond
	}
	if agentsEvery <= 0 {
		agentsEvery = 2 * time.Second
	}
	ctx := r.Context()
	tick := time.NewTicker(every)
	defer tick.Stop()
	var lastAgents, lastScreen [32]byte
	lastPane := ""
	nextAgents, nextBeat := time.Time{}, s.now().Add(15*time.Second)
	for {
		now := s.now()
		if !now.Before(nextAgents) {
			nextAgents = now.Add(agentsEvery)
			if ps, err := s.Engine.Projects(ctx); err == nil {
				b, _ := json.Marshal(ps)
				if sum := sha256.Sum256(b); sum != lastAgents {
					lastAgents = sum
					if !send(map[string]any{"t": "agents", "projects": ps}) {
						return
					}
				}
			}
		}
		s.mu.Lock()
		wt := s.watch[d.ID]
		s.mu.Unlock()
		if pane := wt.pane; pane != "" {
			if sc, err := s.Engine.Screen(ctx, pane, wt.lines); err == nil {
				b, _ := json.Marshal(sc)
				if sum := sha256.Sum256(b); sum != lastScreen || pane != lastPane {
					lastScreen, lastPane = sum, pane
					if !send(map[string]any{"t": "screen", "pane": pane, "screen": sc}) {
						return
					}
				}
			}
		}
		if !now.Before(nextBeat) {
			nextBeat = now.Add(15 * time.Second)
			if _, err := io.WriteString(w, ": beat\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
		select {
		case <-ctx.Done():
			return
		case <-s.Done:
			return
		case <-tick.C:
		case <-ch:
		}
	}
}
