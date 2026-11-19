//go:build e2e

package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// envSecret is the fake value of the test's environment: no tmux command
// may carry it (D84).
const envSecret = "e2e-env-value-42"

// TestEnvironments (T89.5): an environment applied to an agent pane
// restarts the agent with it, resuming its conversation (after a question,
// unless --yes); a shell at its prompt gets it at once and the apply file
// deletes itself; removing it takes it away; a pane that runs another
// program is left alone; no value is ever on a tmux command line.
func TestEnvironments(t *testing.T) {
	h := newH(t, "")
	testutil.WriteFile(t, filepath.Join(h.Homes.Imux, "env", "glm.env"), []byte("# test\nE2E_ENV_PROBE="+envSecret+"\nPATH=/nowhere\n"))
	if out := h.mustImux("env", "ls"); !strings.Contains(out, "glm\tE2E_ENV_PROBE") || strings.Contains(out, envSecret) {
		t.Fatalf("env ls %q", out)
	}

	pane := h.startSession("claude")
	first := h.run("claude", 1)
	claudeID := first[1]
	if env := h.runEnv("claude", 1); env["E2E_ENV_PROBE"] != "" {
		t.Fatalf("no environment yet: %v", env)
	}
	time.Sleep(1100 * time.Millisecond) // whole-second timestamps in the fakes

	// Without --yes an agent pane asks first: nothing restarts.
	_, _, _ = h.imux("env", "apply", "glm", "--pane", pane)
	h.noRun("claude", 2, 1500*time.Millisecond)
	if got := h.paneOpt(pane, "#{@imux_env}"); got != "" {
		t.Fatalf("applied before the answer: %q", got)
	}

	out := h.mustImux("env", "apply", "glm", "--pane", pane, "--yes")
	if !strings.Contains(out, "Claude restarted with 🔑 glm") {
		t.Fatalf("apply output %q", out)
	}
	argv := h.run("claude", 2)
	if len(argv) < 2 || argv[0] != "--resume" || argv[1] != claudeID {
		t.Fatalf("restart argv %q (want --resume %s)", argv, claudeID)
	}
	env := h.runEnv("claude", 2)
	if env["E2E_ENV_PROBE"] != envSecret || strings.HasPrefix(env["PATH"], "/nowhere") {
		t.Fatalf("restarted agent's environment %v", env)
	}
	if got := h.paneOpt(pane, "#{@imux_env}"); got != "glm" {
		t.Fatalf("@imux_env %q", got)
	}
	if border := h.paneOpt(pane, "#{T:pane-border-format}"); !strings.Contains(border, "🔑 glm") {
		t.Fatalf("border %q", border)
	}

	// A switch away and back keeps the pane's environment.
	h.mustImux("switch", "antigravity", "--pane", pane)
	h.run("antigravity", 1)
	if env := h.runEnv("antigravity", 1); env["E2E_ENV_PROBE"] != envSecret {
		t.Fatalf("switched agent's environment %v", env)
	}

	// A shell at its prompt gets it at once.
	shell := strings.TrimSpace(h.mustTmux("split-window", "-d", "-t", pane, "-P", "-F", "#{pane_id}", "sh"))
	h.waitFor("shell prompt", 5*time.Second, func() bool {
		return h.paneOpt(shell, "#{pane_current_command}") == "sh" && strings.Contains(h.capture(shell), "$")
	})
	out = h.mustImux("env", "apply", "glm", "--pane", shell)
	if !strings.Contains(out, "glm applied to this shell") {
		t.Fatalf("shell apply output %q", out)
	}
	h.waitFor("apply file gone", 5*time.Second, func() bool {
		m, _ := filepath.Glob(filepath.Join(h.Homes.Imux, "env", ".apply-*"))
		return len(m) == 0
	})
	h.mustTmux("send-keys", "-t", shell, "echo probe=[$E2E_ENV_PROBE]", "Enter")
	h.waitFor("the shell has the variable", 5*time.Second, func() bool {
		return strings.Contains(h.capture(shell), "probe=["+envSecret+"]")
	})

	// Removing it from the shell unsets it.
	h.mustImux("env", "clear", "--pane", shell)
	h.waitFor("apply file gone", 5*time.Second, func() bool {
		m, _ := filepath.Glob(filepath.Join(h.Homes.Imux, "env", ".apply-*"))
		return len(m) == 0
	})
	h.mustTmux("send-keys", "-t", shell, "clear; echo cleared=[$E2E_ENV_PROBE]", "Enter")
	h.waitFor("the variable is gone", 5*time.Second, func() bool {
		return strings.Contains(h.capture(shell), "cleared=[]")
	})
	if got := h.paneOpt(shell, "#{@imux_env}"); got != "" {
		t.Fatalf("@imux_env after clear %q", got)
	}

	// Another program is left alone.
	busy := strings.TrimSpace(h.mustTmux("split-window", "-d", "-t", pane, "-P", "-F", "#{pane_id}", "sleep 600"))
	// Started with a command: busy, whether tmux shows "sleep" or the
	// "sh -c" that runs it.
	h.waitFor("sleep runs", 5*time.Second, func() bool { return h.paneOpt(busy, "#{pane_pid}") != "" })
	if _, _, err := h.imux("env", "apply", "glm", "--pane", busy); err == nil {
		t.Fatal("applying to a busy pane must fail")
	}
	if got := h.paneOpt(busy, "#{@imux_env}"); got != "" {
		t.Fatalf("busy pane got %q", got)
	}

	for _, r := range h.records() {
		if strings.Contains(strings.Join(r.Args, " "), envSecret) {
			t.Fatalf("a value on a tmux command line: %q", r.Args)
		}
	}
}

// TestEnvNewPopup (T89.7): the Env view's + opens the New environment
// dialog in a popup in the middle of the client: what is typed shows as
// the name it makes; Enter writes the file and opens it in the editor in
// that popup, on the line under its template, and the view lists it. A
// taken name is refused, and Esc closes the dialog making nothing.
func TestEnvNewPopup(t *testing.T) {
	ed := filepath.Join(t.TempDir(), "fake-editor")
	testutil.WriteFile(t, ed, []byte("#!/bin/sh\necho \"EDIT $1 $(basename \"$2\")\"\nread x\n"))
	if err := os.Chmod(ed, 0o755); err != nil {
		t.Fatal(err)
	}
	h := newH(t, "editor = \""+ed+"\"\n"+sidebarConfig)
	_, bar := h.startWithSidebar()
	c, client := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	h.mustTmux("set-option", "-w", "-t", bar, "@imux_view", "env")
	_, _, _ = h.imux("__sidebar", "--signal")
	h.rowOf(bar, "New environment…")

	open := func() {
		t.Helper()
		time.Sleep(400 * time.Millisecond)
		x, y := h.buttonOf(bar, "ENV", "➕")
		mark := c.Mark()
		c.Click(0, x+1, y)
		c.waitScreenAfter(h, mark, "e.g. glm, work, proxy", 5*time.Second)
		time.Sleep(200 * time.Millisecond)
	}
	// closed waits for the dialog's process to end. tmux 3.3+ holds a lone
	// Esc 500 ms while a client has not answered its DA queries (this pty
	// never does): a click before then would reach the popup with it.
	closed := func() {
		t.Helper()
		h.waitFor("the dialog closed", 5*time.Second, func() bool {
			out, err := exec.Command("ps", "-A", "-o", "args=").Output()
			return err == nil && !strings.Contains(string(out), "env new --client "+client)
		})
	}
	file := filepath.Join(h.Homes.Imux, "env", "glm-coding.env")

	open()
	mark := c.Mark()
	c.Write("GLM Coding")
	c.waitScreenAfter(h, mark, "~/.imux/env/glm-coding.env", 5*time.Second)
	mark = c.Mark()
	c.Write("\r")
	h.waitFor("the new file", 5*time.Second, func() bool {
		_, err := os.Stat(file)
		return err == nil
	})
	text := h.readFile(file)
	c.waitScreenAfter(h, mark, "EDIT +"+strconv.Itoa(strings.Count(text, "\n")+1)+" glm-coding.env", 5*time.Second)
	if fi, err := os.Stat(file); err != nil || fi.Mode().Perm() != 0o600 || !strings.Contains(text, "# ANTHROPIC_BASE_URL=") {
		t.Fatalf("file %v %v:\n%s", fi, err, text)
	}
	c.Write("\r")
	closed()
	h.rowOf(bar, "glm-coding")

	// The same name again: refused; Esc closes.
	open()
	mark = c.Mark()
	c.Write("glm-coding")
	c.waitScreenAfter(h, mark, "An environment named glm-coding already exists", 5*time.Second)
	mark = c.Mark()
	c.Write("\r")
	time.Sleep(500 * time.Millisecond)
	if strings.Contains(stripANSI(c.Output()[mark:]), "EDIT ") {
		t.Fatal("a taken name opened the editor")
	}
	c.Write("\x1b")
	closed()

	// Esc with a name typed makes nothing.
	open()
	c.Write("scratch")
	time.Sleep(300 * time.Millisecond)
	c.Write("\x1b")
	closed()
	if _, err := os.Stat(filepath.Join(h.Homes.Imux, "env", "scratch.env")); err == nil {
		t.Fatal("Esc made an environment")
	}

	popups := 0
	for _, r := range h.records() {
		a := strings.Join(r.Args, " ")
		if strings.Contains(a, "command-prompt") {
			t.Fatalf("a command prompt: %q", r.Args)
		}
		if strings.Contains(a, "display-popup") && strings.Contains(a, "-w 78 -h 22 -x C -y C") && strings.Contains(a, "env new") {
			popups++
		}
	}
	if popups != 3 {
		t.Fatalf("%d New environment popups, want 3", popups)
	}
}
