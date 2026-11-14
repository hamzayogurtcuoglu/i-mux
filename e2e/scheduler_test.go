//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/auto"
	"github.com/hamzayogurtcuoglu/i-mux/internal/sidebar"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// buttonOf returns the client position of glyph on the first sidebar line
// with text.
func (h *H) buttonOf(bar, text, glyph string) (x, y int) {
	h.t.Helper()
	_, y = h.rowOf(bar, text)
	l := h.lineOf(bar, text)
	j := strings.Index(l, glyph)
	if j < 0 {
		h.t.Fatalf("no %s on the line %q", glyph, l)
	}
	left, _ := strconv.Atoi(strings.TrimSpace(h.mustTmux("display-message", "-p", "-t", bar, "#{pane_left}")))
	return left + sidebar.DisplayWidth(l[:j]) + 1, y
}

// headless is the argv of agent's n-th print-mode run.
func (h *H) headless(agent string, n int) []string {
	h.t.Helper()
	p := filepath.Join(h.Log, agent+"-headless-"+itoa(n)+".argv")
	var argv []string
	h.waitFor(agent+" print-mode run "+itoa(n), 10*time.Second, func() bool {
		b, err := os.ReadFile(p)
		argv = splitNUL(b)
		return err == nil && len(argv) > 0
	})
	return argv
}

// TestScheduler (T66.4, T66.6): the ⏰ view lists the project's agents
// and skills. 🚀 opens the Run panel with everything chosen (the agent's
// own CLI, the default model, now): Enter runs it headless with --agent and
// Claude's event stream, and its output opens and follows it, tool steps
// and answer. ⋯ → Schedule… takes a time typed in Turkish units; a skill
// scheduled every 5 minutes from the panel starts at once; the watcher
// starts a schedule that is due. A run going on is followed again from its
// row and stopped with s; ⋯ shows the last output and removes a schedule.
func TestScheduler(t *testing.T) {
	h := newH(t, sidebarConfig)
	t.Setenv("IMUX_AUTO_CHECK", "300ms")
	p := h.Homes.Project
	agentFile := filepath.Join(p, ".claude", "agents", "sim-tag.md")
	skillFile := filepath.Join(p, ".agents", "skills", "notes", "SKILL.md")
	testutil.WriteFile(t, agentFile, []byte("---\nname: sim-tag\ndescription: Watches the simulator's tags\n---\nCompare the newest tag.\n"))
	testutil.WriteFile(t, skillFile, []byte("---\nname: notes\ndescription: Writes release notes\n---\n# Notes\n"))
	reply := func(s string) { testutil.WriteFile(t, filepath.Join(h.Log, "claude.headless-reply"), []byte(s)) }
	pause := filepath.Join(h.Log, "claude.headless-pause")
	schedules := func() string { b, _ := os.ReadFile(auto.Path()); return string(b) }
	runs := func(id string) string {
		return h.readFile(filepath.Join(h.Homes.Imux, "auto", id, "runs.json"))
	}

	_, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	h.openView(c, bar, "⏰", "schedule")
	h.rowOf(bar, "Watches the simulator's tags")
	h.rowOf(bar, "Writes release notes")
	h.rowOf(bar, "🚀 run now or on a schedule")
	if l := h.lineOf(bar, "JOBS"); !strings.Contains(l, "✨") {
		t.Errorf("no ✨ New skill… in the title: %q", l) // D66
	}
	// press clicks glyph on the line with text and waits for want.
	press := func(text, glyph, want string) {
		t.Helper()
		time.Sleep(500 * time.Millisecond)
		x, y := h.buttonOf(bar, text, glyph)
		mark := c.Mark()
		c.Click(0, x+sidebar.DisplayWidth(glyph)-1, y)
		c.waitScreenAfter(h, mark, want, 5*time.Second)
		time.Sleep(300 * time.Millisecond)
	}
	// keys types into a popup and waits for want.
	keys := func(k, want string) {
		t.Helper()
		mark := c.Mark()
		c.Write(k)
		c.waitScreenAfter(h, mark, want, 10*time.Second)
		time.Sleep(300 * time.Millisecond)
	}

	// 🚀: the Run panel says what Enter does; Enter runs it as Claude's
	// own agent, and the output opens: the tool step, then the answer.
	reply("v2.3 is out: pipeline started\nThe simulator's pipeline runs.")
	press("sim-tag", "🚀", "Runs now with Claude, full access")
	keys("\r", "The simulator's pipeline runs.")
	c.waitScreen(h, "sim-tag · Claude · fake-model", 5*time.Second)
	c.waitScreen(h, "● Bash echo fake step 1", 5*time.Second)
	c.waitScreen(h, "⎿ fake step 1", 5*time.Second)
	argv := h.headless("claude", 1)
	if len(argv) < 10 || strings.Join(argv[:5], " ") != "-p --output-format stream-json --verbose --session-id" ||
		argv[6] != "--agent" || argv[7] != "sim-tag" || argv[8] != "--dangerously-skip-permissions" ||
		strings.Contains(strings.Join(argv, " "), "--model") {
		t.Fatalf("argv %q", argv)
	}
	if prompt := argv[len(argv)-1]; !strings.Contains(prompt, "started by hand") || !strings.Contains(prompt, "This is the first run.") ||
		strings.Contains(prompt, "Compare the newest tag.") {
		t.Errorf("prompt:\n%s", prompt)
	}
	c.Write("q")
	h.rowOf(bar, "v2.3 is out")
	if s := schedules(); !strings.Contains(s, `name = "sim-tag"`) || !strings.Contains(s, `cli = "claude"`) || !strings.Contains(s, `when = ""`) {
		t.Errorf("schedules.toml:\n%s", s)
	}

	// ⋯ → Schedule…: the panel at its time; one typed in Turkish units.
	press("sim-tag", "⋯", "Schedule")
	keys("t", "Runs now with Claude")
	keys("2 saat", "Runs every 2 h with Claude")
	c.Write("\r")
	h.waitFor("every 2h", 5*time.Second, func() bool { return strings.Contains(schedules(), `when = "2h"`) })
	h.rowOf(bar, "every 2 h · Claude")

	// 🚀 on the skill, its time every 5 minutes: scheduled, it starts at
	// once with the skill's path in its prompt, and its output opens.
	reply("Release notes are written")
	press("notes", "🚀", "Runs now with")
	c.Write("c")
	c.Write("\t\t")
	keys("\x1b[C", "Runs every 5 min with Claude")
	keys("\r", "notes · Claude · fake-model")
	c.waitScreen(h, "Release notes are written", 5*time.Second)
	c.Write("q")
	argv = h.headless("claude", 2)
	if prompt := argv[len(argv)-1]; !strings.HasPrefix(prompt, `Use the Agent Skill "notes" at `+skillFile) || !strings.Contains(prompt, "on a schedule (every 5m)") {
		t.Errorf("skill prompt:\n%s", prompt)
	}
	if strings.Contains(strings.Join(argv, " "), "--agent") {
		t.Errorf("--agent for a skill: %q", argv)
	}
	h.rowOf(bar, "every 5 min · Claude")
	h.rowOf(bar, "Release notes are")
	if s := schedules(); !strings.Contains(s, `name = "notes"`) || !strings.Contains(s, `when = "5m"`) {
		t.Errorf("schedules.toml:\n%s", s)
	}

	// Due: its last run was 10 minutes ago. The watcher starts it.
	id := auto.ID(p, skillFile)
	old := time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	testutil.WriteFile(t, filepath.Join(h.Homes.Imux, "auto", id, "runs.json"), []byte(`[{"start":"`+old+`","end":"`+old+`","status":"ok"}]`))
	reply("Nothing new since " + old)
	argv = h.headless("claude", 3)
	if prompt := argv[len(argv)-1]; !strings.Contains(prompt, "on a schedule (every 5m)") {
		t.Errorf("watcher's run prompt:\n%s", prompt)
	}
	h.rowOf(bar, "Nothing new since")
	time.Sleep(time.Second)
	if _, err := os.Stat(filepath.Join(h.Log, "claude-headless-4.argv")); err == nil {
		t.Error("a schedule started twice")
	}

	// A run that takes long: its output follows it; closed, it goes on
	// (🛑 on its row); its row opens the output again, where s stops it.
	testutil.WriteFile(t, pause, []byte("30"))
	press("sim-tag", "🚀", "Runs now with Claude")
	keys("\r", "● Bash echo fake step 4")
	c.waitScreen(h, "s stop", 5*time.Second)
	c.Write("q")
	h.waitFor("🛑 on sim-tag", 5*time.Second, func() bool { return strings.Contains(h.lineOf(bar, "sim-tag"), "🛑") })
	h.rowOf(bar, "running ·")
	time.Sleep(500 * time.Millisecond)
	x, y := h.rowOf(bar, "sim-tag")
	mark := c.Mark()
	c.Click(0, x+1, y)
	c.waitScreenAfter(h, mark, "● Bash echo fake step 4", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	keys("s", "■ sim-tag")
	c.Write("q")
	simID := auto.ID(p, agentFile)
	h.waitFor("the run stopped", 5*time.Second, func() bool { return strings.Contains(runs(simID), `"status": "stopped"`) })
	h.waitFor("🚀 back on sim-tag", 5*time.Second, func() bool { return strings.Contains(h.lineOf(bar, "sim-tag"), "🚀") })
	if err := os.Remove(pause); err != nil {
		t.Fatal(err)
	}

	// ⋯ → Output: the last run's output in a popup.
	press("notes", "⋯", "Remove schedule")
	keys("o", "notes · Claude · fake-model")
	c.waitScreen(h, "Nothing new since", 5*time.Second)
	c.Write("q")

	// A right-click → Remove schedule… → Remove: back among the skills.
	time.Sleep(700 * time.Millisecond)
	x, y = h.rowOf(bar, "notes")
	mark = c.Mark()
	c.Click(2, x, y)
	c.waitScreenAfter(h, mark, "Remove schedule", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	keys("x", "Remove the schedule of notes?")
	c.Write("y")
	h.waitFor("the schedule removed", 5*time.Second, func() bool { return !strings.Contains(schedules(), `name = "notes"`) })
	h.rowOf(bar, "Writes release notes")
	h.rowOf(bar, "SCHEDULED  1")
	if _, err := os.Stat(filepath.Join(h.Homes.Imux, "auto", id)); !os.IsNotExist(err) {
		t.Errorf("its runs stayed: %v", err)
	}
}
