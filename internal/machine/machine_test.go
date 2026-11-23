package machine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

func TestCheckTarget(t *testing.T) {
	good := []string{"ali@sunucu", "sunucu", "build-box.example.com", "ali@10.0.0.2", "[::1]", "ssh://ali@sunucu:2222", "ali_x@host"}
	bad := []string{
		"", "-oProxyCommand=touch /tmp/x", "-p", "ali@-oProxyCommand=x", "ali@host;rm -rf ~", "host name",
		"$(id)", "`id`", "host'x", "host\"x", "host\nx", "host|x", "host&x", "a/b", "~host", strings.Repeat("a", 260),
	}
	for _, s := range good {
		if err := CheckTarget(s); err != nil {
			t.Errorf("CheckTarget(%q) = %v, want ok", s, err)
		}
	}
	for _, s := range bad {
		if err := CheckTarget(s); err == nil {
			t.Errorf("CheckTarget(%q) = ok, want an error", s)
		}
	}
}

func TestCheckName(t *testing.T) {
	for _, s := range []string{"sunucu", "a", "build-2", strings.Repeat("a", 32)} {
		if CheckName(s) != nil {
			t.Errorf("CheckName(%q) refused", s)
		}
	}
	for _, s := range []string{"", "-a", "A", "a b", "a/b", "../x", "a.b", strings.Repeat("a", 33)} {
		if CheckName(s) == nil {
			t.Errorf("CheckName(%q) accepted", s)
		}
	}
}

func TestNameFor(t *testing.T) {
	for in, want := range map[string]string{
		"ali@sunucu.example.com": "sunucu",
		"ssh://ali@Build-Box:22": "build-box",
		"[::1]":                  "machine",
		"10.0.0.2":               "10",
		"host_x":                 "hostx",
	} {
		if got := NameFor(in); got != want || CheckName(got) != nil {
			t.Errorf("NameFor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStore(t *testing.T) {
	t.Setenv("IMUX_HOME", t.TempDir())
	if ms, err := Load(); err != nil || len(ms) != 0 {
		t.Fatalf("Load on nothing = %v, %v", ms, err)
	}
	if err := Add(Machine{Name: "b", Target: "ali@b"}); err != nil {
		t.Fatal(err)
	}
	if err := Add(Machine{Name: "a", Target: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := Add(Machine{Name: "a", Target: "x"}); !errors.Is(err, ErrExists) {
		t.Fatalf("a second a: %v", err)
	}
	if err := Add(Machine{Name: "c", Target: "-oProxyCommand=x"}); !errors.Is(err, ErrTarget) {
		t.Fatalf("an option as a destination: %v", err)
	}
	fi, err := os.Stat(File())
	if err != nil || (testutil.ModesApply && fi.Mode().Perm() != 0o600) {
		t.Fatalf("machines.json mode %v, %v", fi.Mode(), err)
	}
	ms, _ := Load()
	if len(ms) != 2 || ms[0].Name != "a" || ms[1].Name != "b" || ms[0].Added.IsZero() {
		t.Fatalf("Load = %+v", ms)
	}
	if err := WriteState("a", State{Status: StatusOK}); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(CacheFile("a")); err != nil || (testutil.ModesApply && fi.Mode().Perm() != 0o600) {
		t.Fatalf("state mode %v, %v", fi.Mode(), err)
	}
	if err := Remove("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(CacheFile("a")); !os.IsNotExist(err) {
		t.Fatal("Remove kept the state")
	}
	if _, err := Get("a"); !errors.Is(err, ErrNone) {
		t.Fatal("a is still there")
	}
	if err := Remove("zz"); !errors.Is(err, ErrNone) {
		t.Fatal(err)
	}
	// A file edited by hand is checked as typed input is.
	b, _ := json.Marshal([]Machine{{Name: "ok", Target: "ok"}, {Name: "bad", Target: "-oProxyCommand=x"}, {Name: "../x", Target: "y"}})
	_ = os.WriteFile(File(), b, 0o600)
	if ms, _ := Load(); len(ms) != 1 || ms[0].Name != "ok" {
		t.Fatalf("hand-edited Load = %+v", ms)
	}
}

func TestArgs(t *testing.T) {
	testutil.SkipOnWindows(t, "POSIX ssh control paths (/h/ssh)")
	t.Setenv("IMUX_HOME", "/h")
	m := Machine{Name: "s", Target: "ali@s"}
	a := Args(m, Background, "__remote-state")
	i := slices.Index(a, "--")
	if i < 0 || a[i+1] != "ali@s" || len(a) != i+3 {
		t.Fatalf("destination not after --: %q", a)
	}
	for _, want := range []string{"BatchMode=yes", "ForwardAgent=no", "ForwardX11=no", "ClearAllForwardings=yes", "PermitLocalCommand=no", "ControlPath=/h/ssh/%C", "-T"} {
		if !slices.Contains(a[:i], want) {
			t.Errorf("no %s in %q", want, a)
		}
	}
	for _, o := range a[:i] {
		if strings.HasPrefix(o, "StrictHostKeyChecking") || strings.HasPrefix(o, "UserKnownHostsFile") {
			t.Errorf("host keys are the user's: %s", o)
		}
	}
	if b := Args(m, Interactive); slices.Contains(b, "BatchMode=yes") || !slices.Contains(b, "BatchMode=no") {
		t.Errorf("Interactive: %q", b)
	}
	if b := Args(m, Attach, "x"); !slices.Contains(b, "-t") {
		t.Errorf("Attach has no terminal: %q", b)
	}
	if e := ExitArgs(m); e[len(e)-2] != "--" || e[len(e)-1] != "ali@s" {
		t.Errorf("ExitArgs: %q", e)
	}
}

// The master connection's options are there exactly where ssh can share a
// connection (D96: not on Windows); every other option is there everywhere.
func TestOptionsShare(t *testing.T) {
	testutil.Isolate(t)
	for _, mode := range []Mode{Background, Interactive, Attach} {
		o := Options(mode)
		for _, k := range []string{"ControlMaster=", "ControlPath=", "ControlPersist="} {
			has := slices.ContainsFunc(o, func(s string) bool { return strings.HasPrefix(s, k) })
			if has != SharesConnection {
				t.Errorf("mode %d: %s present %v, SharesConnection %v: %q", mode, k, has, SharesConnection, o)
			}
		}
		for _, want := range []string{"ConnectTimeout=10", "ServerAliveInterval=15", "ServerAliveCountMax=3", "ForwardAgent=no", "ForwardX11=no", "ClearAllForwardings=yes", "PermitLocalCommand=no"} {
			if !slices.Contains(o, want) {
				t.Errorf("mode %d: no %s in %q", mode, want, o)
			}
		}
		for i, s := range o {
			if s == "-o" && (i+1 == len(o) || strings.HasPrefix(o[i+1], "-")) {
				t.Errorf("mode %d: -o without a value in %q", mode, o)
			}
		}
	}
}

// RemoteCommand's words reach i-mux on the machine as they were, whatever
// they hold: the remote shell is run here.
func TestRemoteCommand(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh")
	}
	bin := t.TempDir()
	// A fake imux that prints its arguments, one per line.
	if err := os.WriteFile(filepath.Join(bin, "imux"), []byte("#!/bin/sh\nfor a in \"$@\"; do printf '%s\\n' \"$a\"; done\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	words := []string{"open", "-d", "--", "my dir/$(touch pwned)'\"`id`;x", "*"}
	cmd := exec.CommandContext(context.Background(), sh, "-c", RemoteCommand(words...))
	cmd.Dir = bin
	cmd.Env = append(os.Environ(), "PATH="+bin+":/usr/bin:/bin", "HOME="+bin)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n"); !slices.Equal(got, words) {
		t.Fatalf("got %q, want %q", got, words)
	}
	if _, err := os.Stat(filepath.Join(bin, "pwned")); err == nil {
		t.Fatal("the folder ran as a command")
	}
	// No i-mux there: 127.
	cmd = exec.Command(sh, "-c", RemoteCommand("version"))
	cmd.Env = append(os.Environ(), "PATH=/nonexistent", "HOME="+t.TempDir())
	var ee *exec.ExitError
	if err := cmd.Run(); !errors.As(err, &ee) || ee.ExitCode() != 127 {
		t.Fatalf("without i-mux: %v", err)
	}
}

func TestValidIDs(t *testing.T) {
	for _, s := range []string{"proj", "my-proj_2.x"} {
		if !ValidSession(s) {
			t.Errorf("ValidSession(%q)", s)
		}
	}
	for _, s := range []string{"", "a b", "-t", "a;b", "a:b", "$(x)", strings.Repeat("a", 65)} {
		if ValidSession(s) {
			t.Errorf("ValidSession(%q) accepted", s)
		}
	}
	if !ValidPane("%12") || ValidPane("12") || ValidPane("%1;x") || ValidPane("%") {
		t.Error("ValidPane")
	}
}

func TestClean(t *testing.T) {
	if got := Clean("a\x1b]52;c;aGk=\x07b\u009bc\u2028d\re\xff"); got != "a]52;c;aGk=bcde\ufffd" {
		t.Errorf("Clean = %q", got)
	}
	if got := Clean(strings.Repeat("ş", 100)); len([]rune(got)) != MaxText+1 || !strings.HasSuffix(got, "…") {
		t.Errorf("Clean long = %d runes", len([]rune(got)))
	}
}

func TestParse(t *testing.T) {
	if _, err := Parse([]byte("not json")); err == nil {
		t.Fatal("parsed nothing")
	}
	if _, err := Parse(make([]byte, MaxReport+1)); err == nil {
		t.Fatal("parsed a report too large")
	}
	r := Report{Version: "v1\x1b[2J", Projects: []Project{
		{Session: "ok", Name: "web\x1b[31m", Path: "/p", Panes: []Pane{
			{ID: "%1", Agent: "claude", State: "working", Name: "a\x07"},
			{ID: "1;rm", Agent: "claude", State: "idle"},
			{ID: "%2", Agent: "evil", State: "pwned"},
		}},
		{Session: "bad session", Name: "x"},
		{Session: "noname"},
	}}
	for i := 0; i < MaxPanes+5; i++ {
		r.Projects[2].Panes = append(r.Projects[2].Panes, Pane{ID: "%9", State: "idle"})
	}
	for i := 0; i < MaxProjects+5; i++ {
		r.Projects = append(r.Projects, Project{Session: "p"})
	}
	b, _ := json.Marshal(r)
	got, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "v1[2J" || len(got.Projects) != MaxProjects {
		t.Fatalf("version %q, %d projects", got.Version, len(got.Projects))
	}
	p := got.Projects[0]
	if p.Name != "web[31m" || len(p.Panes) != 2 || p.Panes[0].Name != "a" || p.Panes[0].Agent != "claude" {
		t.Fatalf("project = %+v", p)
	}
	if p.Panes[1].Agent != "" || p.Panes[1].State != "" {
		t.Fatalf("unknown agent and state kept: %+v", p.Panes[1])
	}
	if got.Projects[1].Session != "noname" || got.Projects[1].Name != "noname" || len(got.Projects[1].Panes) != MaxPanes {
		t.Fatalf("second = %+v", got.Projects[1])
	}
}

func TestFailed(t *testing.T) {
	exit := func(code int) error { return exec.Command("sh", "-c", "exit "+strconv.Itoa(code)).Run() }
	if s := failed(exit(255), "ali@s: Permission denied (publickey).\n", nil); s.Status != StatusAuth || s.Error != "ali@s: Permission denied (publickey)." {
		t.Errorf("auth = %+v", s)
	}
	if s := failed(exit(255), "Host key verification failed.\n", nil); s.Status != StatusAuth {
		t.Errorf("host key = %+v", s)
	}
	// An exit code from a real process (Windows has no sh on PATH).
	if _, err := exec.LookPath("sh"); err == nil {
		if s := failed(exit(127), "i-mux is not installed on this machine\n", nil); s.Status != StatusNoImux {
			t.Errorf("no i-mux = %+v", s)
		}
	}
	if s := failed(exit(255), "ssh: connect to host s port 22: Connection refused\n", nil); s.Status != StatusOffline || !strings.Contains(s.Error, "refused") {
		t.Errorf("offline = %+v", s)
	}
	if s := failed(exit(255), "", context.DeadlineExceeded); s.Status != StatusOffline || s.Error != "timed out" {
		t.Errorf("timeout = %+v", s)
	}
}

// Fetch with a fake ssh: the report read, an error classified.
func TestFetch(t *testing.T) {
	testutil.SkipOnWindows(t, "the fake ssh is an sh script")
	t.Setenv("IMUX_HOME", t.TempDir())
	dir := t.TempDir()
	fake := filepath.Join(dir, "ssh")
	script := "#!/bin/sh\ncase \"$*\" in\n" +
		"*ok*) echo '{\"version\":\"v9\",\"projects\":[{\"session\":\"web\",\"name\":\"web\",\"path\":\"/w\",\"panes\":[{\"id\":\"%1\",\"agent\":\"claude\",\"state\":\"blocked\"}]}]}' ;;\n" +
		"*denied*) echo 'Permission denied (publickey).' >&2; exit 255 ;;\n" +
		"*) echo 'i-mux is not installed on this machine' >&2; exit 127 ;;\nesac\n"
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IMUX_SSH", fake)
	r, st := Fetch(context.Background(), Machine{Name: "ok", Target: "ok"})
	if st.Status != StatusOK || r == nil || len(st.Projects) != 1 || st.Projects[0].Panes[0].State != "blocked" || st.Version != "v9" {
		t.Fatalf("ok: %+v %+v", r, st)
	}
	if fi, err := os.Stat(ControlDir()); err != nil || (testutil.ModesApply && fi.Mode().Perm() != 0o700) {
		t.Fatalf("control dir: %v %v", fi, err)
	}
	if _, st := Fetch(context.Background(), Machine{Name: "d", Target: "denied"}); st.Status != StatusAuth {
		t.Fatalf("denied: %+v", st)
	}
	if _, st := Fetch(context.Background(), Machine{Name: "n", Target: "none"}); st.Status != StatusNoImux {
		t.Fatalf("none: %+v", st)
	}
}
