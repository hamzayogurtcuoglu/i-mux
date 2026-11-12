// Package commitmsg asks one of the user's agents for a commit message
// (ARCHITECTURE D29): it gathers the changes, writes the prompt, runs the
// agent once without its UI in an empty folder and cleans the reply.
package commitmsg

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/hamzayogurtcuoglu/i-mux/internal/agent"
	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/handoff/redact"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// Limits.
const (
	MaxPatch          = 48 << 10 // the patch is cut here
	maxUntrackedFile  = 8 << 10  // untracked text files up to this size are shown
	maxUntrackedTotal = 24 << 10
	maxSubjects       = 10
	MaxMessage        = 200 // runes kept of the reply
)

// Timeout bounds one agent run.
var Timeout = 120 * time.Second

// Changes is what the message is written from.
type Changes struct {
	Staged    bool   // the staged changes; else the work tree and untracked files
	Stat      string // git diff --stat
	Patch     string // cut at MaxPatch, secrets redacted
	Cut       bool
	Untracked []string
	Subjects  []string // recent commit subjects, newest first
}

// Empty reports whether there is nothing to commit.
func (c *Changes) Empty() bool {
	return strings.TrimSpace(c.Stat) == "" && len(c.Untracked) == 0
}

// Files counts the changed files.
func (c *Changes) Files() int {
	n := len(c.Untracked)
	for _, l := range strings.Split(c.Stat, "\n") {
		if strings.Contains(l, "|") {
			n++
		}
	}
	return n
}

// Collect reads the changes of the repository at dir: the staged ones, or
// with nothing staged what Commit would commit (the work tree and untracked
// files).
func Collect(ctx context.Context, dir string) (*Changes, error) {
	git := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir, "-c", "core.quotepath=off"}, args...)...)
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git %s: %s", args[0], firstLine(errb.String(), err.Error()))
		}
		return out.String(), nil
	}
	staged, err := git("diff", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}
	c := &Changes{Staged: strings.TrimSpace(staged) != ""}
	diff := []string{"diff", "--no-color", "--no-ext-diff"}
	if c.Staged {
		diff = append(diff, "--cached")
	}
	if c.Stat, err = git(append(diff, "--stat")...); err != nil {
		return nil, err
	}
	patch, err := git(diff...)
	if err != nil {
		return nil, err
	}
	if !c.Staged {
		out, err := git("ls-files", "--others", "--exclude-standard")
		if err != nil {
			return nil, err
		}
		total := 0
		for _, p := range strings.Split(strings.TrimSpace(out), "\n") {
			if p == "" {
				continue
			}
			c.Untracked = append(c.Untracked, p)
			if b, ok := smallText(filepath.Join(dir, p)); ok && total+len(b) <= maxUntrackedTotal {
				total += len(b)
				patch += "\n--- new file: " + p + "\n" + string(b)
				if !strings.HasSuffix(patch, "\n") {
					patch += "\n"
				}
			}
		}
	}
	if len(patch) > MaxPatch {
		cut := strings.LastIndexByte(patch[:MaxPatch], '\n')
		if cut < 0 {
			cut = MaxPatch
		}
		patch, c.Cut = patch[:cut+1], true
	}
	c.Patch, _ = redact.String(strings.ToValidUTF8(patch, "?"))
	if out, err := git("log", "-n", fmt.Sprint(maxSubjects), "--format=%s"); err == nil {
		for _, s := range strings.Split(strings.TrimSpace(out), "\n") {
			if s = strings.TrimSpace(s); s != "" {
				c.Subjects = append(c.Subjects, s)
			}
		}
	}
	return c, nil
}

// smallText returns a regular file's content when it is small text.
func smallText(p string) ([]byte, bool) {
	fi, err := os.Lstat(p)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > maxUntrackedFile {
		return nil, false
	}
	b, err := os.ReadFile(p)
	if err != nil || bytes.IndexByte(b, 0) >= 0 || !utf8.Valid(b) {
		return nil, false
	}
	return b, true
}

// Instructions is what the agent is asked to do.
const Instructions = "Write a git commit message for the changes below.\n" +
	"- One line, at most 72 characters, in the imperative mood (\"Add\", \"Fix\" ...).\n" +
	"- Follow the style of the recent commit subjects (for example a \"feat:\" or \"fix:\" prefix when they use one, and their language).\n" +
	"- Reply with the commit message only: no quotes, no explanation, no code block."

// Body is the material: recent subjects and the changes.
func Body(c *Changes) string {
	var b strings.Builder
	b.WriteString("Recent commit subjects:\n")
	if len(c.Subjects) == 0 {
		b.WriteString("(no commits yet)\n")
	}
	for _, s := range c.Subjects {
		b.WriteString("- " + s + "\n")
	}
	if c.Staged {
		b.WriteString("\nThe staged changes:\n")
	} else {
		b.WriteString("\nThe changes (nothing is staged: all of them are committed):\n")
	}
	b.WriteString(c.Stat)
	if len(c.Untracked) > 0 {
		b.WriteString("New files: " + strings.Join(c.Untracked, ", ") + "\n")
	}
	b.WriteString("\n" + c.Patch)
	if c.Cut {
		b.WriteString("\n[the rest of the patch is left out]\n")
	}
	return b.String()
}

// Run is one agent invocation.
type Run struct {
	Argv    []string // Argv[0] is the binary
	Stdin   string
	OutFile string // the reply is read from this file instead of stdout
}

// Command builds the run of agent id (binary bin) in folder dir.
func Command(id session.AgentID, bin, model, dir, instructions, body string) (Run, error) {
	full := instructions + "\n\n" + body
	opt := func(flag string) []string {
		if model == "" {
			return nil
		}
		return []string{flag, model}
	}
	switch id {
	case session.Claude:
		// The changes on stdin, the instructions as the prompt; no tools, no
		// session file.
		argv := append([]string{bin, "-p", "--no-session-persistence", "--tools", ""}, opt("--model")...)
		return Run{Argv: append(argv, instructions), Stdin: body}, nil
	case session.Codex:
		out := filepath.Join(dir, "message.txt")
		argv := append([]string{bin, "exec", "--skip-git-repo-check", "--sandbox", "read-only", "--color", "never", "--output-last-message", out}, opt("-m")...)
		return Run{Argv: append(argv, "-"), Stdin: full, OutFile: out}, nil
	case session.Copilot:
		return Run{Argv: append([]string{bin, "-p", full, "-s", "--no-color"}, opt("--model")...)}, nil
	case session.Antigravity:
		return Run{Argv: append([]string{bin, "-p", full}, opt("--model")...)}, nil
	case session.OpenCode:
		argv := append([]string{bin, "run", "--archived", "--title", "imux commit message"}, opt("-m")...)
		return Run{Argv: append(argv, full)}, nil
	}
	return Run{}, fmt.Errorf("%s cannot write commit messages", id)
}

// Pick chooses the agent: git.commit_agent, or for "auto" the agent of the
// pane the sidebar follows when installed, else the first installed one in
// order. The model applies only to an agent named in the config.
func Pick(cfg *config.Config, paneAgent string, installed func(session.AgentID) bool) (session.AgentID, string, error) {
	if a := cfg.Git.CommitAgent; a != "" && a != "auto" {
		id := session.AgentID(a)
		if !installed(id) {
			return id, "", errors.New(agent.MustGet(id).Name + " is not installed")
		}
		return id, cfg.Git.CommitModel, nil
	}
	if session.Known(paneAgent) && installed(session.AgentID(paneAgent)) {
		return session.AgentID(paneAgent), "", nil
	}
	for _, a := range agent.Ordered(cfg) {
		if installed(a.ID) {
			return a.ID, "", nil
		}
	}
	return "", "", errors.New("no agent is installed")
}

// Execute runs r in dir with PATH pathEnv (when set) and returns the cleaned
// message.
func Execute(ctx context.Context, r Run, dir, pathEnv string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.Argv[0], r.Argv[1:]...)
	// The agent may start children (node, a shell): on timeout the whole
	// group goes, and pipes they hold do not keep us waiting.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if pathEnv != "" {
		cmd.Env = append(cmd.Env, "PATH="+pathEnv)
	}
	cmd.Env = append(cmd.Env, "NO_COLOR=1")
	cmd.Stdin = strings.NewReader(r.Stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("no reply within %s", Timeout)
	}
	reply := out.String()
	if r.OutFile != "" {
		if b, ferr := os.ReadFile(r.OutFile); ferr == nil {
			reply = string(b)
		}
	}
	if err != nil {
		return "", errors.New(lastLine(errb.String()+"\n"+out.String(), err.Error()))
	}
	msg := Clean(reply)
	if msg == "" {
		return "", errors.New("the reply held no message")
	}
	return msg, nil
}

var (
	ansiRe   = regexp.MustCompile("\x1b\\[[0-9;?]*[ -/]*[@-~]|\x1b\\][^\x07\x1b]*(\x07|\x1b\\\\)")
	prefixRe = regexp.MustCompile(`(?i)^(suggested\s+)?(git\s+)?commit\s+message\s*:\s*`)
	leadInRe = regexp.MustCompile(`(?i)^(here('s| is)|sure|certainly|okay|ok)\b.*:\s*$`)
)

// Clean turns an agent's reply into a one-line message: escape codes and
// code fences go, lead-ins ("Here is the message:") are skipped, a "Commit
// message:" prefix and surrounding quotes are removed.
func Clean(reply string) string {
	reply = ansiRe.ReplaceAllString(reply, "")
	for _, l := range strings.Split(reply, "\n") {
		l = strings.TrimSpace(strings.TrimRight(l, "\r"))
		if l == "" || strings.HasPrefix(l, "```") || leadInRe.MatchString(l) {
			continue
		}
		l = prefixRe.ReplaceAllString(l, "")
		l = unquote(strings.TrimSpace(l))
		if l == "" {
			continue
		}
		if utf8.RuneCountInString(l) > MaxMessage {
			l = string([]rune(l)[:MaxMessage])
		}
		return l
	}
	return ""
}

func unquote(s string) string {
	for _, q := range []string{"`", `"`, "'", "**"} {
		if len(s) >= 2*len(q) && strings.HasPrefix(s, q) && strings.HasSuffix(s, q) {
			s = strings.TrimSpace(s[len(q) : len(s)-len(q)])
		}
	}
	return s
}

func firstLine(s, fallback string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return fallback
}

func lastLine(s, fallback string) string {
	s = ansiRe.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lines[i]); l != "" {
			return l
		}
	}
	return fallback
}
