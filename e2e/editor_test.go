//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestGetMicroDialog (T53.1): with no editor chosen and no micro, the first
// file opened asks whether to get micro; Use nano opens it in nano (a fake
// here) and is remembered: the next file opens without asking.
func TestGetMicroDialog(t *testing.T) {
	h := newH(t, sidebarConfig)
	t.Setenv("EDITOR", "")
	t.Setenv("VISUAL", "")
	if _, err := exec.LookPath("micro"); err == nil {
		t.Skip("micro is installed on this machine")
	}
	fakes := t.TempDir()
	nano := filepath.Join(fakes, "nano")
	testutil.WriteFile(t, nano, []byte("#!/bin/sh\necho \"NANO $*\"\nread x\n"))
	if err := os.Chmod(nano, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakes+":"+os.Getenv("PATH"))
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "notes.txt"), []byte("hi\n"))
	_, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	h.openView(c, bar, "📁", "explorer")
	time.Sleep(500 * time.Millisecond)

	x, y := h.rowOf(bar, "notes.txt")
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Files open in nano", 5*time.Second)
	c.waitScreenAfter(h, mark, "[ Use nano ]", 5*time.Second)
	mark = c.Mark()
	c.Write("n")
	c.waitScreenAfter(h, mark, "NANO notes.txt", 5*time.Second)
	c.Write("\r")
	time.Sleep(500 * time.Millisecond)

	x, y = h.rowOf(bar, "notes.txt")
	mark = c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "NANO notes.txt", 5*time.Second)
	if strings.Contains(stripANSI(c.Output()[mark:]), "Files open in") {
		t.Error("asked again after Use nano")
	}
	c.Write("\r")
}
