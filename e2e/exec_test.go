//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/launch"
)

// TestExecByteExactArgv (T2.3): `ajan __exec <spec>` in a tmux pane deletes
// the spec, sets AJAN_PANE / AJAN_AGENT / PATH and hands the fake agent its
// arguments byte for byte, without any shell in between.
func TestExecByteExactArgv(t *testing.T) {
	h := newH(t, "")
	args := []string{
		"--session-id", "11111111-2222-4333-8444-555555555555",
		"it's \"quoted\" and 'single'",
		"line one\nline two\n",
		"$(touch /tmp/ajan-pwned) `id` $HOME ; | & > <",
		"Türkçe: ğüşıöç ĞÜŞİÖÇ · emoji 🚀",
		"ends with a semicolon;",
		"",
		"--",
	}
	specPATH := filepath.Join(h.Homes.Root, "agentbin") + ":/usr/bin:/bin"
	spec, err := launch.Write(&launch.Spec{
		Agent: "claude", Bin: filepath.Join(h.Fakes, "fake-claude"), Args: args,
		Dir: h.Homes.Project, Env: map[string]string{"PATH": specPATH},
	})
	if err != nil {
		t.Fatal(err)
	}
	pane := strings.TrimSpace(h.mustTmux("-f", "/dev/null", "new-session", "-d", "-s", "exec", "-x", "120", "-y", "40",
		"-P", "-F", "#{pane_id}", h.Bin, "__exec", spec))
	got := h.run("claude", 1)
	if !reflect.DeepEqual(got, args) {
		t.Fatalf("argv mismatch\n got %q\nwant %q", got, args)
	}
	if _, err := os.Stat(spec); !os.IsNotExist(err) {
		t.Fatal("launch spec was not deleted")
	}
	if _, err := os.Stat("/tmp/ajan-pwned"); err == nil {
		os.Remove("/tmp/ajan-pwned")
		t.Fatal("an argument was evaluated by a shell")
	}
	env := h.runEnv("claude", 1)
	if env["AJAN_PANE"] != pane || env["TMUX_PANE"] != pane {
		t.Errorf("AJAN_PANE=%q TMUX_PANE=%q, want %q", env["AJAN_PANE"], env["TMUX_PANE"], pane)
	}
	if env["AJAN_AGENT"] != "claude" {
		t.Errorf("AJAN_AGENT=%q", env["AJAN_AGENT"])
	}
	if env["PATH"] != specPATH {
		t.Errorf("PATH=%q, want %q", env["PATH"], specPATH)
	}
	if env["PWD"] != h.Homes.Project {
		t.Errorf("PWD=%q, want %q", env["PWD"], h.Homes.Project)
	}
	// The fake wrote a session file where Claude would.
	if sess := h.sessionPath("claude", 1); !strings.HasSuffix(sess, "11111111-2222-4333-8444-555555555555.jsonl") {
		t.Fatalf("session file: %q", sess)
	}
}
