package sidebar

import (
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/gitops"
	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
)

func scmModel() Model {
	m := model()
	m.View = ViewChanges
	m.SCM = &gitops.Status{
		Branch:    "main",
		Conflicts: []gitops.Change{{Status: "U", Path: "internal/api/api.go"}},
		Staged:    []gitops.Change{{Status: "M", Path: "internal/api/a.go", Added: 3}},
		Changes: []gitops.Change{
			{Status: "M", Path: "internal/api/b.go", Added: 1},
			{Status: "D", Path: "docs/x.md"},
			{Status: "?", Path: "new.txt", Untracked: true},
		},
		Graph: []gitops.GraphRow{
			{Lanes: "●", Hash: "1a2b3c4", Subject: "mine", When: m.Now.Add(-time.Hour), Out: true, Refs: []gitops.Ref{{Name: "main", Kind: gitops.RefHead}}},
			{Lanes: "│ ●", Hash: "5d6e7f8", Subject: "theirs", When: m.Now.Add(-2 * time.Hour), In: true, Refs: []gitops.Ref{{Name: "origin/main", Kind: gitops.RefRemote}, {Name: "v1", Kind: gitops.RefTag}}},
			{Lanes: "│╱"},
			{Lanes: "●", Hash: "9a8b7c6", Subject: "init", When: m.Now.Add(-48 * time.Hour)},
		},
	}
	return m
}

// lineWith is the rendered line holding sub, and the column sub starts at.
func lineWith(t *testing.T, sc Screen, sub string) (Line, int) {
	t.Helper()
	for _, l := range sc.Lines {
		p := Plain(l.Text)
		if i := strings.Index(p, sub); i >= 0 {
			return l, DisplayWidth(p[:i])
		}
	}
	t.Fatalf("no %q in:\n%s", sub, screenText(sc))
	return Line{}, 0
}

// TestSourceControlLikeVSCode (T63.4): ⋯ and ▾, Publish without an
// upstream, MERGE CHANGES with !, U for untracked files, the graph.
func TestSourceControlLikeVSCode(t *testing.T) {
	i18n.SetLocale("en")
	m := scmModel()
	sc := Render(m, 60, 0, NoItem)
	text := screenText(sc)
	t.Logf("\n%s", text)
	for _, want := range []string{
		"SOURCE CONTROL", "⋯", "📤", "Message (Enter to commit)", "✅ Commit", "▾ MERGE CHANGES  1", "!  api.go", "U  new.txt", "D  x.md",
		"● ↑ 1a2b3c4 (main) mine", "│ ● ↓ 5d6e7f8 (origin/main, v1) theirs", "│╱", "▾ HISTORY  3",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("lacks %q", want)
		}
	}
	if strings.Contains(text, "🔃") {
		t.Error("Sync shown without an upstream")
	}
	click := func(sub string, dx int) Action {
		l, x := lineWith(t, sc, sub)
		return l.ActionAt(x + dx)
	}
	for sub, want := range map[string]Action{
		"⋯":         scmAction("menu", "more"),
		"📤":         scmAction("pick", "publish"),
		"🌿 main":    scmAction("pick", "checkout"),
		"api.go":    {Kind: ActDiff, Path: "internal/api/api.go", Status: "U", Section: "merge"},
		"MERGE":     {Kind: ActFold, Section: SecMerge},
		"1a2b3c4":   {Kind: ActGitPopup, Section: "show", Path: "1a2b3c4"},
		"✅ Commit":  {Kind: ActCommit},
		"Message (": {Kind: ActCommitBox, Section: "0:0"},
	} {
		if got := click(sub, 0); got != want {
			t.Errorf("%q: %+v, want %+v", sub, got, want)
		}
	}
	// ▾ is the commit button's dropdown, at the row's end; no ✨ (D65).
	l, _ := lineWith(t, sc, "✅ Commit")
	if got := l.ActionAt(58); got != scmAction("menu", "commit-button") {
		t.Errorf("▾: %+v", got)
	}
	if strings.Contains(text, "✨") {
		t.Error("✨ is back")
	}
	// MERGE CHANGES' + stages every conflict; a conflict's + stages it.
	l, _ = lineWith(t, sc, "MERGE CHANGES")
	if got := l.ActionAt(58); got != (Action{Kind: ActGit, Section: "stage-merge"}) {
		t.Errorf("stage merge %+v", got)
	}
	// During a merge Commit is Commit Merge: git's message, or the box's;
	// a rebase's, Continue.
	m.SCM.Op = gitops.OpMerge
	sc = Render(m, 60, 0, NoItem)
	if got := click("Commit Merge", 0); got != (Action{Kind: ActGitPopup, Section: "continue"}) {
		t.Errorf("commit merge %+v", got)
	}
	m.Commit.Text = []rune("Merge the fix")
	sc = Render(m, 60, 0, NoItem)
	if got := click("Commit Merge", 0); got != (Action{Kind: ActCommit}) {
		t.Errorf("commit merge with a message %+v", got)
	}
	m.Commit.Text = nil
	m.SCM.Op = gitops.OpRebase
	if text := screenText(Render(m, 50, 0, NoItem)); !strings.Contains(text, "✅ Continue") {
		t.Errorf("continue:\n%s", text)
	}
	// With an upstream: Sync, and the counts.
	m.SCM.Op, m.SCM.Upstream, m.SCM.Behind = "", "origin/main", 1
	if text := screenText(Render(m, 50, 0, NoItem)); !strings.Contains(text, "🔃") || strings.Contains(text, "📤") || !strings.Contains(text, "↑0 ↓1") {
		t.Errorf("upstream:\n%s", text)
	}
	// Narrow: no hash, and only the refs that fit.
	if text := screenText(Render(m, 43, 0, NoItem)); strings.Contains(text, "1a2b3c4") || !strings.Contains(text, "● ↑ (main) mine") ||
		!strings.Contains(text, "│ ● ↓ (origin/main) theirs") {
		t.Errorf("narrow graph:\n%s", text)
	}
}

// TestSourceControlTreeAndSort (T63.4): View as Tree joins folders with one
// folder in them, folds a folder per sidebar and puts its section's buttons
// on it; Sort by Name and Status reorder a list.
func TestSourceControlTreeAndSort(t *testing.T) {
	i18n.SetLocale("en")
	m := scmModel()
	m.SCMView = "tree"
	for _, l := range Render(m, 43, 30, NoItem).Lines {
		if n := DisplayWidth(Plain(l.Text)); n != 43 {
			t.Errorf("%d columns: %q", n, Plain(l.Text))
		}
	}
	sc := Render(m, 50, 0, NoItem)
	text := screenText(sc)
	t.Logf("\n%s", text)
	for _, want := range []string{"▾ internal/api", "M  b.go", "▾ docs", "D  x.md", "U  new.txt"} {
		if !strings.Contains(text, want) {
			t.Errorf("tree lacks %q", want)
		}
	}
	if strings.Contains(text, "▾ internal\n") || strings.Contains(text, "b.go internal") {
		t.Errorf("not compact:\n%s", text)
	}
	// The folder's row folds; its buttons act on the whole folder.
	// A file's row keeps its counts and buttons at the edge.
	l, _ := lineWith(t, sc, "M  b.go")
	if got := l.ActionAt(48); got != (Action{Kind: ActGit, Section: "stage", Path: "internal/api/b.go"}) || !strings.Contains(Plain(l.Text), "+1  ⏪  ➕ ") {
		t.Errorf("file row %q: %+v", Plain(l.Text), got)
	}
	l, x := lineWith(t, sc, "▾ docs")
	if got := l.ActionAt(x); got != (Action{Kind: ActSCMDir, Path: "worktree:docs"}) {
		t.Errorf("folder row %+v", got)
	}
	if got := l.ActionAt(48); got != (Action{Kind: ActGit, Section: "stage", Path: "docs"}) {
		t.Errorf("folder stage %+v", got)
	}
	if got := l.ActionAt(45); got != (Action{Kind: ActGitConfirm, Section: "discard-dir", Path: "docs"}) {
		t.Errorf("folder discard %+v", got)
	}
	m.SCMClosed = map[string]bool{"worktree:docs": true}
	if text := screenText(Render(m, 50, 0, NoItem)); strings.Contains(text, "x.md") || !strings.Contains(text, "▸ docs") {
		t.Errorf("folded folder:\n%s", text)
	}
	// Sorting a list.
	order := func(text string, names ...string) bool {
		last := -1
		for _, n := range names {
			i := strings.Index(text, n)
			if i < last {
				return false
			}
			last = i
		}
		return true
	}
	m.SCMView, m.SCMClosed = "", nil
	if text := screenText(Render(m, 50, 0, NoItem)); !order(text, "x.md", "b.go", "new.txt") {
		t.Errorf("by path:\n%s", text)
	}
	m.SCMSort = "name"
	if text := screenText(Render(m, 50, 0, NoItem)); !order(text, "b.go", "new.txt", "x.md") {
		t.Errorf("by name:\n%s", text)
	}
	m.SCMSort = "status"
	if text := screenText(Render(m, 50, 0, NoItem)); !order(text, "x.md", "b.go", "new.txt") {
		t.Errorf("by status (D, M, U):\n%s", text)
	}
}

// TestSourceControlNoRepo (T63.4): outside a repository, Initialize
// Repository and Clone Repository…, as VS Code's.
func TestSourceControlNoRepo(t *testing.T) {
	i18n.SetLocale("en")
	m := model()
	m.View, m.SCM, m.CWD = ViewChanges, nil, "/home/test/proj"
	sc := Render(m, 41, 0, NoItem)
	text := screenText(sc)
	if !strings.Contains(text, "Not a git repository") {
		t.Fatalf("no git:\n%s", text)
	}
	l, x := lineWith(t, sc, "Initialize Repository")
	if got := l.ActionAt(x); got != (Action{Kind: ActGit, Section: "init"}) {
		t.Errorf("init %+v", got)
	}
	l, x = lineWith(t, sc, "Clone Repository…")
	if got := l.ActionAt(x); got != scmAction("ask", "clone") {
		t.Errorf("clone %+v", got)
	}
}
