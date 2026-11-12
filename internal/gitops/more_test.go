package gitops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// g runs git in dir for a test.
func g(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := git(context.Background(), dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return out
}

// withRemote makes dir's main track a bare "origin" holding the same
// commit, and returns the bare repository's folder.
func withRemote(t *testing.T, dir string) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "origin.git")
	g(t, dir, "init", "-q", "--bare", "-b", "main", bare)
	g(t, dir, "remote", "add", "origin", bare)
	g(t, dir, "push", "-q", "-u", "origin", "main")
	return bare
}

func TestParseGraph(t *testing.T) {
	out := "*   \x001111111aaaa\x1f1111111\x1fMerge b\x1fann\x1f100\x1fHEAD -> refs/heads/main, tag: refs/tags/v1, refs/remotes/origin/main, refs/remotes/origin/HEAD\n" +
		"|\\  \n" +
		"| * \x002222222bbbb\x1f2222222\x1fon b\x1fbob\x1f90\x1frefs/heads/b\n" +
		"* | \x003333333cccc\x1f3333333\x1fon main\x1fann\x1f80\x1f\n" +
		"|/  \n" +
		"* \x004444444dddd\x1f4444444\x1finit\x1fann\x1f70\x1f\n"
	rows := ParseGraph(out)
	var lanes []string
	for _, r := range rows {
		lanes = append(lanes, r.Lanes+"|"+r.Hash)
	}
	if got := strings.Join(lanes, ","); got != "●|1111111,│╲|,│ ●|2222222,● │|3333333,│╱|,●|4444444" {
		t.Fatalf("lanes %s", got)
	}
	want := []Ref{{"main", RefHead}, {"v1", RefTag}, {"origin/main", RefRemote}}
	if r := rows[0]; len(r.Refs) != 3 || r.Refs[0] != want[0] || r.Refs[1] != want[1] || r.Refs[2] != want[2] || r.Author != "ann" || r.When.Unix() != 100 {
		t.Fatalf("first row %+v", r)
	}
	if rows[2].Refs[0] != (Ref{"b", RefBranch}) || rows[1].Hash != "" {
		t.Fatalf("rows %+v", rows)
	}
	MarkInOut(rows, "<1111111aaaa\n>2222222bbbb\n")
	if !rows[0].Out || rows[0].In || !rows[2].In || rows[3].Out {
		t.Fatalf("in/out %+v", rows)
	}
	if r := parseRefs("HEAD"); len(r) != 1 || r[0] != (Ref{"HEAD", RefHead}) {
		t.Fatalf("detached %+v", r)
	}
}

// TestGraphUpstream: the graph has the upstream's commits too, marks what
// is not pushed and what is incoming, and is read again when a ref moves.
func TestGraphUpstream(t *testing.T) {
	ctx := context.Background()
	dir := repo(t)
	bare := withRemote(t, dir)
	// An incoming commit: made in another clone, fetched here.
	other := filepath.Join(t.TempDir(), "other")
	g(t, dir, "clone", "-q", bare, other)
	write(t, other, "c.go", "c\n")
	g(t, other, "add", "-A")
	g(t, other, "commit", "-q", "-m", "theirs")
	g(t, other, "push", "-q")
	g(t, dir, "fetch", "-q")
	// An outgoing one.
	write(t, dir, "a.go", "mine\n")
	g(t, dir, "commit", "-q", "-am", "mine")
	g(t, dir, "tag", "v1")
	st, err := Read(ctx, dir)
	if err != nil || st.Ahead != 1 || st.Behind != 1 {
		t.Fatalf("status %+v %v", st, err)
	}
	subjects := map[string]GraphRow{}
	for _, r := range st.Graph {
		if r.Hash != "" {
			subjects[r.Subject] = r
		}
	}
	if !subjects["mine"].Out || subjects["mine"].In || !subjects["theirs"].In || subjects["init"].Out || subjects["init"].In {
		t.Fatalf("in/out %+v", st.Graph)
	}
	if refs := subjects["mine"].Refs; len(refs) != 2 || refs[0] != (Ref{"main", RefHead}) || refs[1] != (Ref{"v1", RefTag}) {
		t.Fatalf("refs of mine %+v", refs)
	}
	if refs := subjects["theirs"].Refs; len(refs) != 1 || refs[0] != (Ref{"origin/main", RefRemote}) {
		t.Fatalf("refs of theirs %+v", refs)
	}
	// A new tag moves no commit, but the graph shows it at once.
	g(t, dir, "tag", "v2", "HEAD~")
	st, _ = Read(ctx, dir)
	for _, r := range st.Graph {
		if r.Subject == "init" && (len(r.Refs) != 1 || r.Refs[0].Name != "v2") {
			t.Fatalf("v2 not shown: %+v", r)
		}
	}
}

func TestRefsStashesRemotesWorktrees(t *testing.T) {
	ctx := context.Background()
	dir := repo(t)
	withRemote(t, dir)
	g(t, dir, "branch", "feature/#H$(x)")
	g(t, dir, "tag", "-a", "-m", "release one", "v1")
	rs, err := Refs(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]RefInfo{}
	for _, r := range rs {
		kinds[r.Kind+":"+r.Name] = r
	}
	if len(rs) != 4 || !kinds["branch:main"].Current || kinds["branch:main"].Upstream != "origin/main" ||
		kinds["branch:feature/#H$(x)"].Subject != "init" || kinds["remote:origin/main"].Hash == "" || kinds["tag:v1"].Subject != "release one" {
		t.Fatalf("refs %+v", rs)
	}

	// Stashes: with a message and untracked files; none to stash.
	write(t, dir, "a.go", "changed\n")
	write(t, dir, "new.txt", "n\n")
	if err := StashPush(ctx, dir, "  my work ", true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); !os.IsNotExist(err) {
		t.Fatal("untracked file not stashed")
	}
	if err := StashPush(ctx, dir, "", false, false); !errors.Is(err, ErrNoChanges) {
		t.Fatalf("empty stash: %v", err)
	}
	ss, err := Stashes(ctx, dir)
	if err != nil || len(ss) != 1 || ss[0].Ref != "stash@{0}" || ss[0].Message != "On main: my work" {
		t.Fatalf("stashes %+v %v", ss, err)
	}
	if err := StashApply(ctx, dir, ""); err != nil {
		t.Fatal(err)
	}
	if ss, _ := Stashes(ctx, dir); len(ss) != 1 {
		t.Fatal("apply dropped the stash")
	}
	if err := StashClear(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if ss, _ := Stashes(ctx, dir); len(ss) != 0 {
		t.Fatal("stash clear")
	}
	if AtLeast(ctx, 2, 35) {
		g(t, dir, "add", "a.go")
		if err := StashPush(ctx, dir, "staged", false, true); err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(filepath.Join(dir, "new.txt")); string(b) != "n\n" {
			t.Fatal("stash staged took the untracked file")
		}
		if err := StashPop(ctx, dir, "stash@{0}"); err != nil {
			t.Fatal(err)
		}
	}

	// Remotes.
	if err := AddRemote(ctx, dir, "up", "https://example.com/x.git"); err != nil {
		t.Fatal(err)
	}
	rems, _ := Remotes(ctx, dir)
	if len(rems) != 2 || rems[1].Name != "up" || rems[1].FetchURL != "https://example.com/x.git" || rems[1].PushURL != rems[1].FetchURL {
		t.Fatalf("remotes %+v", rems)
	}
	if err := RemoveRemote(ctx, dir, "up"); err != nil {
		t.Fatal(err)
	}

	// Worktrees: next to the project, listed, removed (forced with changes).
	top, _ := TopLevel(ctx, dir)
	wt := WorktreePath(top, "wt/one")
	if filepath.Base(filepath.Dir(filepath.Dir(wt))) != filepath.Base(top)+".worktrees" {
		t.Fatalf("worktree path %s", wt)
	}
	if err := AddWorktree(ctx, dir, wt, "wt/one", true); err != nil {
		t.Fatal(err)
	}
	ws, err := Worktrees(ctx, dir)
	if err != nil || len(ws) != 2 || !ws[0].Main || !ws[0].Current || ws[1].Branch != "wt/one" || ws[1].Current {
		t.Fatalf("worktrees %+v %v", ws, err)
	}
	write(t, ws[1].Path, "dirty.txt", "x")
	if err := RemoveWorktree(ctx, dir, ws[1].Path, false); err == nil {
		t.Fatal("removed a worktree with changes")
	}
	if err := RemoveWorktree(ctx, dir, ws[1].Path, true); err != nil {
		t.Fatal(err)
	}
	if subject, msg, err := LastCommit(ctx, dir); err != nil || subject != "init" || msg != "init" {
		t.Fatalf("last commit %q %q %v", subject, msg, err)
	}
}

func TestCommitWith(t *testing.T) {
	ctx := context.Background()
	dir := repo(t)
	write(t, dir, "a.go", "changed\n")
	if _, err := CommitWith(ctx, dir, CommitOpts{Message: "x", Mode: CommitStaged}); !errors.Is(err, ErrNothingStaged) {
		t.Fatalf("commit staged with nothing staged: %v", err)
	}
	write(t, dir, "new.go", "n\n")
	if staged, err := CommitWith(ctx, dir, CommitOpts{Message: "all", Mode: CommitAll, SignOff: true}); err != nil || !staged {
		t.Fatalf("commit all %v %v", staged, err)
	}
	if st, _ := Read(ctx, dir); len(st.Changes)+len(st.Staged) != 0 {
		t.Fatalf("left %+v", st)
	}
	if _, msg, _ := LastCommit(ctx, dir); !strings.Contains(msg, "Signed-off-by: t <t@example.com>") {
		t.Fatalf("no sign-off: %q", msg)
	}
	// Amend: a new message, then "" keeps it (body included).
	g(t, dir, "commit", "-q", "--amend", "-m", "subject", "-m", "the body")
	write(t, dir, "b.go", "more\n")
	g(t, dir, "add", "b.go")
	if _, err := CommitWith(ctx, dir, CommitOpts{Mode: CommitStaged, Amend: true}); err != nil {
		t.Fatal(err)
	}
	if s, msg, _ := LastCommit(ctx, dir); s != "subject" || msg != "subject\n\nthe body" {
		t.Fatalf("amend kept %q %q", s, msg)
	}
	if n := strings.Count(g(t, dir, "log", "--format=%h"), "\n"); n != 2 {
		t.Fatalf("%d commits after amending", n)
	}
	// Undo: the commit's changes come back staged; the first commit too.
	subject, err := UndoLastCommit(ctx, dir)
	if err != nil || subject != "subject" {
		t.Fatalf("undo %q %v", subject, err)
	}
	st, _ := Read(ctx, dir)
	if names(st.Staged) != "M a.go, M b.go, A new.go" || st.Graph[0].Subject != "init" {
		t.Fatalf("after undo %+v", st)
	}
	if _, err := UndoLastCommit(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if st, _ := Read(ctx, dir); !st.NoCommits || len(st.Graph) != 0 || names(st.Staged) != "A a.go, A b.go, A new.go" {
		t.Fatalf("after undoing the first commit %+v", st)
	}
}

// TestOperations: a merge and a rebase that stop on a conflict show it and
// its file, and abort.
func TestOperations(t *testing.T) {
	ctx := context.Background()
	dir := repo(t)
	g(t, dir, "switch", "-q", "-c", "other")
	write(t, dir, "a.go", "theirs\n")
	g(t, dir, "commit", "-q", "-am", "theirs")
	g(t, dir, "switch", "-q", "main")
	write(t, dir, "a.go", "ours\n")
	g(t, dir, "commit", "-q", "-am", "ours")
	if _, err := git(ctx, dir, "merge", "other"); err == nil {
		t.Fatal("no conflict")
	}
	st, _ := Read(ctx, dir)
	if st.Op != OpMerge || names(st.Conflicts) != "U a.go" || len(st.Changes) != 0 {
		t.Fatalf("merge %+v", st)
	}
	if err := Abort(ctx, dir, st.Op); err != nil {
		t.Fatal(err)
	}
	if _, err := git(ctx, dir, "rebase", "other"); err == nil {
		t.Fatal("no rebase conflict")
	}
	if st, _ := Read(ctx, dir); st.Op != OpRebase || len(st.Conflicts) != 1 {
		t.Fatalf("rebase %+v", st)
	}
	// Resolved and staged, Continue (as the popup runs it) finishes.
	write(t, dir, "a.go", "both\n")
	g(t, dir, "add", "a.go")
	if _, _, err := gitOut(ctx, dir, []string{"GIT_EDITOR=true"}, ContinueArgs(OpRebase)...); err != nil {
		t.Fatal(err)
	}
	if st, _ := Read(ctx, dir); st.Op != "" || st.Graph[0].Subject != "ours" {
		t.Fatalf("after continue %+v", st)
	}
	if err := Abort(ctx, dir, ""); err == nil {
		t.Fatal("aborted nothing")
	}
}

func TestCheckoutBranchesTags(t *testing.T) {
	ctx := context.Background()
	dir := repo(t)
	bare := withRemote(t, dir)
	other := filepath.Join(t.TempDir(), "other")
	g(t, dir, "clone", "-q", bare, other)
	g(t, other, "push", "-q", "origin", "main:feature")
	g(t, dir, "fetch", "-q")
	// A remote branch: a local one tracking it, then switched to.
	if err := CheckoutRef(ctx, dir, "origin/feature", RefRemote); err != nil {
		t.Fatal(err)
	}
	if st, _ := Read(ctx, dir); st.Branch != "feature" || st.Upstream != "origin/feature" {
		t.Fatalf("tracking %+v", st)
	}
	g(t, dir, "switch", "-q", "main")
	if err := CheckoutRef(ctx, dir, "origin/feature", RefRemote); err != nil {
		t.Fatal(err)
	}
	g(t, dir, "tag", "v1")
	if err := CheckoutRef(ctx, dir, "v1", RefTag); err != nil {
		t.Fatal(err)
	}
	if st, _ := Read(ctx, dir); st.Branch != "" {
		t.Fatalf("not detached: %+v", st)
	}
	g(t, dir, "switch", "-q", "main")
	// Create from a ref, rename, delete (not merged: forced).
	if err := CreateBranch(ctx, dir, "from-tag", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := RenameBranch(ctx, dir, "renamed"); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "x.go", "x\n")
	g(t, dir, "add", "-A")
	g(t, dir, "commit", "-q", "-m", "only here")
	g(t, dir, "switch", "-q", "main")
	if err := DeleteBranch(ctx, dir, "renamed", false); !errors.Is(err, ErrNotMerged) {
		t.Fatalf("delete unmerged: %v", err)
	}
	if err := DeleteBranch(ctx, dir, "renamed", true); err != nil {
		t.Fatal(err)
	}
	if err := CreateTag(ctx, dir, "v2", "second", ""); err != nil {
		t.Fatal(err)
	}
	if out := g(t, dir, "cat-file", "-t", "v2"); strings.TrimSpace(out) != "tag" {
		t.Fatalf("v2 is %s, not annotated", out)
	}
	if err := DeleteTag(ctx, dir, "v2"); err != nil {
		t.Fatal(err)
	}
}

func TestIgnoreAndLog(t *testing.T) {
	ctx := context.Background()
	dir := repo(t)
	LogPath = filepath.Join(t.TempDir(), "git.log")
	defer func() { LogPath = "" }()
	write(t, dir, ".gitignore", "*.tmp") // no newline at the end
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "sub"), "x.txt", "x")
	for range 2 {
		if err := Ignore(ctx, filepath.Join(dir, "sub"), "x.txt"); err != nil {
			t.Fatal(err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, ".gitignore")); string(b) != "*.tmp\nsub/x.txt\n" {
		t.Fatalf(".gitignore %q", b)
	}
	if err := StageAll(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if err := DeleteBranch(ctx, dir, "nope", false); err == nil {
		t.Fatal("deleted a missing branch")
	}
	b, _ := os.ReadFile(LogPath)
	if !strings.Contains(string(b), "> git add -A\n") || !strings.Contains(string(b), "> git branch -d -- nope\n! ") {
		t.Fatalf("log:\n%s", b)
	}
	// The log keeps its newer half past its size.
	big := strings.Repeat("y", 1000)
	for range 300 {
		Log(dir, []string{"status", big}, "", nil)
	}
	if fi, _ := os.Stat(LogPath); fi.Size() > logMax {
		t.Fatalf("log is %d bytes", fi.Size())
	}
}

func TestInit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(dir, ".gitconfig-none"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	if _, err := Read(context.Background(), dir); err == nil {
		t.Fatal("read a folder outside git")
	}
	if err := Init(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if st, err := Read(context.Background(), dir); err != nil || !st.NoCommits {
		t.Fatalf("after init %+v %v", st, err)
	}
}

// TestSaid: the log keeps a progress line as it ended, without colors or
// blank lines.
func TestSaid(t *testing.T) {
	in := "remote: Counting objects:  50% (1/2)\rremote: Counting objects: 100% (2/2), done.\r\n" +
		"\x1b[32mTo git@h:o/r.git\x1b[m\n\n   1a2b3c4..5d6e7f8  main -> main\n"
	want := "remote: Counting objects: 100% (2/2), done.\nTo git@h:o/r.git\n   1a2b3c4..5d6e7f8  main -> main"
	if got := said(in); got != want {
		t.Errorf("said:\n%q\nwant\n%q", got, want)
	}
}
