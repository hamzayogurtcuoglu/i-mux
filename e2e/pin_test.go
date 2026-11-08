//go:build e2e

package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestPinConversation (T28.2): in History an old conversation (page 2) is
// pinned from its menu; the list opens again with it first, marked 📌; its
// menu then offers Unpin, which puts it back in its place.
func TestPinConversation(t *testing.T) {
	h := newH(t, "")
	now := time.Now()
	for i := 0; i < 12; i++ {
		h.synthetic("claude", i+1, fmt.Sprintf("synthetic task %02d", i), now.Add(-time.Duration(i+2)*time.Hour))
	}
	pane := h.startSession("claude")
	h.run("claude", 1)
	c, _ := h.attach(140, 40)

	h.mustImux("pick", "--pane", pane)
	c.waitScreen(h, "Sessions · ", 5*time.Second)
	c.waitScreen(h, "More…", 5*time.Second)
	mark := c.Mark()
	c.Write("m")
	c.waitScreenAfter(h, mark, "(2/2)", 5*time.Second)
	c.waitScreenAfter(h, mark, "synthetic task 11", 5*time.Second)
	mark = c.Mark()
	// Page 1: run 1's conversation and tasks 00-07; page 2: tasks 08-11.
	c.Write("4")
	c.waitScreenAfter(h, mark, "Pin to the top", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	mark = c.Mark()
	c.Write("t")
	c.waitScreenAfter(h, mark, "📌 Claude", 5*time.Second)
	c.waitScreenAfter(h, mark, "synthetic task 11", 5*time.Second)
	text := stripANSI(c.Output()[mark:])
	if i, j := strings.Index(text, "📌 Claude"), strings.Index(text, "synthetic task 00"); i < 0 || j >= 0 && j < i {
		t.Fatalf("the pinned conversation is not first:\n%s", text)
	}

	// Its menu now offers Unpin; the list opens again without the pin.
	time.Sleep(200 * time.Millisecond)
	mark = c.Mark()
	c.Write("1")
	c.waitScreenAfter(h, mark, "Unpin", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	mark = c.Mark()
	c.Write("t")
	c.waitScreenAfter(h, mark, "Sessions · ", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	if strings.Contains(stripANSI(c.Output()[mark:]), "📌") {
		t.Fatal("still pinned after Unpin")
	}
}
