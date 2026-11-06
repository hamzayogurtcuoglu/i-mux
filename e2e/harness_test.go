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
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/state"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
	"github.com/hamzayogurtcuoglu/i-mux/internal/tmux"
	"github.com/hamzayogurtcuoglu/i-mux/internal/watch"
)

// H is one isolated e2e environment: its own tmux socket, IMUX_HOME, fake
// agent homes, fake agent log directory and tmux command recorder.
type H struct {
	t      *testing.T
	Bin    string // imux under test
	Homes  *testutil.Homes
	Socket string
	Log    string // IMUX_FAKE_LOG
	Fakes  string // e2e/fakeagents
	Rec    string // IMUX_TMUX_RECORD
	Tmux   string // tmux binary
}

// newH sets up the environment. extraConfig is appended to config.toml.
func newH(t *testing.T, extraConfig string) *H {
	t.Helper()
	bin := imuxBin(t)
	tm, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not installed")
	}
	homes := testutil.Isolate(t)
	var r [5]byte
	_, _ = rand.Read(r[:])
	h := &H{
		t: t, Bin: bin, Homes: homes, Tmux: tm,
		Socket: "imux-test-" + hex.EncodeToString(r[:]),
		Log:    filepath.Join(homes.Root, "fakelog"),
		Fakes:  stagedFakes,
		Rec:    filepath.Join(homes.Root, "tmux-record.jsonl"),
	}
	if err := os.MkdirAll(h.Log, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IMUX_TMUX_SOCKET", h.Socket)
	t.Setenv("IMUX_TMUX_RECORD", h.Rec)
	t.Setenv("IMUX_FAKE_LOG", h.Log)
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
	if !strings.Contains(tables, "[limits]") {
		// A fast watcher keeps tests short; it still has to see a screen
		// twice before it acts.
		cfg.WriteString("[limits]\npoll_interval = \"200ms\"\nidle_interval = \"500ms\"\ngrace = \"0s\"\n\n")
	}
	cfg.WriteString(tables)
	testutil.WriteFile(t, filepath.Join(homes.Imux, "config.toml"), []byte(cfg.String()))
	t.Cleanup(func() {
		failed := t.Failed()
		if failed {
			h.dumpPanes() // while the server still runs
		}
		_ = exec.Command(h.Bin, "__watch", "stop").Run()
		_ = exec.Command(h.Tmux, "-L", h.Socket, "kill-server").Run()
		h.waitQuiet()
		h.checkBins()
		if failed {
			h.dumpLogs()
		}
	})
	return h
}

// checkBins fails the test if imux ever resolved an agent outside e2e/fakeagents.
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
	if b, err := os.ReadFile(filepath.Join(h.Homes.Imux, "watch.log")); err == nil {
		h.t.Logf("--- watch.log ---\n%.4000s", b)
	}
}

// dumpPanes logs the state of every pane and the last tmux calls.
func (h *H) dumpPanes() {
	panes, _ := h.tmux("list-panes", "-a", "-F",
		"#{pane_id} pid=#{pane_pid} dead=#{pane_dead} status=#{pane_dead_status} signal=#{pane_dead_signal} remain=#{remain-on-exit} cmd=#{pane_current_command} agent=#{@imux_agent}")
	h.t.Logf("--- panes ---\n%s", panes)
	// The processes of this test (tmux server, pane processes, their
	// children), to tell a zombie or stopped agent from a running one.
	pids := map[string]bool{}
	for _, f := range strings.Fields(panes) {
		if p, ok := strings.CutPrefix(f, "pid="); ok {
			pids[p] = true
		}
	}
	if ps, err := exec.Command("ps", "-A", "-o", "pid=,ppid=,stat=,etime=,args=").Output(); err == nil {
		var b strings.Builder
		for _, l := range strings.Split(string(ps), "\n") {
			f := strings.Fields(l)
			if len(f) > 4 && (pids[f[0]] || pids[f[1]] || strings.Contains(l, h.Socket) || strings.Contains(l, h.Homes.Root)) {
				b.WriteString(l + "\n")
			}
		}
		h.t.Logf("--- processes ---\n%s", b.String())
	}
	if out, err := exec.Command("grep", "-E", "^Sig(Blk|Ign|Cgt)", "/proc/self/status").Output(); err == nil {
		h.t.Logf("--- test process signal masks ---\n%s", out)
	}
	if srv, _ := h.tmux("display-message", "-p", "#{pid}"); strings.TrimSpace(srv) != "" {
		if out, err := exec.Command("grep", "-E", "^(State|Sig(Blk|Ign|Cgt))", "/proc/"+strings.TrimSpace(srv)+"/status").Output(); err == nil {
			h.t.Logf("--- tmux server %s ---\n%s", strings.TrimSpace(srv), out)
		}
	}
	// The last tmux calls made through imux by hooks and UI commands, with
	// the pid that made them (not the watcher: it polls every tick).
	_, watcher := watch.Status()
	var recs []tmux.Recorded
	for _, r := range h.records() {
		if r.Pid != watcher {
			recs = append(recs, r)
		}
	}
	if len(recs) > 25 {
		recs = recs[len(recs)-25:]
	}
	var b strings.Builder
	for _, r := range recs {
		fmt.Fprintf(&b, "%d %.300q\n", r.Pid, r.Args)
	}
	h.t.Logf("--- last tmux calls (watcher pid %d excluded) ---\n%s", watcher, b.String())
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

// sessionPath waits for fake agent run n to record the session file it
// wrote and returns that file's path. The fake writes it after its .env, so
// run returning does not mean it exists yet.
func (h *H) sessionPath(agent string, n int) string {
	h.t.Helper()
	p := filepath.Join(h.Log, fmt.Sprintf("%s-%d.session", agent, n))
	var b []byte
	h.waitFor(fmt.Sprintf("fake %s run %d session", agent, n), 5*time.Second, func() bool {
		var err error
		b, err = os.ReadFile(p)
		return err == nil && strings.HasSuffix(string(b), "\n")
	})
	return strings.TrimSpace(string(b))
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

// imux runs the binary in the project directory.
func (h *H) imux(args ...string) (string, string, error) {
	h.t.Helper()
	return h.imuxEnv(nil, args...)
}

// imuxEnv runs the binary with extra environment variables.
func (h *H) imuxEnv(env []string, args ...string) (string, string, error) {
	h.t.Helper()
	cmd := exec.Command(h.Bin, args...)
	cmd.Dir = h.Homes.Project
	cmd.Env = append(os.Environ(), env...)
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	return o.String(), e.String(), err
}

// mustImux runs imux and fails the test on error.
func (h *H) mustImux(args ...string) string {
	h.t.Helper()
	out, errOut, err := h.imux(args...)
	if err != nil {
		h.t.Fatalf("imux %s: %v\nstdout: %s\nstderr: %s", strings.Join(args, " "), err, out, errOut)
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

// newPane runs `imux <agent>` inside pane (opens a window) and returns the
// new pane's id.
func (h *H) newPane(from, agent string) string {
	h.t.Helper()
	before := h.mustTmux("list-panes", "-a", "-F", "#{pane_id}")
	if o, e, err := h.imuxEnv(h.inside(from), agent); err != nil {
		h.t.Fatalf("imux %s inside %s: %v\n%s%s", agent, from, err, o, e)
	}
	for _, p := range strings.Fields(h.mustTmux("list-panes", "-a", "-F", "#{pane_id}")) {
		if !strings.Contains(before+"\n", p+"\n") {
			return p
		}
	}
	h.t.Fatal("no new pane")
	return ""
}

// startSession runs `imux <agent> -d` and returns the first pane.
func (h *H) startSession(agent string) string {
	h.t.Helper()
	h.mustImux(agent, "-d")
	return strings.TrimSpace(h.mustTmux("list-panes", "-a", "-F", "#{pane_id}"))
}

// records returns the tmux invocations recorded so far.
func (h *H) records() []tmux.Recorded {
	r, _ := tmux.ReadRecord(h.Rec)
	return r
}

// attach starts a pty client of cols×rows attached to the project's session
// and returns it with its tmux client name.
func (h *H) attach(cols, rows int) (*Client, string) {
	h.t.Helper()
	c := h.startClient(cols, rows, h.Homes.Project, h.Tmux, "-u", "-L", h.Socket, "attach-session")
	var name string
	h.waitFor("client attached", 5*time.Second, func() bool {
		out, _ := h.tmux("list-clients", "-F", "#{client_name}")
		name = strings.TrimSpace(out)
		return name != ""
	})
	return c, name
}

// waitScreen waits until the client's output (escape sequences removed)
// contains s.
func (c *Client) waitScreen(h *H, s string, timeout time.Duration) {
	h.t.Helper()
	h.waitFor("screen to show "+strconv.Quote(s), timeout, func() bool { return strings.Contains(c.Text(), s) })
}

// waitScreenAfter is waitScreen for output written after offset (see Mark).
func (c *Client) waitScreenAfter(h *H, offset int, s string, timeout time.Duration) {
	h.t.Helper()
	h.waitFor("screen to show "+strconv.Quote(s), timeout, func() bool {
		out := c.Output()
		if offset > len(out) {
			offset = len(out)
		}
		return strings.Contains(stripANSI(out[offset:]), s)
	})
}

// Mark returns the current output length (for waitScreenAfter).
func (c *Client) Mark() int { return len(c.Output()) }

// Text is the client's output without escape sequences.
func (c *Client) Text() string { return stripANSI(c.Output()) }

var ansiRe = regexp.MustCompile("\x1b(\\[[0-9;?<>=]*[ -/]*[@-~]|\\][^\x07\x1b]*(\x07|\x1b\\\\)|[()][0-9A-Za-z]|[=>78DEHMNOc])")

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// noRun asserts that agent run n does not appear within d.
func (h *H) noRun(agent string, n int, d time.Duration) {
	h.t.Helper()
	time.Sleep(d)
	if _, err := os.Stat(filepath.Join(h.Log, fmt.Sprintf("%s-%d.env", agent, n))); err == nil {
		h.t.Fatalf("unexpected fake %s run %d", agent, n)
	}
}

// waitQuiet waits (up to 5 s) until no imux process started from the staged
// binary and no fake agent of this test is left: background commands run by
// menus and hooks may still be finishing when a test ends.
func (h *H) waitQuiet() {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		out, _ := exec.Command("pgrep", "-f", "^"+regexp.QuoteMeta(h.Bin)).Output()
		if len(strings.TrimSpace(string(out))) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// capture returns the visible screen of pane.
func (h *H) capture(pane string) string {
	out, _ := h.tmux("capture-pane", "-p", "-J", "-t", pane)
	return out
}

// control writes the fake agent's control script ($IMUX_FAKE_LOG/<agent>.sh).
func (h *H) control(agent, script string) {
	h.t.Helper()
	testutil.WriteFile(h.t, filepath.Join(h.Log, agent+".sh"), []byte(script))
}

func (h *H) readFile(p string) string {
	b, _ := os.ReadFile(p)
	return string(b)
}

// uiRecords counts tmux invocations made by processes other than the
// watcher (whose ticks also go through the recorder).
func (h *H) uiRecords() int {
	_, pid := watch.Status()
	n := 0
	for _, r := range h.records() {
		if r.Pid != pid {
			n++
		}
	}
	return n
}
