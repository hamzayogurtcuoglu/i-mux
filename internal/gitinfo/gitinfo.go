// Package gitinfo collects branch, `git status --short` and `git diff --stat`
// for the handoff's "Repository state" section, with a timeout per call.
// A missing git binary, a non-repository and a timeout are results, not
// errors.
package gitinfo

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// State is the overall outcome.
type State uint8

// States.
const (
	OK State = iota
	NotRepo
	NoGit
	TimedOut
	Failed
)

// Caps.
const (
	MaxStatusLines = 40
	MaxDiffLines   = 25
)

// Timeout per git call.
var Timeout = 2 * time.Second

// GitBinary is the git executable name; tests replace it.
var GitBinary = "git"

// Info is the repository state of a directory.
type Info struct {
	State       State
	Dir         string
	Branch      string   // "" when detached or unknown
	Status      []string // `git status --short` lines, capped
	StatusTotal int      // total status lines before capping
	DiffStat    []string // `git diff --stat` lines, capped (summary line kept)
	DiffTotal   int
	Err         string // short description for Failed / TimedOut
}

// StatusMore returns how many status lines were cut.
func (i Info) StatusMore() int { return i.StatusTotal - len(i.Status) }

// DiffMore returns how many diff --stat lines were cut.
func (i Info) DiffMore() int { return i.DiffTotal - len(i.DiffStat) }

var errTimeout = errors.New("timeout")

func run(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	// No lock: git diff rewrites the index (under .git/index.lock) even
	// with GIT_OPTIONAL_LOCKS=0 unless diff.autoRefreshIndex is off, and the
	// sidebar runs this every few seconds.
	// core.quotePath=false: non-ASCII paths as they are, not "\303\274".
	cmd := exec.CommandContext(ctx, GitBinary, append([]string{"-C", dir, "-c", "diff.autoRefreshIndex=false", "-c", "core.quotePath=false"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C", "GIT_TERMINAL_PROMPT=0")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	cmd.WaitDelay = 500 * time.Millisecond // do not wait for grandchildren holding the pipes
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", errTimeout
	}
	if err != nil {
		return out.String(), errors.New(strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// Collect gathers the repository state of dir.
func Collect(ctx context.Context, dir string) Info {
	info := Info{Dir: dir}
	if _, err := exec.LookPath(GitBinary); err != nil {
		info.State = NoGit
		return info
	}
	out, err := run(ctx, dir, "rev-parse", "--is-inside-work-tree")
	switch {
	case errors.Is(err, errTimeout):
		info.State = TimedOut
		return info
	case err != nil || strings.TrimSpace(out) != "true":
		info.State = NotRepo
		return info
	}
	var (
		wg                   sync.WaitGroup
		branch, status, diff string
		errB, errS, errD     error
	)
	wg.Add(3)
	go func() {
		defer wg.Done()
		branch, errB = run(ctx, dir, "symbolic-ref", "--short", "-q", "HEAD")
	}()
	go func() {
		defer wg.Done()
		status, errS = run(ctx, dir, "status", "--short", "--untracked-files=normal")
	}()
	go func() {
		defer wg.Done()
		diff, errD = run(ctx, dir, "diff", "--stat", "HEAD")
		if errD != nil && !errors.Is(errD, errTimeout) {
			diff, errD = run(ctx, dir, "diff", "--stat") // no commits yet
		}
	}()
	wg.Wait()
	if errB == nil {
		info.Branch = strings.TrimSpace(branch)
	}
	for _, e := range []error{errS, errD} {
		if errors.Is(e, errTimeout) {
			info.State = TimedOut
		} else if e != nil && info.State == OK {
			info.State = Failed
			info.Err = e.Error()
		}
	}
	info.Status, info.StatusTotal = capLines(status, MaxStatusLines, false)
	info.DiffStat, info.DiffTotal = capLines(diff, MaxDiffLines, true)
	return info
}

// capLines splits out into lines and keeps at most max. With keepLast, the
// final line (diff --stat's summary) is always kept.
func capLines(out string, max int, keepLast bool) ([]string, int) {
	out = strings.TrimRight(out, "\n")
	if out == "" {
		return nil, 0
	}
	lines := strings.Split(out, "\n")
	total := len(lines)
	if total <= max {
		return lines, total
	}
	if keepLast {
		kept := append([]string{}, lines[:max-1]...)
		return append(kept, lines[total-1]), total
	}
	return lines[:max], total
}
