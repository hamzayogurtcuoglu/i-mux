package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// canSymlink reports whether this process may create symlinks in dir
// (Windows allows it only in Developer Mode or elevated).
func canSymlink(dir string) bool {
	link := filepath.Join(dir, ".symlink-probe")
	if err := os.Symlink(".", link); err != nil {
		return false
	}
	_ = os.Remove(link)
	return true
}

func skill(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n# Body\n"
}

func find(t *testing.T, list []Skill, folder string) Skill {
	t.Helper()
	for _, s := range list {
		if s.Folder == folder {
			return s
		}
	}
	t.Fatalf("no skill %q in %+v", folder, list)
	return Skill{}
}

// TestListAndSync: skills in .agents/skills get a link in .claude/skills and
// every agent sees them; one only in .claude/skills is seen by the agents
// that read that folder; problems are found.
func TestListAndSync(t *testing.T) {
	root := t.TempDir()
	if !canSymlink(root) {
		t.Skip("Sync links skills with symlinks, which need Developer Mode or elevation on Windows")
	}
	write(t, filepath.Join(root, ".git", "HEAD"), "ref: refs/heads/main\n")
	write(t, filepath.Join(root, Home, "release-notes", File), skill("release-notes", "Writes release notes. Use when tagging a release."))
	write(t, filepath.Join(root, Home, "broken", File), "# no front matter\n")
	write(t, filepath.Join(root, Home, "other-name", File), skill("something", "x"))
	write(t, filepath.Join(root, ClaudeDir, "claude-only", File), "---\nname: claude-only\ndescription: >\n  Folded\n  text.\n---\n")
	if err := os.MkdirAll(filepath.Join(root, Home, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	sub := filepath.Join(root, "src", "deep")
	_ = os.MkdirAll(sub, 0o755)
	if got := Root(sub); got != root {
		t.Fatalf("Root = %q", got)
	}

	linked := Sync(root)
	if strings.Join(linked, ",") != "broken,other-name,release-notes" {
		t.Fatalf("Sync linked %v", linked)
	}
	if target, err := os.Readlink(filepath.Join(root, ClaudeDir, "release-notes")); err != nil || target != filepath.Join("..", "..", Home, "release-notes") {
		t.Fatalf("link %q %v", target, err)
	}
	if again := Sync(root); len(again) != 0 {
		t.Fatalf("second Sync linked %v", again)
	}

	list := List(root)
	rn := find(t, list, "release-notes")
	if !rn.Shared || rn.Place != Home || rn.Problem != ProbNone || len(rn.Agents) != 5 {
		t.Errorf("release-notes: %+v", rn)
	}
	co := find(t, list, "claude-only")
	if co.Shared || co.Description != "Folded text." || !co.Agents[session.Claude] || co.Agents[session.Antigravity] || co.Agents[session.Codex] {
		t.Errorf("claude-only: %+v", co)
	}
	if p := find(t, list, "broken").Problem; p != ProbNoHeader {
		t.Errorf("broken: %q", p)
	}
	if p := find(t, list, "other-name").Problem; p != ProbNameFolder {
		t.Errorf("other-name: %q", p)
	}
	if p := find(t, list, "empty"); p.Problem != ProbNoFile || len(p.Agents) != 0 {
		t.Errorf("empty: %+v", p)
	}
	if list[0].Name > list[len(list)-1].Name {
		t.Errorf("not sorted: %+v", list)
	}

	// Share moves it into .agents/skills and links it back.
	dst, err := Share(root, co.Dir)
	if err != nil || dst != filepath.Join(root, Home, "claude-only") {
		t.Fatalf("Share = %q, %v", dst, err)
	}
	if co = find(t, List(root), "claude-only"); !co.Shared || len(co.Agents) != 5 {
		t.Errorf("shared: %+v", co)
	}
	if links := Links(root, dst); len(links) != 1 || links[0] != filepath.Join(root, ClaudeDir, "claude-only") {
		t.Errorf("Links = %v", links)
	}
}

func TestCreate(t *testing.T) {
	root := t.TempDir()
	p, err := Create(root, "deploy-staging", "Deploys to staging.\nUse before a release.")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(b), "---\nname: deploy-staging\ndescription: Deploys to staging. Use before a release.\n---\n\n# Deploy staging\n") {
		t.Fatalf("template:\n%s", b)
	}
	if s := find(t, List(root), "deploy-staging"); s.Problem != ProbNone || !s.Shared && canSymlink(t.TempDir()) {
		t.Errorf("%+v", s)
	}
	if _, err := Create(root, "deploy-staging", ""); err != ErrExists {
		t.Errorf("again: %v", err)
	}
	if _, err := Create(root, "Bad Name", ""); err == nil {
		t.Error("an invalid name was taken")
	}
}

func TestNames(t *testing.T) {
	for in, want := range map[string]string{
		"Sürüm notları yaz":        "surum-notlari-yaz",
		"  Deploy -- to staging! ": "deploy-to-staging",
		"İOS build & test":         "ios-build-test",
		"???":                      "",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
	if len(Slug(strings.Repeat("abc ", 40))) > 64 {
		t.Error("slug too long")
	}
	for n, ok := range map[string]bool{"a": true, "pdf-tools": true, "a1-b2": true, "-a": false, "a-": false, "a--b": false, "Ab": false, "a_b": false, "": false} {
		if ValidName(n) != ok {
			t.Errorf("ValidName(%q) != %v", n, ok)
		}
	}
}

func TestPrompts(t *testing.T) {
	p := WritePrompt("/repo", "  write\r\nrelease notes\n", "Turkish")
	for _, want := range []string{filepath.Join("/repo", Home) + "/<name>/SKILL.md", "description", "1024", "What the skill should do:\nwrite\nrelease notes"} {
		if !strings.Contains(p, want) {
			t.Errorf("WritePrompt lacks %q:\n%s", want, p)
		}
	}
	// i-mux's guide comes first, the ask last.
	if !strings.HasPrefix(p, "Create an Agent Skill for this project (/repo).") || !strings.HasSuffix(p, "release notes") {
		t.Errorf("WritePrompt order:\n%s", p)
	}
	// Nothing asked: the agent asks first, in the user's language.
	p = WritePrompt("/repo", " \n", "Turkish")
	if !strings.Contains(p, "First ask me, in Turkish and in one short question, what the skill should do") || strings.Contains(p, "What the skill should do:") {
		t.Errorf("WritePrompt without an ask:\n%s", p)
	}
	if !strings.Contains(WritePrompt("/repo", "", ""), "in English and") {
		t.Error("WritePrompt without a language asks in English")
	}
	if !strings.Contains(ImprovePrompt("/repo/.agents/skills/x/SKILL.md"), "/repo/.agents/skills/x/SKILL.md") {
		t.Error("ImprovePrompt lacks the path")
	}
}
