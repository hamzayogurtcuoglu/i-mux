//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestSkillsInJobs (M40, M42, T71.5): Jobs lists project skills, and a
// skill's ⋯ menu shares a single-agent skill with all agents. ✨ New skill…
// in its title opens the New skill dialog: a blank SKILL.md is named there
// and opened in the editor; this pane's agent gets i-mux's skill-writing
// guide pasted; an agent chosen for a new pane starts beside it, named,
// with the guide and the description as its first message. A skill's menu
// sends an improve prompt to the active agent and deletes the skill with
// its link. A sidebar left at the removed Skills view shows Jobs.
func TestSkillsInJobs(t *testing.T) {
	ed := filepath.Join(t.TempDir(), "fake-editor")
	testutil.WriteFile(t, ed, []byte("#!/bin/sh\necho \"EDIT $*\"\nread x\n"))
	if err := os.Chmod(ed, 0o755); err != nil {
		t.Fatal(err)
	}
	h := newH(t, "editor = \""+ed+"\"\n"+sidebarConfig)
	p := h.Homes.Project
	testutil.WriteFile(t, filepath.Join(p, ".claude", "skills", "claude-only", "SKILL.md"),
		[]byte("---\nname: claude-only\ndescription: Helps with local migrations.\n---\n\n# Body\n"))

	agent, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)

	h.mustTmux("set-option", "-w", "-t", bar, "@imux_view", "skills")
	_, _, _ = h.imux("__sidebar", "--signal")
	h.rowOf(bar, "claude-only")
	if s := h.screen(bar); !strings.Contains(s, "JOBS") || !strings.Contains(s, "Helps with local migrations.") {
		t.Fatalf("skills in Jobs:\n%s", s)
	}

	// Share with all agents moves it into .agents/skills and links .claude/skills.
	x, y := h.rowOf(bar, "claude-only")
	time.Sleep(400 * time.Millisecond)
	mark := c.Mark()
	c.Click(2, x, y)
	c.waitScreenAfter(h, mark, "Share with all agents", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	c.Write("g")
	h.waitFor("shared with all agents", 5*time.Second, func() bool {
		_, err := os.Stat(filepath.Join(p, ".agents", "skills", "claude-only", "SKILL.md"))
		return err == nil
	})
	if target, err := os.Readlink(filepath.Join(p, ".claude", "skills", "claude-only")); err != nil || !strings.HasSuffix(target, "claude-only") {
		t.Fatalf("claude link: %q %v", target, err)
	}

	// ✨ New skill… asks who writes the skill.
	newSkill := func() {
		t.Helper()
		time.Sleep(400 * time.Millisecond)
		x, y := h.buttonOf(bar, "JOBS", "✨")
		mark := c.Mark()
		c.Click(0, x+1, y)
		c.waitScreenAfter(h, mark, "Who should write it?", 5*time.Second)
		time.Sleep(200 * time.Millisecond)
	}

	// t: a blank SKILL.md, named in the dialog, created in .agents/skills,
	// linked for Claude and opened in the editor.
	newSkill()
	mark = c.Mark()
	c.Write("t")
	c.waitScreenAfter(h, mark, "Skill name", 5*time.Second)
	mark = c.Mark()
	c.Write("Release Notes")
	c.waitScreenAfter(h, mark, ".agents/skills/release-notes/SKILL.md", 5*time.Second)
	mark = c.Mark()
	c.Write("\r")
	c.waitScreenAfter(h, mark, "EDIT .agents/skills/release-notes/SKILL.md", 5*time.Second)
	c.Write("\r")
	h.rowOf(bar, "release-notes")
	if _, err := os.Lstat(filepath.Join(p, ".claude", "skills", "release-notes")); err != nil {
		t.Fatalf("no claude link for release-notes: %v", err)
	}

	// h: this pane's agent; nothing described, so the guide tells it to ask.
	newSkill()
	mark = c.Mark()
	c.Write("h")
	c.waitScreenAfter(h, mark, "What should the skill do?", 5*time.Second)
	c.Write("\r")
	h.waitFor("write prompt sent to claude", 5*time.Second, func() bool {
		s := h.stdinOf("claude", 1)
		return strings.Contains(s, "Create an Agent Skill for this project") &&
			strings.Contains(s, ".agents/skills/<name>/SKILL.md") &&
			strings.Contains(s, "First ask me, in English and in one short question")
	})

	// Improve with Agent sends the skill's path to the active agent.
	x, y = h.rowOf(bar, "release-notes")
	time.Sleep(400 * time.Millisecond)
	mark = c.Mark()
	c.Click(2, x, y)
	c.waitScreenAfter(h, mark, "Improve with Agent", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	c.Write("i")
	h.waitFor("improve prompt sent to claude", 5*time.Second, func() bool {
		s := h.stdinOf("claude", 1)
		return strings.Contains(s, "Improve the Agent Skill at") && strings.Contains(s, "release-notes/SKILL.md")
	})

	// Enter: the current pane's agent (Claude) in a new pane beside it, with
	// the guide and the description as its first message.
	panes := len(strings.Fields(h.mustTmux("list-panes", "-t", agent, "-F", "#{pane_id}")))
	newSkill()
	mark = c.Mark()
	c.Write("\r")
	c.waitScreenAfter(h, mark, "Claude · new pane", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	c.Write("write changelog entries\r")
	argv := strings.Join(h.run("claude", 2), "\n")
	for _, want := range []string{"Create an Agent Skill for this project", ".agents/skills/<name>/SKILL.md", "What the skill should do:\nwrite changelog entries"} {
		if !strings.Contains(argv, want) {
			t.Errorf("new pane's first message lacks %q:\n%s", want, argv)
		}
	}
	h.waitFor("the skill writer's pane, named", 5*time.Second, func() bool {
		out := strings.TrimSpace(h.mustTmux("list-panes", "-t", agent, "-F", "#{pane_id}\t#{@imux_name}"))
		return strings.Count(out, "\n")+1 == panes+1 && strings.Contains(out, "\tnew skill")
	})
	if strings.Contains(h.stdinOf("claude", 1), "write changelog entries") {
		t.Error("the description went to the first pane too")
	}

	// Delete moves the skill to Trash and removes its .claude/skills link.
	x, y = h.rowOf(bar, "release-notes")
	time.Sleep(400 * time.Millisecond)
	mark = c.Mark()
	c.Click(2, x, y)
	c.waitScreenAfter(h, mark, "Delete skill…", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	c.Write("d")
	c.waitScreenAfter(h, mark, "Delete 'release-notes'?", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	c.Write("y")
	h.waitFor("release-notes deleted", 5*time.Second, func() bool {
		_, err1 := os.Lstat(filepath.Join(p, ".agents", "skills", "release-notes"))
		_, err2 := os.Lstat(filepath.Join(p, ".claude", "skills", "release-notes"))
		return os.IsNotExist(err1) && os.IsNotExist(err2)
	})
}
