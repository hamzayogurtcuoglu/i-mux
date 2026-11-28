package phone

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// The relay (D115, docs/RELAY-PROTOCOL.md): the computer dials a relay the
// user runs and serves the phone's handler over that connection, so the
// phone reaches it from anywhere.

// RelayVersion is the protocol's Upgrade token.
const RelayVersion = "imux-relay.v1"

// Relay is the relay this computer uses: its address, its secret, and this
// computer's room on it.
type Relay struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
	Room   string `json:"room"`
}

// RelayState is what the server last knew of the relay, for the Phone popup
// and `imux phone relay`.
type RelayState struct {
	State string `json:"state"`           // connected, connecting, refused
	Error string `json:"error,omitempty"` // the last error, when not connected
	Since int64  `json:"since"`           // unix seconds
}

// Relay is the relay set up, or nil.
func (s *Store) Relay() *Relay {
	var r Relay
	if err := s.read("relay.json", &r); err != nil || r.URL == "" {
		return nil
	}
	return &r
}

// SetRelay saves the relay; the running server takes it within seconds.
// A room is made unless the relay is the one already set (its paired
// phones keep their address).
func (s *Store) SetRelay(rawURL, secret string) (*Relay, error) {
	u, err := NormalizeRelayURL(rawURL)
	if err != nil {
		return nil, err
	}
	r := &Relay{URL: u, Secret: secret, Room: randomID(16)}
	if old := s.Relay(); old != nil && old.URL == u && validRoom(old.Room) {
		r.Room = old.Room
	}
	return r, s.write("relay.json", r)
}

// ClearRelay stops using a relay.
func (s *Store) ClearRelay() error {
	_ = os.Remove(s.file("relay-state.json"))
	if err := os.Remove(s.file("relay.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// RelayState is the server's last word on the relay.
func (s *Store) RelayState() RelayState {
	var st RelayState
	_ = s.read("relay-state.json", &st)
	return st
}

// validRoom reports whether r is a room as the protocol has it: 16 bytes in
// URL-safe base64 without padding.
func validRoom(r string) bool {
	b, err := b64.DecodeString(r)
	return err == nil && len(r) == 22 && len(b) == 16
}

// ErrRelayURL means the relay's address is not one i-mux uses.
var ErrRelayURL = errors.New("phone: the relay's address must be https://host[:port], with no path")

// NormalizeRelayURL checks a relay's address: HTTPS (plain HTTP only on
// this computer, for tests and a relay beside it), a host, no path, query
// or user.
func NormalizeRelayURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", ErrRelayURL
	}
	switch u.Scheme {
	case "https":
	case "http":
		if ip := net.ParseIP(u.Hostname()); u.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return "", ErrRelayURL
		}
	default:
		return "", ErrRelayURL
	}
	return u.Scheme + "://" + strings.ToLower(u.Host), nil
}

// RelayPairURL is the QR code's address through the relay.
func RelayPairURL(r *Relay, p Pending) string {
	return r.URL + "/r/" + r.Room + "/#p=" + p.Token + "." + p.Key
}

// ErrRelaySecret means the relay refused the secret.
var ErrRelaySecret = errors.New("phone: the relay refused the secret")

// relayClient is the HTTP client that reaches a relay: HTTP/1.1 only (the
// relay takes the connection over), through the environment's proxy.
func relayClient() *http.Client {
	var p http.Protocols
	p.SetHTTP1(true)
	return &http.Client{Transport: &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		Protocols:           &p,
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}},
		TLSHandshakeTimeout: 15 * time.Second,
		DisableCompression:  true,
	}}
}

// CheckRelay asks the relay whether it takes the secret.
func CheckRelay(ctx context.Context, rawURL, secret string) error {
	u, err := NormalizeRelayURL(rawURL)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u+"/v1/check", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := relayClient().Do(req)
	if err != nil {
		return fmt.Errorf("phone: cannot reach the relay: %w", err)
	}
	_ = resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusUnauthorized:
		return ErrRelaySecret
	}
	return fmt.Errorf("phone: %s does not answer as an i-mux relay (%s)", u, resp.Status)
}

// errRefused is a relay's answer that dialing again will not change.
type errRefused struct{ msg string }

func (e errRefused) Error() string { return e.msg }

// dialRelay makes the computer's connection to the relay: the upgraded
// connection, over which the relay speaks HTTP/2 to this computer.
func dialRelay(ctx context.Context, c *http.Client, r *Relay, version string) (io.ReadWriteCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL+"/v1/connect?room="+url.QueryEscape(r.Room), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.Secret)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", RelayVersion)
	req.Header.Set("User-Agent", "imux/"+version)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusSwitchingProtocols {
		if rwc, ok := resp.Body.(io.ReadWriteCloser); ok {
			return rwc, nil
		}
		_ = resp.Body.Close()
		return nil, errors.New("phone: the relay's connection cannot be written to")
	}
	_ = resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return nil, errRefused{ErrRelaySecret.Error()}
	case http.StatusUpgradeRequired, http.StatusBadRequest:
		return nil, errRefused{fmt.Sprintf("phone: the relay does not speak %s (%s)", RelayVersion, resp.Status)}
	}
	return nil, fmt.Errorf("phone: the relay answered %s", resp.Status)
}

// RunRelay keeps this computer connected to the relay that the store names,
// serving h on it, until ctx ends. It reads the store every few seconds, so
// `imux phone relay` takes effect without a restart; version goes in the
// User-Agent.
func RunRelay(ctx context.Context, st *Store, h http.Handler, version string, now func() time.Time) {
	if now == nil {
		now = time.Now
	}
	say := func(state string, err error) {
		rs := RelayState{State: state, Since: now().Unix()}
		if err != nil {
			rs.Error = err.Error()
		}
		if cur := st.RelayState(); cur.State == rs.State && cur.Error == rs.Error {
			return
		}
		_ = st.write("relay-state.json", rs)
	}
	c := relayClient()
	backoff := relayMinBackoff
	var refused *Relay // the relay as it was when it refused
	for ctx.Err() == nil {
		r := st.Relay()
		if r == nil || (refused != nil && *refused == *r) {
			if !sleepCtx(ctx, relayPoll) {
				return
			}
			continue
		}
		refused = nil
		say("connecting", nil)
		rwc, err := dialRelay(ctx, c, r, version)
		if err != nil {
			var ref errRefused
			if errors.As(err, &ref) {
				refused = r
				say("refused", err)
				continue
			}
			say("connecting", err)
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = min(2*backoff, relayMaxBackoff)
			continue
		}
		say("connected", nil)
		began := now()
		err = serveRelay(ctx, st, r, rwc, h)
		if now().Sub(began) >= time.Minute {
			backoff = relayMinBackoff
		}
		if ctx.Err() != nil {
			return
		}
		say("connecting", err)
		if !sleepCtx(ctx, backoff) {
			return
		}
		backoff = min(2*backoff, relayMaxBackoff)
	}
}

// How often the store is read for a new relay, and the shortest and
// longest waits between dials (tests shorten them).
var (
	relayPoll       = 5 * time.Second
	relayMinBackoff = time.Second
	relayMaxBackoff = 60 * time.Second
)

func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// serveRelay serves h in HTTP/2 over the relay's connection until it ends,
// ctx ends, or the store names another relay.
func serveRelay(ctx context.Context, st *Store, r *Relay, rwc io.ReadWriteCloser, h http.Handler) error {
	conn := &relayConn{rwc: rwc, done: make(chan struct{})}
	var p http.Protocols
	p.SetUnencryptedHTTP2(true)
	srv := &http.Server{Handler: h, Protocols: &p, ReadHeaderTimeout: 10 * time.Second}
	ln := &oneConnListener{conn: conn, closed: make(chan struct{})}
	go func() { _ = srv.Serve(ln) }()
	tick := time.NewTicker(relayPoll)
	defer tick.Stop()
	var err error
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-conn.done:
			err = conn.err()
			break loop
		case <-tick.C:
			if cur := st.Relay(); cur == nil || *cur != *r {
				break loop
			}
		}
	}
	_ = srv.Close()
	_ = conn.Close()
	if err == nil || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return errors.New("phone: the relay closed the connection")
	}
	return err
}

// relayConn makes the upgraded connection a net.Conn for http.Server.
type relayConn struct {
	rwc  io.ReadWriteCloser
	once sync.Once
	mu   sync.Mutex
	e    error
	done chan struct{}
}

func (c *relayConn) end(err error) {
	c.once.Do(func() {
		c.mu.Lock()
		c.e = err
		c.mu.Unlock()
		close(c.done)
	})
}

func (c *relayConn) err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.e
}

func (c *relayConn) Read(b []byte) (int, error) {
	n, err := c.rwc.Read(b)
	if err != nil {
		c.end(err)
	}
	return n, err
}

func (c *relayConn) Write(b []byte) (int, error) {
	n, err := c.rwc.Write(b)
	if err != nil {
		c.end(err)
	}
	return n, err
}

func (c *relayConn) Close() error {
	err := c.rwc.Close()
	c.end(net.ErrClosed)
	return err
}

type relayAddr struct{}

func (relayAddr) Network() string { return "relay" }
func (relayAddr) String() string  { return "relay" }

func (c *relayConn) LocalAddr() net.Addr              { return relayAddr{} }
func (c *relayConn) RemoteAddr() net.Addr             { return relayAddr{} }
func (c *relayConn) SetDeadline(time.Time) error      { return nil }
func (c *relayConn) SetReadDeadline(time.Time) error  { return nil }
func (c *relayConn) SetWriteDeadline(time.Time) error { return nil }

// oneConnListener hands http.Server the relay's connection, once.
type oneConnListener struct {
	conn   net.Conn
	once   sync.Once
	closed chan struct{}
	cl     sync.Once
}

func (l *oneConnListener) Accept() (net.Conn, error) {
	var c net.Conn
	l.once.Do(func() { c = l.conn })
	if c != nil {
		return c, nil
	}
	<-l.closed
	return nil, net.ErrClosed
}

func (l *oneConnListener) Close() error {
	l.cl.Do(func() { close(l.closed) })
	return nil
}

func (l *oneConnListener) Addr() net.Addr { return relayAddr{} }
