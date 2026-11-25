package phone

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestSeal: sealed for one device opens with its key and id only, and any
// change to the message is refused.
func TestSeal(t *testing.T) {
	k, _ := NewKey()
	other, _ := NewKey()
	s, err := Seal(k, "dev1", []byte(`{"ts":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if plain, _, err := Open(k, "dev1", s); err != nil || string(plain) != `{"ts":1}` {
		t.Fatalf("open: %q %v", plain, err)
	}
	if _, _, err := Open(other, "dev1", s); !errors.Is(err, ErrSeal) {
		t.Error("another key opened it")
	}
	if _, _, err := Open(k, "dev2", s); !errors.Is(err, ErrSeal) {
		t.Error("another device's id opened it")
	}
	b, _ := b64.DecodeString(s)
	b[len(b)-1] ^= 1
	if _, _, err := Open(k, "dev1", b64.EncodeToString(b)); !errors.Is(err, ErrSeal) {
		t.Error("a changed message opened")
	}
	if k2, err := ParseKey(k.String()); err != nil || k2 != k {
		t.Error("key round trip")
	}
	if _, err := ParseKey("short"); err == nil {
		t.Error("a short key parsed")
	}
}

// TestGuard: a message's time must be within a minute, and a nonce is
// taken once.
func TestGuard(t *testing.T) {
	var g Guard
	now := time.Unix(1_800_000_000, 0)
	if err := g.Check("a", now.UnixMilli(), now); err != nil {
		t.Fatal(err)
	}
	if err := g.Check("a", now.UnixMilli(), now); !errors.Is(err, ErrStale) {
		t.Error("a replay was taken")
	}
	if err := g.Check("b", now.Add(-2*time.Minute).UnixMilli(), now); !errors.Is(err, ErrStale) {
		t.Error("an old message was taken")
	}
	if err := g.Check("c", now.Add(2*time.Minute).UnixMilli(), now); !errors.Is(err, ErrStale) {
		t.Error("a message from the future was taken")
	}
}

// TestPairing: a pairing is used once and only in time; devices can be
// made read-only and forgotten.
func TestPairing(t *testing.T) {
	st := &Store{Dir: t.TempDir()}
	now := time.Unix(1_800_000_000, 0)
	p, err := st.NewPairing(now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseKey(p.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Pair("wrong", "x", now); !errors.Is(err, ErrPairing) {
		t.Error("a wrong token paired")
	}
	d, err := st.Pair(p.Token, "iPhone", now.Add(time.Minute))
	if err != nil || d.Key != p.Key || d.Name != "iPhone" {
		t.Fatalf("pair: %+v %v", d, err)
	}
	if _, err := st.Pair(p.Token, "again", now); !errors.Is(err, ErrPairing) {
		t.Error("a pairing was used twice")
	}
	late, _ := st.NewPairing(now)
	if _, err := st.Pair(late.Token, "late", now.Add(PairFor+time.Second)); !errors.Is(err, ErrPairing) {
		t.Error("an expired pairing paired")
	}
	if st.Device(d.ID) == nil || st.Device("nope") != nil {
		t.Error("Device")
	}
	if err := st.SetReadOnly(d.ID, true); err != nil || !st.Device(d.ID).ReadOnly {
		t.Errorf("read-only: %v", err)
	}
	if err := st.Forget(d.ID); err != nil || st.Device(d.ID) != nil {
		t.Errorf("forget: %v", err)
	}
	if err := st.Forget(d.ID); !errors.Is(err, ErrNoDevice) {
		t.Error("forgot twice")
	}
}

// fakeEngine is a workspace with one project and two agents.
type fakeEngine struct {
	mu     sync.Mutex
	screen string
	sent   []string
}

func (f *fakeEngine) Projects(context.Context) ([]Project, error) {
	return []Project{{Name: "api", Branch: "main", Agents: []Agent{{Pane: "%1", CLI: "claude", State: "working"}, {Pane: "%2", CLI: "codex", State: "blocked"}}}}, nil
}

func (f *fakeEngine) Screen(_ context.Context, pane string) (Screen, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return Screen{Text: pane + ": " + f.screen, Width: 80, Height: 24}, nil
}

func (f *fakeEngine) Send(_ context.Context, pane, text string, keys []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, pane+"|"+text+"|"+strings.Join(keys, ","))
	return nil
}

// phone is a paired phone talking to a test server.
type phone struct {
	t   *testing.T
	srv *httptest.Server
	id  string
	key Key
}

func newPhone(t *testing.T, eng Engine) (*phone, *Store) {
	t.Helper()
	st := &Store{Dir: t.TempDir()}
	s := &Server{Store: st, Engine: eng, Every: 10 * time.Millisecond, AgentsEvery: 10 * time.Millisecond}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(func() { close(s.Done); srv.Close() })
	p, err := st.NewPairing(time.Now())
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(srv.URL+"/api/pair", "application/json", strings.NewReader(`{"token":"`+p.Token+`","name":"test phone"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct{ ID string }
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&out) != nil {
		t.Fatalf("pair: %s", resp.Status)
	}
	k, _ := ParseKey(p.Key)
	return &phone{t: t, srv: srv, id: out.ID, key: k}, st
}

func (p *phone) seal(v map[string]any) string {
	p.t.Helper()
	v["ts"] = time.Now().UnixMilli()
	b, _ := json.Marshal(v)
	s, err := Seal(p.key, p.id, b)
	if err != nil {
		p.t.Fatal(err)
	}
	return s
}

func (p *phone) cmd(v map[string]any) int {
	p.t.Helper()
	body, _ := json.Marshal(map[string]string{"d": p.id, "m": p.seal(v)})
	resp, err := http.Post(p.srv.URL+"/api/cmd", "application/json", bytes.NewReader(body))
	if err != nil {
		p.t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

// events opens the stream and returns its decrypted messages.
func (p *phone) events(ctx context.Context) (<-chan map[string]any, int) {
	p.t.Helper()
	u := p.srv.URL + "/api/events?d=" + url.QueryEscape(p.id) + "&a=" + url.QueryEscape(p.seal(map[string]any{"op": "events"}))
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		p.t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, resp.StatusCode
	}
	ch := make(chan map[string]any, 64)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			line, ok := strings.CutPrefix(sc.Text(), "data: ")
			if !ok {
				continue
			}
			plain, _, err := Open(p.key, p.id, line)
			if err != nil {
				p.t.Errorf("an event not sealed for this phone: %v", err)
				return
			}
			var m map[string]any
			_ = json.Unmarshal(plain, &m)
			ch <- m
		}
	}()
	return ch, 200
}

func next(t *testing.T, ch <-chan map[string]any, kind string) map[string]any {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case m, ok := <-ch:
			if !ok {
				t.Fatalf("stream ended before %s", kind)
			}
			if m["t"] == kind {
				return m
			}
		case <-timeout:
			t.Fatalf("no %s", kind)
		}
	}
}

// TestServer: a paired phone gets the agents and the screen it watches,
// sealed; it types into an agent's pane; the server refuses what it must.
func TestServer(t *testing.T) {
	eng := &fakeEngine{screen: "hello"}
	p, st := newPhone(t, eng)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, code := p.events(ctx)
	if code != 200 {
		t.Fatalf("events: %d", code)
	}
	if h := next(t, ch, "hello"); h["read_only"] != false || h["name"] != "test phone" {
		t.Errorf("hello %v", h)
	}
	a := next(t, ch, "agents")
	if b, _ := json.Marshal(a["projects"]); !strings.Contains(string(b), `"pane":"%2"`) {
		t.Errorf("agents %s", b)
	}
	if c := p.cmd(map[string]any{"op": "watch", "pane": "%2"}); c != 204 {
		t.Fatalf("watch: %d", c)
	}
	sc := next(t, ch, "screen")
	if b, _ := json.Marshal(sc["screen"]); !strings.Contains(string(b), `"text":"%2: hello"`) {
		t.Errorf("screen %s", b)
	}
	eng.mu.Lock()
	eng.screen = "changed"
	eng.mu.Unlock()
	if b, _ := json.Marshal(next(t, ch, "screen")["screen"]); !strings.Contains(string(b), "changed") {
		t.Errorf("no new screen: %s", b)
	}

	if c := p.cmd(map[string]any{"op": "send", "pane": "%2", "text": "yes please", "keys": []string{"Enter"}}); c != 204 {
		t.Fatalf("send: %d", c)
	}
	if c := p.cmd(map[string]any{"op": "send", "pane": "%2", "keys": []string{"C-c", "1"}}); c != 204 {
		t.Fatalf("keys: %d", c)
	}
	eng.mu.Lock()
	sent := strings.Join(eng.sent, " / ")
	eng.mu.Unlock()
	if sent != "%2|yes please|Enter / %2||C-c,1" {
		t.Errorf("sent %q", sent)
	}

	// Refused: a key not on the list, a pane that is no agent's, a message
	// sealed again (a replay), another device's id.
	if c := p.cmd(map[string]any{"op": "send", "pane": "%2", "keys": []string{"C-d"}}); c != 400 {
		t.Errorf("unknown key: %d", c)
	}
	if c := p.cmd(map[string]any{"op": "send", "pane": "%9", "text": "x"}); c != 404 {
		t.Errorf("not an agent: %d", c)
	}
	m := p.seal(map[string]any{"op": "watch", "pane": "%1"})
	body, _ := json.Marshal(map[string]string{"d": p.id, "m": m})
	for i, want := range []int{204, 401} {
		resp, err := http.Post(p.srv.URL+"/api/cmd", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("send %d of the same message: %d, want %d", i+1, resp.StatusCode, want)
		}
	}
	stranger := &phone{t: t, srv: p.srv, id: "nobody", key: p.key}
	if c := stranger.cmd(map[string]any{"op": "watch", "pane": "%1"}); c != 401 {
		t.Errorf("unknown device: %d", c)
	}
	if _, c := stranger.events(ctx); c != 401 {
		t.Errorf("unknown device's events: %d", c)
	}

	// Read-only: watches, never types.
	if err := st.SetReadOnly(p.id, true); err != nil {
		t.Fatal(err)
	}
	if c := p.cmd(map[string]any{"op": "send", "pane": "%2", "text": "x"}); c != 403 {
		t.Errorf("read-only send: %d", c)
	}
	if c := p.cmd(map[string]any{"op": "watch", "pane": "%1"}); c != 204 {
		t.Errorf("read-only watch: %d", c)
	}
	// Forgotten: nothing any more.
	if err := st.Forget(p.id); err != nil {
		t.Fatal(err)
	}
	if c := p.cmd(map[string]any{"op": "watch", "pane": "%1"}); c != 401 {
		t.Errorf("forgotten device: %d", c)
	}
}

// TestStatic: the app's files, with headers that keep it to itself.
func TestStatic(t *testing.T) {
	s := &Server{Store: &Store{Dir: t.TempDir()}, Engine: &fakeEngine{}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	for _, f := range []string{"/", "/app.js", "/app.css", "/noble-aes.js", "/manifest.webmanifest", "/icon.svg"} {
		resp, err := http.Get(srv.URL + f)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("%s: %d", f, resp.StatusCode)
		}
		if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") || resp.Header.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s: headers %v", f, resp.Header)
		}
	}
	if resp, _ := http.Get(srv.URL + "/../phone.go"); resp.StatusCode != 404 {
		t.Errorf("outside the app: %d", resp.StatusCode)
	}
}

// TestPairURL: the token and key go in the fragment.
func TestPairURL(t *testing.T) {
	u := PairURL("100.101.1.2", 7655, Pending{Token: "tok", Key: "key"})
	if u != "http://100.101.1.2:7655/#p=tok.key" {
		t.Fatal(u)
	}
}
