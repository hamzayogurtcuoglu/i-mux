package gitops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// LogPath is the file the actions append the git commands they run to,
// with what git said (Show Git Output); "" logs nothing.
var LogPath string

// logMax bounds the log: past it, its older half goes.
const logMax = 256 << 10

var logMu sync.Mutex

// Log appends one command run in dir, its output and its error to LogPath.
func Log(dir string, args []string, output string, err error) {
	if LogPath == "" {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n> git %s\n", time.Now().Format("2006-01-02 15:04:05"), dir, strings.Join(args, " "))
	if out := strings.TrimRight(said(output), "\n"); out != "" {
		b.WriteString(out + "\n")
	}
	if err != nil {
		fmt.Fprintf(&b, "! %v\n", err)
	}
	b.WriteString("\n")
	f, ferr := os.OpenFile(LogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if ferr != nil {
		return
	}
	_, _ = f.WriteString(b.String())
	fi, _ := f.Stat()
	_ = f.Close()
	if fi != nil && fi.Size() > logMax {
		if data, err := os.ReadFile(LogPath); err == nil && len(data) > logMax/2 {
			keep := data[len(data)-logMax/2:]
			if i := strings.Index(string(keep), "\n\n"); i >= 0 {
				keep = keep[i+2:]
			}
			_ = os.WriteFile(LogPath, keep, 0o600)
		}
	}
}

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// said is git's output as the log keeps it: a progress line as it ended
// (what follows its last \r), without colors, no blank lines.
func said(out string) string {
	out = ansiRE.ReplaceAllString(out, "")
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		l = strings.TrimRight(l, "\r")
		if i := strings.LastIndex(l, "\r"); i >= 0 {
			l = l[i+1:]
		}
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	return strings.Join(lines, "\n")
}

// run runs an action's git command and logs it.
func run(ctx context.Context, dir string, args ...string) (string, error) {
	return runEnv(ctx, dir, nil, args...)
}

// LockWait is how long an action waits for another git's index lock (an
// agent's git, an editor's), as VS Code does.
var LockWait = 2 * time.Second

func runEnv(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	deadline := time.Now().Add(LockWait)
	for wait := 50 * time.Millisecond; ; wait *= 2 {
		out, errOut, err := gitOut(ctx, dir, env, args...)
		if err != nil && locked(errOut) && time.Now().Add(wait).Before(deadline) {
			select {
			case <-ctx.Done():
			case <-time.After(wait):
				continue
			}
		}
		if err == nil {
			Log(dir, args, out+errOut, nil)
		} else {
			Log(dir, args, out, err)
		}
		return out, err
	}
}

// locked reports git's failure to take .git/index.lock because another git
// holds it: git stops there, before it changed anything.
func locked(stderr string) bool {
	return strings.Contains(stderr, "index.lock") && strings.Contains(stderr, "File exists")
}

// Errors of the actions.
var (
	ErrNothingStaged = errors.New("there are no staged changes to commit")
	ErrNotMerged     = errors.New("the branch is not fully merged")
	ErrNoChanges     = errors.New("there are no changes to stash")
)

// Commit modes (CommitOpts.Mode).
const (
	CommitDefault = ""       // the staged changes; with none staged, everything
	CommitStaged  = "staged" // the staged changes only
	CommitAll     = "all"    // every change, staged first
)

// CommitOpts is one of VS Code's commit commands.
type CommitOpts struct {
	Message string // "" with Amend keeps the last commit's message
	Mode    string // CommitDefault, CommitStaged, CommitAll
	Amend   bool
	SignOff bool
}

// CommitWith commits as o says and reports whether it staged every change
// first.
func CommitWith(ctx context.Context, dir string, o CommitOpts) (bool, error) {
	msg := strings.TrimSpace(o.Message)
	if msg == "" && !o.Amend {
		return false, errors.New("empty commit message")
	}
	_, err := git(ctx, dir, "diff", "--cached", "--quiet")
	nothingStaged := err == nil // exit 0: no staged change
	staged := false
	switch o.Mode {
	case CommitAll:
		if err := StageAll(ctx, dir); err != nil {
			return false, err
		}
		staged = true
	case CommitStaged:
		if nothingStaged && !o.Amend {
			return false, ErrNothingStaged
		}
	default:
		if nothingStaged && !o.Amend {
			if err := StageAll(ctx, dir); err != nil {
				return false, err
			}
			staged = true
		}
	}
	args := []string{"commit", "-q"}
	if o.Amend {
		args = append(args, "--amend")
	}
	if o.SignOff {
		args = append(args, "--signoff")
	}
	if msg == "" {
		args = append(args, "--no-edit")
	} else {
		args = append(args, "-m", msg)
	}
	_, err = run(ctx, dir, args...)
	return staged, err
}

// UndoLastCommit takes HEAD's commit back, its changes staged, and returns
// its subject. The first commit leaves the branch without commits.
func UndoLastCommit(ctx context.Context, dir string) (string, error) {
	subject, _, err := LastCommit(ctx, dir)
	if err != nil {
		return "", err
	}
	if _, err := git(ctx, dir, "rev-parse", "-q", "--verify", "HEAD~"); err == nil {
		_, err = run(ctx, dir, "reset", "--soft", "HEAD~")
		return strings.TrimSpace(subject), err
	}
	_, err = run(ctx, dir, "update-ref", "-d", "HEAD")
	return strings.TrimSpace(subject), err
}

// Abort stops the operation under way (OpMerge ...), as it was before.
func Abort(ctx context.Context, dir, op string) error {
	switch op {
	case OpMerge, OpRebase, OpCherryPick, OpRevert:
		_, err := run(ctx, dir, op, "--abort")
		return err
	}
	return errors.New("nothing to abort")
}

// ContinueArgs is the git command that goes on with op once its conflicts
// are resolved and staged (run with GIT_EDITOR=true: the message stays).
func ContinueArgs(op string) []string {
	if op == OpMerge {
		return []string{"commit", "--no-edit"}
	}
	return []string{op, "--continue"}
}

// Checkout switches to ref of kind: a local branch; a remote branch, through
// its local branch (made, tracking it, when there is none); a tag or a
// commit, detached.
func Checkout(ctx context.Context, dir, ref string) error {
	return CheckoutRef(ctx, dir, ref, RefBranch)
}

// CheckoutRef is Checkout for ref of kind.
func CheckoutRef(ctx context.Context, dir, ref, kind string) error {
	var err error
	switch kind {
	case RefBranch, RefHead:
		_, err = run(ctx, dir, "switch", "--", ref)
	case RefRemote:
		_, local, ok := strings.Cut(ref, "/")
		if !ok {
			return fmt.Errorf("not a remote branch: %s", ref)
		}
		if _, e := git(ctx, dir, "rev-parse", "-q", "--verify", "refs/heads/"+local); e == nil {
			_, err = run(ctx, dir, "switch", "--", local)
		} else {
			_, err = run(ctx, dir, "switch", "--track", ref)
		}
	default:
		_, err = run(ctx, dir, "switch", "--detach", ref)
	}
	return err
}

// NewBranch creates branch from HEAD and switches to it.
func NewBranch(ctx context.Context, dir, branch string) error {
	return CreateBranch(ctx, dir, branch, "")
}

// CreateBranch creates branch name from ref ("" = HEAD) and switches to it.
func CreateBranch(ctx context.Context, dir, name, from string) error {
	args := []string{"switch", "-c", strings.TrimSpace(name)}
	if from != "" {
		args = append(args, from)
	}
	_, err := run(ctx, dir, args...)
	return err
}

// RenameBranch renames the current branch.
func RenameBranch(ctx context.Context, dir, name string) error {
	_, err := run(ctx, dir, "branch", "-m", strings.TrimSpace(name))
	return err
}

// DeleteBranch deletes a local branch; one not merged needs force
// (ErrNotMerged without it).
func DeleteBranch(ctx context.Context, dir, name string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := run(ctx, dir, "branch", flag, "--", name)
	if err != nil && strings.Contains(err.Error(), "not fully merged") {
		return ErrNotMerged
	}
	return err
}

// AddRemote adds remote name at url.
func AddRemote(ctx context.Context, dir, name, url string) error {
	_, err := run(ctx, dir, "remote", "add", "--", strings.TrimSpace(name), strings.TrimSpace(url))
	return err
}

// RemoveRemote removes remote name and its remote-tracking branches.
func RemoveRemote(ctx context.Context, dir, name string) error {
	_, err := run(ctx, dir, "remote", "remove", name)
	return err
}

// StashPush stashes the changes: with untracked files too, or the staged
// changes only (git 2.35); message may be "".
func StashPush(ctx context.Context, dir, message string, untracked, staged bool) error {
	args := []string{"stash", "push"}
	if untracked {
		args = append(args, "--include-untracked")
	}
	if staged {
		args = append(args, "--staged")
	}
	if m := strings.TrimSpace(message); m != "" {
		args = append(args, "-m", m)
	}
	out, err := run(ctx, dir, args...)
	if err == nil && strings.Contains(out, "No local changes to save") {
		return ErrNoChanges
	}
	return err
}

// StashApply applies stash ref ("" = the latest), keeping it.
func StashApply(ctx context.Context, dir, ref string) error { return stash(ctx, dir, "apply", ref) }

// StashPop applies stash ref ("" = the latest) and drops it.
func StashPop(ctx context.Context, dir, ref string) error { return stash(ctx, dir, "pop", ref) }

// StashDrop drops stash ref.
func StashDrop(ctx context.Context, dir, ref string) error { return stash(ctx, dir, "drop", ref) }

// StashClear drops every stash.
func StashClear(ctx context.Context, dir string) error {
	_, err := run(ctx, dir, "stash", "clear")
	return err
}

func stash(ctx context.Context, dir, op, ref string) error {
	args := []string{"stash", op}
	if ref != "" {
		args = append(args, ref)
	}
	_, err := run(ctx, dir, args...)
	return err
}

// CreateTag tags ref ("" = HEAD): annotated with message, lightweight
// without one.
func CreateTag(ctx context.Context, dir, name, message, ref string) error {
	args := []string{"tag"}
	if m := strings.TrimSpace(message); m != "" {
		args = append(args, "-a", "-m", m)
	}
	args = append(args, "--", strings.TrimSpace(name))
	if ref != "" {
		args = append(args, ref)
	}
	_, err := run(ctx, dir, args...)
	return err
}

// DeleteTag deletes a local tag.
func DeleteTag(ctx context.Context, dir, name string) error {
	_, err := run(ctx, dir, "tag", "-d", "--", name)
	return err
}

// AddWorktree checks branch out in a new worktree at path; with newBranch
// the branch is made from HEAD.
func AddWorktree(ctx context.Context, dir, path, branch string, newBranch bool) error {
	args := []string{"worktree", "add"}
	if newBranch {
		args = append(args, "-b", branch, "--", path)
	} else {
		args = append(args, "--", path, branch)
	}
	_, err := run(ctx, dir, args...)
	return err
}

// RemoveWorktree removes the worktree at path; one with changes needs force.
func RemoveWorktree(ctx context.Context, dir, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	_, err := run(ctx, dir, append(args, "--", path)...)
	return err
}

// WorktreePath is where Create Worktree puts branch: in a folder next to
// the project's, "<project>.worktrees/<branch>".
func WorktreePath(top, branch string) string {
	return filepath.Join(filepath.Dir(top), filepath.Base(top)+".worktrees", filepath.FromSlash(branch))
}

// Ignore adds p (relative to dir) to the .gitignore at the top of the
// working tree, as a path from there. Where dir is in the tree is git's
// answer: dir compared with --show-toplevel's path would differ when it was
// reached through a symlink (macOS's /var is /private/var).
func Ignore(ctx context.Context, dir, p string) error {
	out, err := git(ctx, dir, "rev-parse", "--show-cdup", "--show-prefix")
	if err != nil {
		return err
	}
	cdup, prefix, _ := strings.Cut(strings.TrimSuffix(strings.ReplaceAll(out, "\r", ""), "\n"), "\n")
	rel := path.Clean(prefix + filepath.ToSlash(p))
	if filepath.IsAbs(p) || rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return fmt.Errorf("%s is outside the repository", p)
	}
	file := filepath.Join(dir, filepath.FromSlash(cdup), ".gitignore")
	b, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	entry := rel
	for _, l := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(l) == entry {
			return nil
		}
	}
	if len(b) > 0 && b[len(b)-1] != '\n' {
		entry = "\n" + entry
	}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	_, err = f.WriteString(entry + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	Log(dir, []string{"(add to .gitignore)", rel}, "", err)
	return err
}

// Init makes dir a git repository.
func Init(ctx context.Context, dir string) error {
	_, err := run(ctx, dir, "init")
	return err
}
