package phone

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRelay follows docs/RELAY-PROTOCOL.md v1 just enough for the
// computer's side to be tested against it (the relay's own repository is
// not imported).
type fakeRelay struct {
	t        *testing.T
	srv      *httptest.Server
	secret   string
	mu       sync.Mutex
	rooms    map[string]*http.ClientConn
	connects int
	agents   []string // User-Agent of each connection
}

func newFakeRelay(t *testing.T, secret string) *fakeRelay {
	t.Helper()
	fr := &fakeRelay{t: t, secret: secret, rooms: map[string]*http.ClientConn{}}
	fr.srv = httptest.NewServer(http.HandlerFunc(fr.serve))
	t.Cleanup(func() {
		fr.mu.Lock()
		for _, cc := range fr.rooms {
			_ = cc.Close()
		}
		fr.mu.Unlock()
		fr.srv.Close()
	})
	return fr
}

func (fr *fakeRelay) serve(w http.ResponseWriter, r *http.Request) {
	fr.mu.Lock()
	authed := r.Header.Get("Authorization") == "Bearer "+fr.secret
	fr.mu.Unlock()
	switch {
	case r.URL.Path == "/v1/check":
		if !authed {
			w.WriteHeader(401)
			return
		}
		w.WriteHeader(204)
	case r.URL.Path == "/v1/connect":
		fr.mu.Lock()
		fr.connects++
		fr.agents = append(fr.agents, r.Header.Get("User-Agent"))
		fr.mu.Unlock()
		if !authed {
			w.WriteHeader(401)
			return
		}
		if r.Header.Get("Upgrade") != RelayVersion {
			w.WriteHeader(426)
			return
		}
		conn, brw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			fr.t.Error(err)
			return
		}
		_, _ = brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: " + RelayVersion + "\r\n\r\n")
		_ = brw.Flush()
		var p http.Protocols
		p.SetUnencryptedHTTP2(true)
		var once sync.Once
		tr := &http.Transport{Protocols: &p, DialContext: func(context.Context, string, string) (net.Conn, error) {
			var c net.Conn
			once.Do(func() { c = &readerConn{Conn: conn, r: brw.Reader} })
			if c == nil {
				return nil, errors.New("gone")
			}
			return c, nil
		}}
		cc, err := tr.NewClientConn(context.Background(), "http", "computer:80")
		if err != nil {
			fr.t.Error(err)
			return
		}
		room := r.URL.Query().Get("room")
		fr.mu.Lock()
		if old := fr.rooms[room]; old != nil {
			_ = old.Close()
		}
		fr.rooms[room] = cc
		fr.mu.Unlock()
	case strings.HasPrefix(r.URL.Path, "/r/"):
		room, rest, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/r/"), "/")
		fr.mu.Lock()
		cc := fr.rooms[room]
		fr.mu.Unlock()
		if cc == nil {
			w.WriteHeader(503)
			return
		}
		out, _ := http.NewRequestWithContext(r.Context(), r.Method, "http://computer/"+rest+"?"+r.URL.RawQuery, r.Body)
		out.Header.Set("Content-Type", r.Header.Get("Content-Type"))
		resp, err := cc.RoundTrip(out)
		if err != nil {
			w.WriteHeader(503)
			return
		}
		defer resp.Body.Close()
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		buf := make([]byte, 32<<10)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				_, _ = w.Write(buf[:n])
				w.(http.Flusher).Flush()
			}
			if err != nil {
				return
			}
		}
	default:
		w.WriteHeader(404)
	}
}

// kick drops a room's computer, as a relay restarting would.
func (fr *fakeRelay) kick(room string) {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	if cc := fr.rooms[room]; cc != nil {
		_ = cc.Close()
		delete(fr.rooms, room)
	}
}

func (fr *fakeRelay) count() int {
	fr.mu.Lock()
	defer fr.mu.Unlock()
	return fr.connects
}

type readerConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *readerConn) Read(b []byte) (int, error) { return c.r.Read(b) }

func fastRelay(t *testing.T) {
	t.Helper()
	poll, lo, hi := relayPoll, relayMinBackoff, relayMaxBackoff
	relayPoll, relayMinBackoff, relayMaxBackoff = 30*time.Millisecond, 20*time.Millisecond, 100*time.Millisecond
	t.Cleanup(func() { relayPoll, relayMinBackoff, relayMaxBackoff = poll, lo, hi })
}

func waitState(t *testing.T, st *Store, want string) RelayState {
	t.Helper()
	for i := 0; i < 300; i++ {
		if s := st.RelayState(); s.State == want {
			return s
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("relay state %+v, want %s", st.RelayState(), want)
	return RelayState{}
}

// TestRelayURL: only HTTPS, with no path; plain HTTP on this computer only.
func TestRelayURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://Relay.Example.com":       "https://relay.example.com",
		"https://relay.example.com/":      "https://relay.example.com",
		" https://relay.example.com:8443": "https://relay.example.com:8443",
		"http://127.0.0.1:8080":           "http://127.0.0.1:8080",
		"http://localhost:8080":           "http://localhost:8080",
	} {
		if got, err := NormalizeRelayURL(in); err != nil || got != want {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
	for _, in := range []string{"http://relay.example.com", "relay.example.com", "https://relay.example.com/x", "https://u:p@relay.example.com", "https://relay.example.com?a=1", "ftp://relay.example.com", "https://"} {
		if _, err := NormalizeRelayURL(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
	u := RelayPairURL(&Relay{URL: "https://r.example.com", Room: "AAAAAAAAAAAAAAAAAAAAAA"}, Pending{Token: "tok", Key: "key"})
	if u != "https://r.example.com/r/AAAAAAAAAAAAAAAAAAAAAA/#p=tok.key" {
		t.Error(u)
	}
}

// TestRelayLink: imux-relay's link carries the secret in its fragment.
func TestRelayLink(t *testing.T) {
	u, sec, err := ParseRelayLink(" https://relay.example.com#abc-_123 ")
	if err != nil || u != "https://relay.example.com" || sec != "abc-_123" {
		t.Fatalf("%q %q %v", u, sec, err)
	}
	for _, in := range []string{"https://relay.example.com", "https://relay.example.com#", "http://relay.example.com#x", "https://relay.example.com/x#y", "https://relay.example.com#a b"} {
		if _, _, err := ParseRelayLink(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}

// TestSetRelay: the relay is kept 0600; the same relay keeps its room (paired
// phones keep their address), another gets a new one.
func TestSetRelay(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	if st.Relay() != nil {
		t.Fatal("a relay with none set")
	}
	r1, err := st.SetRelay("https://relay.example.com/", "s1")
	if err != nil || !validRoom(r1.Room) || r1.URL != "https://relay.example.com" {
		t.Fatalf("%+v %v", r1, err)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(st.file("relay.json")); fi.Mode().Perm() != 0o600 {
			t.Errorf("relay.json mode %v", fi.Mode())
		}
	}
	r2, _ := st.SetRelay("https://relay.example.com", "s2")
	if r2.Room != r1.Room || st.Relay().Secret != "s2" {
		t.Errorf("same relay, new room: %+v", r2)
	}
	r3, _ := st.SetRelay("https://other.example.com", "s2")
	if r3.Room == r1.Room {
		t.Error("another relay kept the room")
	}
	if _, err := st.SetRelay("http://relay.example.com", "s"); err == nil {
		t.Error("plain HTTP accepted")
	}
	if err := st.ClearRelay(); err != nil || st.Relay() != nil {
		t.Errorf("clear: %v", err)
	}
	if err := st.ClearRelay(); err != nil {
		t.Errorf("clear twice: %v", err)
	}
}

// TestCheckRelay: the relay's answer to the secret.
func TestCheckRelay(t *testing.T) {
	fr := newFakeRelay(t, "right")
	ctx := context.Background()
	if err := CheckRelay(ctx, fr.srv.URL, "right"); err != nil {
		t.Errorf("right secret: %v", err)
	}
	if err := CheckRelay(ctx, fr.srv.URL, "wrong"); !errors.Is(err, ErrRelaySecret) {
		t.Errorf("wrong secret: %v", err)
	}
	other := httptest.NewServer(http.NotFoundHandler())
	defer other.Close()
	if err := CheckRelay(ctx, other.URL, "right"); err == nil || errors.Is(err, ErrRelaySecret) {
		t.Errorf("not a relay: %v", err)
	}
}

// TestRunRelay: through the relay a phone pairs, gets its sealed events and
// types into a pane; the computer comes back when the relay drops it, and
// stops dialing when refused until the relay is set up again.
func TestRunRelay(t *testing.T) {
	fastRelay(t)
	fr := newFakeRelay(t, "right")
	st := &Store{Dir: t.TempDir()}
	r, err := st.SetRelay(fr.srv.URL, "right")
	if err != nil {
		t.Fatal(err)
	}
	eng := &fakeEngine{screen: "far away"}
	s := &Server{Store: st, Engine: eng, Every: 10 * time.Millisecond, AgentsEvery: 10 * time.Millisecond}
	h := s.Handler()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { RunRelay(ctx, st, h, "v9.9.9", nil); close(done) }()
	defer func() { cancel(); close(s.Done); <-done }()
	waitState(t, st, "connected")
	fr.mu.Lock()
	ua := fr.agents[0]
	fr.mu.Unlock()
	if ua != "imux/v9.9.9" {
		t.Errorf("User-Agent %q", ua)
	}

	base := fr.srv.URL + "/r/" + r.Room
	resp, err := http.Get(base + "/")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(page), `src="app.js"`) {
		t.Fatalf("the app through the relay: %d %.200s", resp.StatusCode, page)
	}

	pend, _ := st.NewPairing(time.Now())
	resp, err = http.Post(base+"/api/pair", "application/json", strings.NewReader(`{"token":"`+pend.Token+`","name":"far phone"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	ds, _ := st.Devices()
	if resp.StatusCode != 200 || len(ds) != 1 {
		t.Fatalf("pair through the relay: %d %s", resp.StatusCode, b)
	}
	k, _ := ParseKey(pend.Key)
	p := &phone{t: t, srv: &httptest.Server{URL: base}, id: ds[0].ID, key: k}
	sctx, scancel := context.WithCancel(ctx)
	ch, code := p.events(sctx)
	if code != 200 {
		t.Fatalf("events: %d", code)
	}
	next(t, ch, "hello")
	if c := p.cmd(map[string]any{"op": "watch", "pane": "%1"}); c != 204 {
		t.Fatalf("watch: %d", c)
	}
	if sc := next(t, ch, "screen"); !strings.Contains(sc["screen"].(map[string]any)["text"].(string), "far away") {
		t.Errorf("screen %v", sc)
	}
	if c := p.cmd(map[string]any{"op": "send", "pane": "%1", "text": "go on"}); c != 204 {
		t.Errorf("send: %d", c)
	}
	scancel()

	// The relay drops the computer: it dials again.
	n := fr.count()
	fr.kick(r.Room)
	for i := 0; i < 300 && fr.count() == n; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	waitState(t, st, "connected")
	if resp, err := http.Get(base + "/"); err != nil || resp.StatusCode != 200 {
		t.Errorf("after coming back: %v %v", resp, err)
	} else {
		resp.Body.Close()
	}

	// A wrong secret: refused, and no dialing again until it changes.
	fr.mu.Lock()
	fr.secret = "rotated"
	fr.mu.Unlock()
	fr.kick(r.Room)
	if s := waitState(t, st, "refused"); !strings.Contains(s.Error, "secret") {
		t.Errorf("refused: %+v", s)
	}
	n = fr.count()
	time.Sleep(200 * time.Millisecond)
	if fr.count() != n {
		t.Errorf("dialed %d more times after a refusal", fr.count()-n)
	}
	if _, err := st.SetRelay(fr.srv.URL, "rotated"); err != nil {
		t.Fatal(err)
	}
	waitState(t, st, "connected")

	// No relay any more: the connection ends.
	if err := st.ClearRelay(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 300; i++ {
		resp, err := http.Get(base + "/")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 503 {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("still connected after the relay was cleared")
}
