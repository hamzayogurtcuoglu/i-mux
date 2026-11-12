package scmui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/workspace"
)

// A folder and refs a shell or tmux would read as commands.
const evilDir = "/tmp/a b;#(touch x)$(y)"

// evilBits are pieces of the data above that no command may hold (the
// commands themselves have ";" and "&" on Windows, where PowerShell runs
// them).
var evilBits = []string{"a b;", "touch x", "$(y)", "#(x)"}

func testCtx() Ctx {
	return Ctx{Self: "/opt/imux", Dir: evilDir, Client: "/dev/pts/3", Pane: "%5", Sidebar: "%6", Now: time.Unix(1_800_000_000, 0)}
}

func allMenus(c Ctx) map[string]*workspace.Menu {
	st := &gitops.Status{Branch: "main", Upstream: "origin/main", Op: gitops.OpMerge}
	return map[string]*workspace.Menu{
		"more": More(c), "view": ViewSort(c, "", ""), "commit": Commit(c, st), "changes": Changes(c),
		"pullpush": PullPush(c), "branch": Branch(c, st), "remote": Remote(c), "stash": Stash(c, true),
		"tags": Tags(c), "worktrees": Worktrees(c), "commit-button": CommitButton(c, st),
		"file":          File(c, "a.go", SideWorktree, "M", false, "file.reveal_finder"),
		"file-new":      File(c, "n.txt", SideWorktree, "?", true, ""),
		"file-staged":   File(c, "a.go", SideStaged, "A", false, ""),
		"file-conflict": File(c, "a.go", SideMerge, "U", false, ""),
		"commit-row":    CommitRow(c, "1a2b3c4", "feat: #(x)"),
	}
}

var submenus = []string{"view", "commit", "changes", "pullpush", "branch", "remote", "stash", "tags", "worktrees"}

// TestMenusCarryNoData (T63.3): every item has a translated name and a key
// of its own, its command runs imux with fixed words and never the folder;
// a submenu starts with ‹ Back.
func TestMenusCarryNoData(t *testing.T) {
	for _, loc := range []string{"en", "tr"} {
		i18n.SetLocale(loc)
		c := testCtx()
		for name, m := range allMenus(c) {
			keys := map[string]bool{}
			for _, it := range m.Items {
				if it.Name == "" {
					continue
				}
				if strings.HasPrefix(it.Name, "scm.") || strings.Contains(it.Name, " scm.") || strings.HasPrefix(it.Name, "file.") {
					t.Errorf("%s %s: untranslated %q", loc, name, it.Name)
				}
				if it.Key == "" || keys[it.Key] {
					t.Errorf("%s %s: key %q of %q missing or taken", loc, name, it.Key, it.Name)
				}
				keys[it.Key] = true
				if it.Cmd == "" && !it.Disabled {
					t.Errorf("%s %s: %q does nothing", loc, name, it.Name)
				}
				for _, bit := range evilBits {
					if strings.Contains(it.Cmd, bit) {
						t.Errorf("%s %s: %q carries data: %s", loc, name, it.Name, it.Cmd)
					}
				}
				if it.Cmd != "" && !strings.Contains(it.Cmd, "/opt/imux") {
					t.Errorf("%s %s: %q runs no imux: %s", loc, name, it.Name, it.Cmd)
				}
			}
			if slices.Contains(submenus, name) {
				if b := m.Items[0]; !strings.HasPrefix(b.Name, "‹ ") || b.Key != "BSpace" || !strings.Contains(b.Cmd, "__scm menu more") {
					t.Errorf("%s %s: first item %+v is not ‹ Back", loc, name, b)
				}
			}
		}
	}
	i18n.SetLocale("en")
}

func entry(t *testing.T, m *workspace.Menu, name string) workspace.MenuItem {
	t.Helper()
	for _, it := range m.Items {
		if strings.TrimSpace(strings.TrimPrefix(it.Name, "●")) == name || it.Name == name {
			return it
		}
	}
	t.Fatalf("%s has no %q", m.Title, name)
	return workspace.MenuItem{}
}

func has(m *workspace.Menu, name string) bool {
	for _, it := range m.Items {
		if it.Name == name {
			return true
		}
	}
	return false
}

// TestMenuStates (T63.3): what VS Code disables or leaves out.
func TestMenuStates(t *testing.T) {
	i18n.SetLocale("en")
	c := testCtx()
	// No commit yet: nothing to undo or amend.
	m := Commit(c, &gitops.Status{Branch: "main", NoCommits: true})
	for _, n := range []string{"Undo Last Commit", "Commit (Amend)", "Commit Staged (Amend)", "Commit All (Amend)"} {
		if !entry(t, m, n).Disabled {
			t.Errorf("%q enabled without commits", n)
		}
	}
	if has(m, "Abort Merge") {
		t.Error("Abort without a merge")
	}
	// A merge or a rebase under way: its Abort.
	if it := entry(t, Commit(c, &gitops.Status{Branch: "main", Op: gitops.OpMerge}), "Abort Merge"); !strings.Contains(it.Cmd, "__git abort --scm") {
		t.Errorf("abort merge %+v", it)
	}
	if !has(Commit(c, &gitops.Status{Op: gitops.OpRebase}), "Abort Rebase") {
		t.Error("no Abort Rebase")
	}
	if !entry(t, Stash(c, false), "Stash Staged…").Disabled || entry(t, Stash(c, true), "Stash Staged…").Disabled {
		t.Error("Stash Staged needs git 2.35, and only that")
	}
	// Publish only without an upstream; Rename only on a branch.
	if !entry(t, Branch(c, &gitops.Status{Branch: "main", Upstream: "origin/main"}), "Publish Branch…").Disabled {
		t.Error("Publish with an upstream")
	}
	if entry(t, Branch(c, &gitops.Status{Branch: "main"}), "Publish Branch…").Disabled {
		t.Error("no Publish without an upstream")
	}
	detached := Branch(c, &gitops.Status{})
	if !entry(t, detached, "Rename Branch…").Disabled || !entry(t, detached, "Publish Branch…").Disabled {
		t.Error("Rename or Publish on a detached HEAD")
	}
	// View & Sort marks the current ones.
	vs := ViewSort(c, "tree", "status")
	for _, it := range vs.Items {
		on := strings.HasPrefix(it.Name, "● ")
		if want := it.Name == "● View as Tree" || it.Name == "● Sort by Status"; on != want && it.Name != "" && !strings.HasPrefix(it.Name, "‹") {
			t.Errorf("view & sort %q", it.Name)
		}
	}
	// A file's menu, by side.
	if f := File(c, "n.txt", SideWorktree, "?", true, ""); !entry(t, f, "Open File (HEAD)").Disabled || !has(f, "Add to .gitignore") ||
		!strings.Contains(entry(t, f, "Discard Changes…").Cmd, "--untracked") {
		t.Error("untracked file's menu")
	}
	if f := File(c, "a.go", SideStaged, "M", false, "file.reveal_finder"); !has(f, "Unstage Changes") || has(f, "Discard Changes…") ||
		has(f, "Add to .gitignore") || !has(f, "Reveal in Finder") {
		t.Error("staged file's menu")
	}
	if f := File(c, "a.go", SideMerge, "U", false, ""); !entry(t, f, "Open Changes").Disabled || !has(f, "Stage Changes (Resolved)") {
		t.Error("conflict's menu")
	}
	if it := entry(t, CommitRow(c, "1a2b3c4", "x"), "Cherry Pick"); !strings.Contains(it.Cmd, "__scm popup cherry-pick") ||
		!strings.Contains(it.Cmd, "--hash 1a2b3c4") {
		t.Errorf("cherry pick %+v", it)
	}
}

// TestPicksCarryDataAsArgv (T63.3): a ref, a remote, a stash or a worktree
// reaches imux as one argument after "--", whatever its characters.
func TestPicksCarryDataAsArgv(t *testing.T) {
	i18n.SetLocale("en")
	c := testCtx()
	when := c.Now.Add(-time.Hour)
	refs := []gitops.RefInfo{
		{Name: "main", Kind: gitops.RefBranch, Current: true, Hash: "1a2b3c4", Subject: "init", When: when},
		{Name: "feat/#(x)$y", Kind: gitops.RefBranch, Hash: "5d6e7f8"},
		{Name: "origin/main", Kind: gitops.RefRemote},
		{Name: "v1.0", Kind: gitops.RefTag},
	}
	flags := []string{"--client", "/dev/pts/3", "--pane", "%5", "--sidebar", "%6"}
	argv := func(words []string, data ...string) []string {
		a := append(append(append([]string{}, words...), "--cwd", evilDir), flags...)
		if len(data) > 0 {
			a = append(append(a, "--"), data...)
		}
		return a
	}
	p := Checkout(c, refs)
	if len(p.Items) != 7 || p.Client != c.Client || p.Pane != c.Pane {
		t.Fatalf("checkout: %+v", p)
	}
	for i, want := range [][]string{
		argv([]string{"__scm", "ask", "create-branch"}),
		argv([]string{"__scm", "pick", "branch-from"}),
		argv([]string{"__scm", "pick", "checkout-detached"}),
		argv([]string{"__git", "checkout-ref", "--kind", gitops.RefBranch}, "main"),
		argv([]string{"__git", "checkout-ref", "--kind", gitops.RefBranch}, "feat/#(x)$y"),
		argv([]string{"__git", "checkout-ref", "--kind", gitops.RefRemote}, "origin/main"),
		argv([]string{"__git", "checkout-ref", "--kind", gitops.RefTag}, "v1.0"),
	} {
		if it := p.Items[i]; !slices.Equal(it.Argv, want) || it.Always != (i < 3) || it.Query != (i < 2) {
			t.Errorf("item %d %+v\nwant argv %q", i, it, want)
		}
	}
	if it := p.Items[3]; it.Label != "main  ✓" || it.Group != "branches" || it.Description != "1a2b3c4 · init · 1 h ago" {
		t.Errorf("current branch %+v", it)
	}
	if g := p.Items[5].Group + "," + p.Items[6].Group; g != "remote branches,tags" {
		t.Errorf("groups %q", g)
	}
	// Merge Branch… leaves the current branch out and shows git's output.
	m := RefPick(c, "Merge", refs, []string{gitops.RefBranch, gitops.RefRemote}, true, true, "__scm", "popup", "merge")
	if len(m.Items) != 2 || !m.Items[0].InPopup || !slices.Equal(m.Items[0].Argv, argv([]string{"__scm", "popup", "merge"}, "feat/#(x)$y")) {
		t.Errorf("merge pick %+v", m.Items)
	}
	r := RemotePick(c, "Remove", []gitops.Remote{{Name: "up;stream", FetchURL: "git@h:o/r.git"}}, false, "__scm", "confirm", "remove-remote")
	if len(r.Items) != 1 || r.Items[0].Description != "git@h:o/r.git" || !slices.Equal(r.Items[0].Argv, argv([]string{"__scm", "confirm", "remove-remote"}, "up;stream")) {
		t.Errorf("remote pick %+v", r.Items)
	}
	s := StashPick(c, "Pop", []gitops.Stash{{Ref: "stash@{0}", Message: "On main: wip", When: when}}, true, "__git", "stash-pop")
	if it := s.Items[0]; it.Label != "On main: wip" || it.Description != "stash@{0} · 1 h ago" || !slices.Equal(it.Argv, argv([]string{"__git", "stash-pop"}, "stash@{0}")) {
		t.Errorf("stash pick %+v", it)
	}
	ws := []gitops.Worktree{
		{Path: "/r", Branch: "main", Main: true},
		{Path: "/r.worktrees/cur", Branch: "cur", Current: true},
		{Path: "/r.worktrees/x", Branch: "feat/#(x)$y"},
		{Path: "/r.worktrees/d", Detached: true},
	}
	if w := WorktreePick(c, "Delete", ws, true, "__scm", "confirm", "worktree-remove"); len(w.Items) != 2 ||
		w.Items[0].Label != "/r.worktrees/x" || w.Items[1].Description != "(detached)" {
		t.Errorf("worktree pick %+v", w.Items)
	}
	if w := WorktreePick(c, "Open", ws, false, "__git", "worktree-open"); len(w.Items) != 3 || w.Items[0].Label != "/r" {
		t.Errorf("worktree open pick %+v", w.Items)
	}
	// Create Worktree… offers the branches no worktree has.
	wc := WorktreeCreate(c, refs, ws)
	if len(wc.Items) != 1 || !wc.Items[0].Always {
		t.Errorf("worktree create %+v", wc.Items)
	}
	ws = ws[:2]
	if wc = WorktreeCreate(c, refs, ws); len(wc.Items) != 2 || !slices.Equal(wc.Items[1].Argv, argv([]string{"__git", "worktree-add"}, "feat/#(x)$y")) {
		t.Errorf("worktree create %+v", wc.Items)
	}
	// An empty list says so.
	if e := RemotePick(c, "Pull from", nil, true, "__scm", "popup", "pull-from"); len(e.Items) != 0 || e.Empty != i18n.T("scm.pick_no_remotes") {
		t.Errorf("empty pick %+v", e)
	}
}

func TestValidHash(t *testing.T) {
	for h, want := range map[string]bool{
		"1a2b3c4": true, "0123456789abcdef0123456789abcdef01234567": true,
		"": false, "abc": false, "1A2B3C4": false, "1a2b;rm": false, "-1a2b3c4": false, "1a2b3c4 ": false,
	} {
		if ValidHash(h) != want {
			t.Errorf("ValidHash(%q) != %v", h, want)
		}
	}
}

// TestMenuFlags: a submenu, prompt or confirmation opens where its menu
// was (__scm gets --x and --y); __git and __file get only the flags they
// take.
func TestMenuFlags(t *testing.T) {
	i18n.SetLocale("en")
	c := testCtx()
	c.X, c.Y = 40, 3
	n := 0
	for name, m := range allMenus(c) {
		for _, it := range m.Items {
			switch {
			case strings.Contains(it.Cmd, " __scm "):
				n++
				if !strings.Contains(it.Cmd, "--x 40 --y 3") {
					t.Errorf("%s %q: not at the menu's place: %s", name, it.Name, it.Cmd)
				}
			case strings.Contains(it.Cmd, " __git "):
				if strings.Contains(it.Cmd, "--x") {
					t.Errorf("%s %q: %s", name, it.Name, it.Cmd)
				}
			case strings.Contains(it.Cmd, " __file "):
				if strings.Contains(it.Cmd, "--x") || strings.Contains(it.Cmd, "--sidebar") {
					t.Errorf("%s %q: __file takes --client and --pane only: %s", name, it.Name, it.Cmd)
				}
			}
		}
	}
	if n == 0 {
		t.Fatal("no __scm command seen")
	}
}
