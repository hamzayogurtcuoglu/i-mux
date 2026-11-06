//go:build e2e

package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// Client is a terminal client (`tmux attach`, `ajan`) running on a pty of a
// fixed size. Tests write key bytes and mouse sequences to it, exactly as a
// terminal emulator would.
type Client struct {
	t      *testing.T
	master *os.File
	cmd    *exec.Cmd
	mu     sync.Mutex
	out    bytes.Buffer
	done   chan struct{}
}

type winsize struct{ Row, Col, X, Y uint16 }

func ioctl(fd, req, arg uintptr) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, arg); e != 0 {
		return e
	}
	return nil
}

// startClient runs argv on a new pty of cols×rows in dir.
func (h *H) startClient(cols, rows int, dir string, argv ...string) *Client {
	h.t.Helper()
	master, slaveName, err := openPty()
	if err != nil {
		h.t.Fatalf("pty: %v", err)
	}
	slave, err := os.OpenFile(slaveName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		h.t.Fatalf("open %s: %v", slaveName, err)
	}
	// Set the size on the slave: macOS rejects TIOCSWINSZ on a master whose
	// slave is not open yet (ENOTTY).
	ws := winsize{Row: uint16(rows), Col: uint16(cols)}
	if err := ioctl(slave.Fd(), syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws))); err != nil {
		h.t.Fatalf("TIOCSWINSZ: %v", err)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "LANG=C.UTF-8", "LC_ALL=C.UTF-8")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		h.t.Fatalf("start %v: %v", argv, err)
	}
	slave.Close()
	c := &Client{t: h.t, master: master, cmd: cmd, done: make(chan struct{})}
	go func() {
		buf := make([]byte, 32<<10)
		for {
			n, err := master.Read(buf)
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
	if _, err := c.master.Write([]byte(b)); err != nil {
		c.t.Fatalf("pty write: %v", err)
	}
}

// Output returns everything the client printed so far.
func (c *Client) Output() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.out.String()
}

// Close kills the client.
func (c *Client) Close() {
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_, _ = c.cmd.Process.Wait()
	}
	c.master.Close()
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
