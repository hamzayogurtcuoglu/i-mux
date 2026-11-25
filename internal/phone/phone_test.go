package phone

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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

// fakeStream is a stream client: it says "screen of <name> <cols>x<rows>"
// when it starts, and records what the phone sends.
type fakeStream struct {
	mu     sync.Mutex
	input  []string
	sizes  []string
	done   chan struct{}
	once   sync.Once
	closed bool
}

func (f *fakeStream) Input(b []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.input = append(f.input, string(b))
	return nil
}

func (f *fakeStream) Resize(c, r int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sizes = append(f.sizes, fmt.Sprintf("%dx%d", c, r))
	return nil
}

func (f *fakeStream) Close() error {
	f.once.Do(func() { f.mu.Lock(); f.closed = true; f.mu.Unlock(); close(f.done) })
	return nil
}

func (f *fakeStream) Done() <-chan struct{} { return f.done }

type fakeEngine struct {
	mu      sync.Mutex
	streams []*fakeStream
	desktop [2]int
}

func (f *fakeEngine) Attach(_ context.Context, name string, cols, rows int, out io.Writer) (Stream, error) {
	st := &fakeStream{done: make(chan struct{})}
	f.mu.Lock()
	f.streams = append(f.streams, st)
	f.mu.Unlock()
	_, _ = fmt.Fprintf(out, "screen of %s %dx%d", name, cols, rows)
	return st, nil
}

func (f *fakeEngine) DesktopSize(context.Context) (int, int) { return f.desktop[0], f.desktop[1] }

func (f *fakeEngine) stream(i int) *fakeStream {
	f.mu.Lock()
	defer f.mu.Unlock()
	if i < len(f.streams) {
		return f.streams[i]
	}
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
	s := &Server{Store: st, Engine: eng}
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

type event struct {
	name string
	data string
}

// term opens the stream and returns its events, decrypted.
func (p *phone) term(ctx context.Context, cols, rows int) (<-chan event, int) {
	p.t.Helper()
	u := p.srv.URL + "/api/term?d=" + url.QueryEscape(p.id) + "&a=" + url.QueryEscape(p.seal(map[string]any{"op": "term", "cols": cols, "rows": rows}))
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		p.t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		resp.Body.Close()
		return nil, resp.StatusCode
	}
	ch := make(chan event, 64)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		name := ""
		for sc.Scan() {
			line := sc.Text()
			if n, ok := strings.CutPrefix(line, "event: "); ok {
				name = n
				continue
			}
			data, ok := strings.CutPrefix(line, "data: ")
			if !ok {
				continue
			}
			plain, _, err := Open(p.key, p.id, data)
			if err != nil {
				p.t.Errorf("an event not sealed for this phone: %v", err)
				return
			}
			ch <- event{name: name, data: string(plain)}
			name = ""
		}
	}()
	return ch, 200
}

func next(t *testing.T, ch <-chan event, name string) event {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				t.Fatalf("stream ended before %q", name)
			}
			if e.name == name {
				return e
			}
		case <-timeout:
			t.Fatalf("no %q event", name)
		}
	}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestServer: a paired phone gets the workspace's terminal output, sealed,
// at its size; its keys and sizes reach the stream; the server refuses what
// it must.
func TestServer(t *testing.T) {
	eng := &fakeEngine{desktop: [2]int{200, 50}}
	p, st := newPhone(t, eng)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, code := p.term(ctx, 60, 30)
	if code != 200 {
		t.Fatalf("term: %d", code)
	}
	if h := next(t, ch, "hello"); !strings.Contains(h.data, `"read_only":false`) || !strings.Contains(h.data, `"cols":60`) {
		t.Errorf("hello %s", h.data)
	}
	name := ClientName(p.id)
	if out := next(t, ch, ""); out.data != "screen of "+name+" 60x30" {
		t.Errorf("output %q", out.data)
	}
	eventually(t, "the stream", func() bool { return eng.stream(0) != nil })

	if c := p.cmd(map[string]any{"op": "input", "data": base64.StdEncoding.EncodeToString([]byte("\x1b[<0;3;4M"))}); c != 204 {
		t.Fatalf("input: %d", c)
	}
	if c := p.cmd(map[string]any{"op": "resize", "cols": 90, "rows": 9999}); c != 204 {
		t.Fatalf("resize: %d", c)
	}
	s0 := eng.stream(0)
	s0.mu.Lock()
	in, sizes := strings.Join(s0.input, "|"), strings.Join(s0.sizes, "|")
	s0.mu.Unlock()
	if in != "\x1b[<0;3;4M" || sizes != fmt.Sprintf("90x%d", MaxRows) {
		t.Errorf("input %q sizes %q", in, sizes)
	}

	// Refused: a replayed message, another device, bad data, a big paste.
	m := p.seal(map[string]any{"op": "resize", "cols": 80, "rows": 24})
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
	if c := stranger.cmd(map[string]any{"op": "input", "data": "eA=="}); c != 401 {
		t.Errorf("unknown device: %d", c)
	}
	if _, c := stranger.term(ctx, 80, 24); c != 401 {
		t.Errorf("unknown device's stream: %d", c)
	}
	if c := p.cmd(map[string]any{"op": "input", "data": "not base64!"}); c != 400 {
		t.Errorf("bad data: %d", c)
	}
	big := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("x"), MaxInput+1))
	if c := p.cmd(map[string]any{"op": "input", "data": big}); c != 400 {
		t.Errorf("too big: %d", c)
	}

	// A new stream ends the old one: one client a phone.
	ch2, _ := p.term(ctx, 70, 20)
	next(t, ch2, "hello")
	eventually(t, "the second stream", func() bool { return eng.stream(1) != nil })
	select {
	case <-s0.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the first stream was not closed")
	}

	// Read-only: the desktop's size, no input, resizes ignored.
	if err := st.SetReadOnly(p.id, true); err != nil {
		t.Fatal(err)
	}
	ch3, _ := p.term(ctx, 60, 30)
	if h := next(t, ch3, "hello"); !strings.Contains(h.data, `"read_only":true`) || !strings.Contains(h.data, `"cols":200`) {
		t.Errorf("read-only hello %s", h.data)
	}
	eventually(t, "the read-only stream", func() bool { return eng.stream(2) != nil })
	if c := p.cmd(map[string]any{"op": "input", "data": "eA=="}); c != 403 {
		t.Errorf("read-only input: %d", c)
	}
	if c := p.cmd(map[string]any{"op": "resize", "cols": 50, "rows": 20}); c != 204 {
		t.Errorf("read-only resize: %d", c)
	}
	if n := len(eng.stream(2).sizes); n != 0 {
		t.Errorf("a read-only phone resized: %v", eng.stream(2).sizes)
	}

	// Forgotten: nothing any more.
	if err := st.Forget(p.id); err != nil {
		t.Fatal(err)
	}
	if c := p.cmd(map[string]any{"op": "resize", "cols": 80, "rows": 24}); c != 401 {
		t.Errorf("forgotten device: %d", c)
	}
}

// TestStatic: the app's files, xterm.js compressed, the font when there is
// one, with headers that keep the app to itself.
func TestStatic(t *testing.T) {
	s := &Server{Store: &Store{Dir: t.TempDir()}, Engine: &fakeEngine{}}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	for _, f := range []string{"/", "/app.js", "/app.css", "/noble-aes.js", "/xterm.mjs", "/xterm.css", "/manifest.webmanifest", "/icon.svg", "/LICENSE-xterm.txt"} {
		resp, err := client.Get(srv.URL + f)
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
		if f == "/xterm.mjs" && (resp.Header.Get("Content-Encoding") != "gzip" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/javascript")) {
			t.Errorf("xterm.mjs: %v", resp.Header)
		}
	}
	if resp, _ := http.Get(srv.URL + "/../phone.go"); resp.StatusCode != 404 {
		t.Errorf("outside the app: %d", resp.StatusCode)
	}
	if resp, _ := http.Get(srv.URL + "/font.ttf"); resp.StatusCode != 404 {
		t.Errorf("no font: %d", resp.StatusCode)
	}
	font := filepath.Join(t.TempDir(), "f.ttf")
	if err := os.WriteFile(font, []byte("ttf"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := FontFile
	FontFile = font
	defer func() { FontFile = old }()
	resp, _ := http.Get(srv.URL + "/font.ttf")
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(b) != "ttf" || resp.Header.Get("Content-Type") != "font/ttf" {
		t.Errorf("font: %d %q", resp.StatusCode, b)
	}
}

// TestPairURL: the token and key go in the fragment.
func TestPairURL(t *testing.T) {
	u := PairURL("100.101.1.2", 7655, Pending{Token: "tok", Key: "key"})
	if u != "http://100.101.1.2:7655/#p=tok.key" {
		t.Fatal(u)
	}
}
