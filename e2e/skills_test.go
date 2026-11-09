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

// TestSkillsView (M40): the Skills view lists project skills with their
// agent badges and problems, shares a single-agent skill with all agents,
// creates a template skill and opens it in the editor, sends a write-skill
// prompt and an improve-skill prompt to the active agent, and deletes a
// skill with its link.
func TestSkillsView(t *testing.T) {
	ed := filepath.Join(t.TempDir(), "fake-editor")
	testutil.WriteFile(t, ed, []byte("#!/bin/sh\necho \"EDIT $*\"\nread x\n"))
	if err := os.Chmod(ed, 0o755); err != nil {
		t.Fatal(err)
	}
	h := newH(t, "editor = \""+ed+"\"\n"+sidebarConfig)
	p := h.Homes.Project
	testutil.WriteFile(t, filepath.Join(p, ".claude", "skills", "claude-only", "SKILL.md"),
		[]byte("---\nname: claude-only\ndescription: Helps with local migrations.\n---\n\n# Body\n"))

	_, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)

	h.openView(c, bar, "🧩", "skills")
	h.rowOf(bar, "claude-only")
	if s := h.screen(bar); !strings.Contains(s, "✻·◉·▣") || !strings.Contains(s, "Helps with local migrations.") {
		t.Fatalf("skills screen:\n%s", s)
	}

	// Share with all agents moves it into .agents/skills and links .claude/skills.
	x, y := h.rowOf(bar, "claude-only")
	time.Sleep(400 * time.Millisecond)
	mark := c.Mark()
	c.Click(2, x, y)
	c.waitScreenAfter(h, mark, "Share with all agents", 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	c.Write("s")
	h.waitFor("shared with all agents", 5*time.Second, func() bool {
		return strings.Contains(h.screen(bar), "✻▲◉◎▣")
	})
	if _, err := os.Stat(filepath.Join(p, ".agents", "skills", "claude-only", "SKILL.md")); err != nil {
		t.Fatalf("not in .agents/skills: %v", err)
	}
	if target, err := os.Readlink(filepath.Join(p, ".claude", "skills", "claude-only")); err != nil || !strings.HasSuffix(target, "claude-only") {
		t.Fatalf("claude link: %q %v", target, err)
	}

	// + creates a template skill in .agents/skills, links .claude/skills, and opens the editor.
	x, y = h.rowOf(bar, "➕")
	time.Sleep(400 * time.Millisecond)
	mark = c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "Skill name", 5*time.Second)
	mark = c.Mark()
	c.Write("Release Notes\r")
	c.waitScreenAfter(h, mark, "EDIT .agents/skills/release-notes/SKILL.md", 5*time.Second)
	c.Write("\r")
	h.rowOf(bar, "release-notes")
	if _, err := os.Lstat(filepath.Join(p, ".claude", "skills", "release-notes")); err != nil {
		t.Fatalf("no claude link for release-notes: %v", err)
	}

	// Write with Agent… asks what the skill should do and sends the prompt to the active agent.
	x, y = h.rowOf(bar, "Write with Agent")
	time.Sleep(400 * time.Millisecond)
	mark = c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "What should this skill teach the agent?", 5*time.Second)
	c.Write("write changelog entries\r")
	h.waitFor("write prompt sent to claude", 5*time.Second, func() bool {
		b, _ := os.ReadFile(filepath.Join(h.Log, "claude-1.stdin"))
		return strings.Contains(string(b), "What it should do: write changelog entries") &&
			strings.Contains(string(b), ".agents/skills/<name>/SKILL.md")
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
		b, _ := os.ReadFile(filepath.Join(h.Log, "claude-1.stdin"))
		return strings.Contains(string(b), "Improve the Agent Skill at") &&
			strings.Contains(string(b), "release-notes/SKILL.md")
	})

	// Delete moves the skill to Trash and removes its .claude/skills link.
	time.Sleep(400 * time.Millisecond)
	mark = c.Mark()
	c.Click(2, x, y)
	c.waitScreenAfter(h, mark, "Delete", 5*time.Second)
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
