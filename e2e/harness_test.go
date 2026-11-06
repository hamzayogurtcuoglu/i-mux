//go:build e2e

package e2e

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
)

// H is one isolated e2e environment: its own tmux socket, AJAN_HOME, fake
// agent homes, fake agent log directory and tmux command recorder.
type H struct {
	t      *testing.T
	Bin    string // ajan under test
	Homes  *testutil.Homes
	Socket string
	Log    string // AJAN_FAKE_LOG
	Fakes  string // e2e/fakeagents
	Rec    string // AJAN_TMUX_RECORD
	Tmux   string // tmux binary
}

// newH sets up the environment. extraConfig is appended to config.toml.
func newH(t *testing.T, extraConfig string) *H {
	t.Helper()
	bin := ajanBin(t)
	tm, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not installed")
	}
	homes := testutil.Isolate(t)
	var r [5]byte
	_, _ = rand.Read(r[:])
	h := &H{
		t: t, Bin: bin, Homes: homes, Tmux: tm,
		Socket: "ajan-test-" + hex.EncodeToString(r[:]),
		Log:    filepath.Join(homes.Root, "fakelog"),
		Fakes:  filepath.Join(testutil.ModuleRoot(t), "e2e", "fakeagents"),
		Rec:    filepath.Join(homes.Root, "tmux-record.jsonl"),
	}
	if err := os.MkdirAll(h.Log, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AJAN_TMUX_SOCKET", h.Socket)
	t.Setenv("AJAN_TMUX_RECORD", h.Rec)
	t.Setenv("AJAN_FAKE_LOG", h.Log)
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	t.Setenv("SHELL", "/bin/sh")
	t.Setenv("TERM", "xterm-256color")
	// PATH holds tmux and the base system only: never a real agent CLI.
	t.Setenv("PATH", filepath.Dir(tm)+":/usr/bin:/bin")
	var cfg strings.Builder
	// Top-level keys of extraConfig must come before any table.
	top, tables := extraConfig, ""
	if strings.HasPrefix(extraConfig, "[") {
		top, tables = "", extraConfig
	} else if i := strings.Index(extraConfig, "\n["); i >= 0 {
		top, tables = extraConfig[:i+1], extraConfig[i+1:]
	}
	cfg.WriteString(top + "\n")
	for _, a := range []string{"claude", "gemini", "copilot", "codex"} {
		if strings.Contains(tables, "[agents."+a+"]") {
			continue // the test configures this agent itself
		}
		fmt.Fprintf(&cfg, "[agents.%s]\nbin = %q\n\n", a, filepath.Join(h.Fakes, "fake-"+a))
	}
	cfg.WriteString(tables)
	testutil.WriteFile(t, filepath.Join(homes.Ajan, "config.toml"), []byte(cfg.String()))
	t.Cleanup(func() {
		_ = exec.Command(h.Tmux, "-L", h.Socket, "kill-server").Run()
		h.checkBins()
		if t.Failed() {
			h.dumpLogs()
		}
	})
	return h
}

// checkBins fails the test if ajan ever resolved an agent outside e2e/fakeagents.
func (h *H) checkBins() {
	st, err := state.Read()
	if err != nil {
		return
	}
	for id, b := range st.Bins {
		if b != nil && b.Path != "" && !strings.HasPrefix(b.Path, h.Fakes+string(filepath.Separator)) {
			h.t.Errorf("agent %s resolved outside the fake agents: %s", id, b.Path)
		}
	}
}

func (h *H) dumpLogs() {
	ents, _ := os.ReadDir(h.Log)
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".argv") {
			continue
		}
		b, _ := os.ReadFile(filepath.Join(h.Log, e.Name()))
		h.t.Logf("--- %s ---\n%.2000s", e.Name(), b)
	}
	if b, err := os.ReadFile(filepath.Join(h.Homes.Ajan, "watch.log")); err == nil {
		h.t.Logf("--- watch.log ---\n%.4000s", b)
	}
}

// tmux runs a tmux command on the test socket.
func (h *H) tmux(args ...string) (string, error) {
	cmd := exec.Command(h.Tmux, append([]string{"-L", h.Socket}, args...)...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (h *H) mustTmux(args ...string) string {
	h.t.Helper()
	out, err := h.tmux(args...)
	if err != nil {
		h.t.Fatalf("tmux %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// waitFor polls cond every 50 ms until it holds or timeout passes.
func (h *H) waitFor(what string, timeout time.Duration, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("timed out after %v waiting for %s", timeout, what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// run waits for fake agent run n of agent and returns its argv.
func (h *H) run(agent string, n int) []string {
	h.t.Helper()
	env := filepath.Join(h.Log, fmt.Sprintf("%s-%d.env", agent, n))
	h.waitFor(fmt.Sprintf("fake %s run %d", agent, n), 10*time.Second, func() bool {
		_, err := os.Stat(env)
		return err == nil
	})
	b, err := os.ReadFile(filepath.Join(h.Log, fmt.Sprintf("%s-%d.argv", agent, n)))
	if err != nil {
		h.t.Fatal(err)
	}
	return splitNUL(b)
}

// runEnv returns the recorded environment of run n.
func (h *H) runEnv(agent string, n int) map[string]string {
	h.t.Helper()
	b, err := os.ReadFile(filepath.Join(h.Log, fmt.Sprintf("%s-%d.env", agent, n)))
	if err != nil {
		h.t.Fatal(err)
	}
	m := map[string]string{}
	for _, l := range strings.Split(string(b), "\n") {
		if k, v, ok := strings.Cut(l, "="); ok {
			m[k] = v
		}
	}
	return m
}

func splitNUL(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	parts := strings.Split(string(b), "\x00")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

// ajan runs the binary in the project directory.
func (h *H) ajan(args ...string) (string, string, error) {
	h.t.Helper()
	return h.ajanEnv(nil, args...)
}

// ajanEnv runs the binary with extra environment variables.
func (h *H) ajanEnv(env []string, args ...string) (string, string, error) {
	h.t.Helper()
	cmd := exec.Command(h.Bin, args...)
	cmd.Dir = h.Homes.Project
	cmd.Env = append(os.Environ(), env...)
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	return o.String(), e.String(), err
}

// mustAjan runs ajan and fails the test on error.
func (h *H) mustAjan(args ...string) string {
	h.t.Helper()
	out, errOut, err := h.ajan(args...)
	if err != nil {
		h.t.Fatalf("ajan %s: %v\nstdout: %s\nstderr: %s", strings.Join(args, " "), err, out, errOut)
	}
	return out
}

// paneOpt reads one pane option (formats allowed).
func (h *H) paneOpt(pane, format string) string {
	h.t.Helper()
	return strings.TrimSuffix(h.mustTmux("display-message", "-p", "-t", pane, format), "\n")
}

// runs counts the runs of agent so far.
func (h *H) runs(agent string) int {
	m, _ := filepath.Glob(filepath.Join(h.Log, agent+"-*.env"))
	return len(m)
}

// inside returns the environment of a process running in pane (TMUX and
// TMUX_PANE pointing at the test server).
func (h *H) inside(pane string) []string {
	h.t.Helper()
	sock := strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", pane, "#{socket_path}"))
	return []string{"TMUX=" + sock + ",1,0", "TMUX_PANE=" + pane}
}

// newPane runs `ajan <agent>` inside pane (opens a window) and returns the
// new pane's id.
func (h *H) newPane(from, agent string) string {
	h.t.Helper()
	before := h.mustTmux("list-panes", "-a", "-F", "#{pane_id}")
	if o, e, err := h.ajanEnv(h.inside(from), agent); err != nil {
		h.t.Fatalf("ajan %s inside %s: %v\n%s%s", agent, from, err, o, e)
	}
	for _, p := range strings.Fields(h.mustTmux("list-panes", "-a", "-F", "#{pane_id}")) {
		if !strings.Contains(before+"\n", p+"\n") {
			return p
		}
	}
	h.t.Fatal("no new pane")
	return ""
}

// startSession runs `ajan <agent> -d` and returns the first pane.
func (h *H) startSession(agent string) string {
	h.t.Helper()
	h.mustAjan(agent, "-d")
	return strings.TrimSpace(h.mustTmux("list-panes", "-a", "-F", "#{pane_id}"))
}

// records returns the tmux invocations recorded so far.
func (h *H) records() []tmux.Recorded {
	r, _ := tmux.ReadRecord(h.Rec)
	return r
}
