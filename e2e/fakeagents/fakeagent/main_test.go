package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// fakeRun is one run of a fake agent: the agent, its arguments, what it
// reads, and the control files in its log directory.
type fakeRun struct {
	agent string
	args  []string
	stdin string
	files map[string]string // log dir file → content
	env   []string
}

// TestSameAsFakeSh (T90.9): the Go fake and fake.sh, run on the same
// arguments in the same place, print, exit and write the same — times, ids
// and dates aside — so the fake agents of Windows and of macOS and Linux
// cannot drift apart.
func TestSameAsFakeSh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake.sh is the fake of macOS and Linux")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh")
	}
	fakes, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "fakeagent")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	hand := "[i-mux handoff · Claude → Codex]\nsay \"hi\" \\ there\n"
	cases := map[string][]fakeRun{
		"probes": {
			{agent: "claude", args: []string{"--version"}},
			{agent: "codex", args: []string{"--version"}, env: []string{"FAKE_VERSION=1.2.3"}},
			{agent: "opencode", args: []string{"models"}},
			{agent: "copilot", args: []string{"--help"}},
			{agent: "antigravity", args: []string{"update", "--yes"}},
			{agent: "opencode", args: []string{"api", "experimental.usage.limits"}},
			{agent: "opencode", args: []string{"api", "experimental.usage.limits"}, files: map[string]string{"opencode.go-usage": "{\"data\":1}\n"}},
		},
		"claude": {
			{agent: "claude", args: []string{"--session-id", "11111111-2222-4333-8444-555555555555", "fix it"}, stdin: "first line\nsecond\n"},
			{agent: "claude", args: []string{"--resume", "11111111-2222-4333-8444-555555555555"}, stdin: "more\n/exit 3\n"},
			{agent: "claude", args: []string{hand}, files: map[string]string{"claude.handoff-as-request": ""}},
			{agent: "claude", files: map[string]string{"claude.nosession": ""}, env: []string{"FAKE_EXIT_STATUS=5"}},
		},
		"others": {
			{agent: "codex", args: []string{"do it"}},
			{agent: "codex", args: []string{"resume", "abc"}},
			{agent: "antigravity", args: []string{"--conversation", "conv-1"}},
			{agent: "antigravity", args: []string{"--conversation", "conv-1"}},
			{agent: "opencode", args: []string{"--session", "ses_one"}},
			{agent: "opencode", args: []string{"session", "export", "--standalone", "ses_one"}},
			{agent: "opencode", args: []string{"session", "export", "ses_missing"}},
			{agent: "copilot", args: []string{"--resume", "cop-1", "hi"}},
		},
		"headless": {
			{agent: "claude", args: []string{"-p", "--output-format", "stream-json", "--session-id", "s1"}, stdin: "diff\n"},
			{agent: "claude", args: []string{"-p"}, stdin: "x", files: map[string]string{"claude.headless-reply": "fix: \"quoted\"\tand\nmore\n"}},
			{agent: "copilot", args: []string{"-p", "msg"}, files: map[string]string{"copilot.headless-plain": ""}},
			{agent: "antigravity", args: []string{"-p", "msg"}},
			{agent: "opencode", args: []string{"run", "msg"}},
		},
		"control": {
			{agent: "claude", args: []string{"--resume", "gone"}, files: map[string]string{"claude.sh": "case \"$1\" in --resume) echo 'session not found'; exit 1;; esac\n"}},
			{agent: "claude", args: []string{"a"}, stdin: "typed\n", files: map[string]string{"claude.sh": "echo \"ctl $n $agent $1\"; [ -d \"$log\" ] && echo has-log\n"}},
			{agent: "codex", stdin: "x\n", env: []string{"FAKE_SCRIPT=echo scripted; exit 7"}},
		},
		"nvim": {
			{agent: "nvim", args: []string{"+12", "a file.go"}, env: []string{"NVIM_APPNAME=imux-nvim"}},
		},
	}
	for name, runs := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			sh := play(t, root, runs, func(r fakeRun) *exec.Cmd {
				if r.agent == "nvim" {
					return exec.Command("sh", append([]string{filepath.Join(fakes, "nvim")}, r.args...)...)
				}
				c := exec.Command("sh", append([]string{filepath.Join(fakes, "fake.sh")}, r.args...)...)
				c.Env = append(c.Env, "FAKE_AGENT="+r.agent)
				return c
			})
			gobin := filepath.Join(t.TempDir(), "bin")
			_ = os.MkdirAll(gobin, 0o755)
			goRes := play(t, root, runs, func(r fakeRun) *exec.Cmd {
				name := "fake-" + r.agent
				if r.agent == "nvim" {
					name = "nvim"
				}
				p := filepath.Join(gobin, name)
				if _, err := os.Stat(p); err != nil {
					if err := os.Link(bin, p); err != nil {
						t.Fatal(err)
					}
				}
				return exec.Command(p, r.args...)
			})
			if sh != goRes {
				t.Errorf("fake.sh and the Go fake differ\n--- fake.sh\n%s\n--- Go\n%s", sh, goRes)
			}
		})
	}
}

var (
	reTime = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d[:-]\d\d[:-]\d\dZ?`)
	reMs   = regexp.MustCompile(`\b1\d{12}\b`)
	reUUID = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	reDay  = regexp.MustCompile(`sessions/\d{4}/\d\d/\d\d`)
)

// play runs runs in a fresh root (the same path for both fakes) and returns
// everything they printed, their exit statuses, and every file under root.
func play(t *testing.T, root string, runs []fakeRun, cmd func(fakeRun) *exec.Cmd) string {
	t.Helper()
	ents, _ := os.ReadDir(root)
	for _, e := range ents {
		_ = os.RemoveAll(filepath.Join(root, e.Name()))
	}
	home, log, work := filepath.Join(root, "home"), filepath.Join(root, "log"), filepath.Join(root, "work")
	for _, d := range []string{home, log, work} {
		_ = os.MkdirAll(d, 0o755)
	}
	var out strings.Builder
	for i, r := range runs {
		for f, s := range r.files {
			if err := os.WriteFile(filepath.Join(log, f), []byte(s), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		c := cmd(r)
		c.Dir = work
		c.Env = append(append(c.Env, "HOME="+home, "IMUX_FAKE_LOG="+log, "PATH="+os.Getenv("PATH"), "PWD="+work, "TMUX_PANE=%3", "IMUX_PANE=%3", "IMUX_AGENT="+r.agent), r.env...)
		c.Stdin = strings.NewReader(r.stdin)
		var o, e bytes.Buffer
		c.Stdout, c.Stderr = &o, &e
		err := c.Run()
		code := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		out.WriteString("=== run " + r.agent + " " + strings.Join(r.args, " ") + "\nstdout:\n" + o.String() + "stderr:\n" + e.String() + "exit " + strconv.Itoa(code) + "\n")
		for f := range r.files {
			_ = os.Remove(filepath.Join(log, f))
		}
	}
	var files []string
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	var dump []string // normalized first, then sorted: random ids are in names
	for _, p := range files {
		b, _ := os.ReadFile(p)
		rel, _ := filepath.Rel(root, p)
		dump = append(dump, normalize("=== file "+rel+"\n"+string(b)+"\n"))
	}
	sort.Strings(dump)
	return normalize(out.String()) + strings.Join(dump, "")
}

func normalize(s string) string {
	s = reTime.ReplaceAllString(s, "<time>")
	s = reMs.ReplaceAllString(s, "<ms>")
	s = reUUID.ReplaceAllString(s, "<uuid>")
	return reDay.ReplaceAllString(s, "sessions/<day>")
}
