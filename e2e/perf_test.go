//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session/claude"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestHandoffPrintLargeSession checks §17: `imux handoff --print` on a 64 MB
// Claude session takes < 1 s with peak RSS < 80 MB.
func TestHandoffPrintLargeSession(t *testing.T) {
	bin := imuxBin(t)
	h := testutil.Isolate(t)
	id := "99999999-0000-4000-8000-000000000064"
	p := filepath.Join(h.Claude, "projects", claude.EncodeCWD(h.Project), id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := testutil.GenClaudeSession(f, 64<<20, h.Project, id); err != nil {
		t.Fatal(err)
	}
	f.Close()

	var best time.Duration
	var rss int64
	for i := 0; i < 3; i++ {
		cmd := exec.Command(bin, "handoff", "--print", "--from", "claude", "--to", "gemini")
		cmd.Dir = h.Project
		cmd.Env = os.Environ()
		start := time.Now()
		out, err := cmd.Output()
		d := time.Since(start)
		if err != nil {
			t.Fatalf("%v", err)
		}
		if !strings.HasPrefix(string(out), "[i-mux handoff · Claude Code → Gemini CLI") {
			t.Fatalf("unexpected output %.200s", out)
		}
		if best == 0 || d < best {
			best = d
		}
		ru := cmd.ProcessState.SysUsage().(*syscall.Rusage)
		r := ru.Maxrss
		if runtime.GOOS == "linux" {
			r *= 1024 // kilobytes on Linux, bytes on macOS
		}
		if r > rss {
			rss = r
		}
	}
	t.Logf("handoff --print on 64 MB: best %v, peak RSS %.1f MB", best, float64(rss)/(1<<20))
	if best > time.Second {
		t.Errorf("took %v, budget 1s", best)
	}
	if rss > 80<<20 {
		t.Errorf("peak RSS %.1f MB, budget 80 MB", float64(rss)/(1<<20))
	}
}
