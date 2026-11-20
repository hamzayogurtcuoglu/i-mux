//go:build e2e

package e2e

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// Client is a terminal client (`tmux attach`, `imux`) running on a pty of a
// fixed size (a pseudo console on Windows). Tests write key bytes and mouse
// sequences to it, exactly as a terminal emulator would.
type Client struct {
	t    *testing.T
	term terminal
	mu   sync.Mutex
	out  bytes.Buffer
	done chan struct{}
}

// terminal is a program on a pty: what it prints is read, keys are written.
type terminal interface {
	io.ReadWriter
	kill()      // ends the program; it does not wait
	closeTerm() // closes the terminal; a blocked Read returns
	wait()      // waits for the program to end
}

// startClient runs argv on a new pty of cols×rows in dir.
func (h *H) startClient(cols, rows int, dir string, argv ...string) *Client {
	h.t.Helper()
	env := append(os.Environ(), "TERM=xterm-256color", "LANG=C.UTF-8", "LC_ALL=C.UTF-8")
	term, err := startTerminal(cols, rows, dir, env, argv)
	if err != nil {
		h.t.Fatalf("start %v: %v", argv, err)
	}
	c := &Client{t: h.t, term: term, done: make(chan struct{})}
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := term.Read(buf)
			if n > 0 {
				c.mu.Lock()
				c.out.Write(buf[:n])
				c.mu.Unlock()
			}
			if err != nil {
				close(c.done)
				return
			}
		}
	}()
	h.t.Cleanup(c.Close)
	return c
}

// Write sends raw bytes (keys, mouse sequences) to the client.
func (c *Client) Write(b string) {
	c.t.Helper()
	if _, err := c.term.Write([]byte(b)); err != nil {
		c.t.Fatalf("pty write: %v", err)
	}
}

// Output returns everything the client printed so far.
func (c *Client) Output() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.out.String()
}

// Close kills the client. The terminal goes first and the wait is bounded: a
// client killed while its terminal still holds output it wrote (macOS
// waits for that to drain as the terminal closes) once hung the whole run
// here until its 15-minute timeout.
func (c *Client) Close() {
	c.term.kill()
	c.term.closeTerm()
	waited := make(chan struct{})
	go func() {
		c.term.wait()
		close(waited)
	}()
	select {
	case <-waited:
	case <-time.After(5 * time.Second):
	}
	select {
	case <-c.done:
	case <-time.After(time.Second):
	}
}

// Click sends an SGR mouse press and release of button (0 left, 2 right) at
// the 1-based column x and row y.
func (c *Client) Click(button, x, y int) {
	c.Write(sgrMouse(button, x, y, 'M'))
	time.Sleep(30 * time.Millisecond)
	c.Write(sgrMouse(button, x, y, 'm'))
}

func sgrMouse(button, x, y int, final byte) string {
	return "\x1b[<" + itoa(button) + ";" + itoa(x) + ";" + itoa(y) + string(final)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// openWith sends keys until want appears on the screen (up to 3 attempts,
// pressing Escape in between): under load the first key press can reach a
// client that is still drawing. It returns the output offset before the
// successful attempt.
func (c *Client) openWith(h *H, keys, want string) int {
	h.t.Helper()
	for attempt := 0; ; attempt++ {
		mark := c.Mark()
		c.Write(keys)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			out := c.Output()
			if mark <= len(out) && strings.Contains(stripANSI(out[mark:]), want) {
				return mark
			}
			time.Sleep(50 * time.Millisecond)
		}
		if attempt == 2 {
			h.t.Fatalf("%q did not show after %q (3 attempts)", want, keys)
		}
		c.Write("\x1b")
		time.Sleep(200 * time.Millisecond)
	}
}
