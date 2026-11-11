package tmux

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// FakeRunner records invocations and returns scripted output.
type fakeRunner struct {
	calls  [][]string
	out    string
	stderr string
	err    error
}

func (f *fakeRunner) Run(_ context.Context, _ string, args []string) ([]byte, []byte, error) {
	f.calls = append(f.calls, args)
	return []byte(f.out), []byte(f.stderr), f.err
}

func TestArgvBatchingAndEscaping(t *testing.T) {
	c := &Client{Socket: "s", Config: "/conf"}
	got := c.Argv(Cmd{"set", "-p", "@x", "a;"}, Cmd{"display", "-p", "mid;dle"})
	want := []string{"-L", "s", "-f", "/conf", "-u", "set", "-p", "@x", `a\;`, ";", "display", "-p", "mid;dle"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	c.Config = ""
	if got := c.Argv(Cmd{"ls"}); !reflect.DeepEqual(got, []string{"-L", "s", "-u", "ls"}) {
		t.Fatalf("no config: %q", got)
	}
}

func TestRunClassifiesErrors(t *testing.T) {
	cases := []struct {
		stderr string
		want   error
	}{
		{"no server running on /tmp/tmux-501/imux\n", ErrNoServer},
		{"error connecting to /tmp/tmux-1000/imux (No such file or directory)\n", ErrNoServer},
		{"can't find session: proj-1a2b\n", ErrNoSession},
		{"can't find pane: %99\n", ErrNoPane},
		{"can't find client: /dev/pts/9\n", ErrNoClient},
		{"unknown command: frobnicate\n", nil},
	}
	for _, cs := range cases {
		f := &fakeRunner{stderr: cs.stderr, err: errors.New("exit status 1")}
		c := &Client{Bin: "tmux", Socket: "s", Runner: f}
		_, err := c.Do(Cmd{"x"})
		var te *Error
		if !errors.As(err, &te) {
			t.Fatalf("%q: want *Error, got %v", cs.stderr, err)
		}
		if cs.want != nil && !errors.Is(err, cs.want) {
			t.Errorf("%q: got %v, want %v", cs.stderr, err, cs.want)
		}
		if cs.want == nil && te.Kind != nil {
			t.Errorf("%q: unexpected kind %v", cs.stderr, te.Kind)
		}
		if !strings.Contains(err.Error(), strings.TrimSpace(cs.stderr)) {
			t.Errorf("message %q lacks stderr", err.Error())
		}
	}
}

func TestMissingBinary(t *testing.T) {
	c := &Client{Bin: "/nonexistent/tmux-xyz", Socket: "s"}
	if _, err := c.Do(Cmd{"ls"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v", err)
	}
	if _, err := c.BinVersion(context.Background()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("version: got %v", err)
	}
}

func TestRecorder(t *testing.T) {
	rec := filepath.Join(t.TempDir(), "rec.jsonl")
	f := &fakeRunner{}
	c := &Client{Bin: "tmux", Socket: "s", Runner: f, Record: rec}
	_, _ = c.Do(Cmd{"list-panes", "-a"})
	_, _ = c.Do(Cmd{"display", "-p", "x"}, Cmd{"set", "-g", "@y", "1"})
	got, err := ReadRecord(rec)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 records, got %d", len(got))
	}
	if !reflect.DeepEqual(got[1].Args, []string{"-L", "s", "-u", "display", "-p", "x", ";", "set", "-g", "@y", "1"}) {
		t.Fatalf("args %q", got[1].Args)
	}
	if got[0].Pid != os.Getpid() || got[0].T == 0 {
		t.Fatalf("record metadata: %+v", got[0])
	}
	// Windows keeps no Unix permission bits; files in the profile are private.
	if fi, _ := os.Stat(rec); runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Fatalf("recorder mode %v", fi.Mode().Perm())
	}
}

func TestSections(t *testing.T) {
	out := "pre\n" + Delim + "%1\nline a\nline b\n" + Delim + "%2\n" + Delim + "%3\nlast"
	got := Sections(out)
	want := map[string]string{"": "pre\n", "%1": "line a\nline b\n", "%2": "", "%3": "last"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if got := Sections(""); len(got) != 0 {
		t.Fatalf("empty: %q", got)
	}
}

func TestRows(t *testing.T) {
	got := Rows("%1\tclaude\t123\n\n%2\t\n%3\ta\tb\tc\td\n", 3)
	want := [][]string{{"%1", "claude", "123"}, {"%2", "", ""}, {"%3", "a", "b\tc\td"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in       string
		maj, min int
		suffix   string
		ok       bool
	}{
		{"tmux 3.2a\n", 3, 2, "a", true},
		{"tmux 3.8", 3, 8, "", true},
		{"tmux 3.4", 3, 4, "", true},
		{"tmux next-3.6", 3, 6, "-next", true},
		{"tmux master", 99, 0, "", true},
		{"tmux openbsd-7.6", 99, 0, "", true},
		{"tmux 2.9a", 2, 9, "a", true},
		{"tmux", 0, 0, "", false},
		{"garbage", 0, 0, "", false},
		{"tmux 3.x", 0, 0, "", false},
	}
	for _, c := range cases {
		v, ok := ParseVersion(c.in)
		if ok != c.ok || ok && (v.Major != c.maj || v.Minor != c.min || v.Suffix != c.suffix) {
			t.Errorf("ParseVersion(%q) = %+v, %v", c.in, v, ok)
		}
	}
	v, _ := ParseVersion("tmux 3.2a")
	if !v.AtLeast(3, 2) || v.AtLeast(3, 3) || !v.AtLeast(2, 9) || v.AtLeast(4, 0) {
		t.Fatalf("AtLeast wrong for %v", v)
	}
	if v.String() != "3.2a" {
		t.Fatalf("tmux shows %q", v.String())
	}
	// psmux: the tmux line gates features, psmux's own number is shown.
	for in, want := range map[string]string{
		"tmux 3.4.0\r\npsmux 3.4.0 (20aa1d7 2026-10-13)\r\n": "3.4.0",
		"tmux 3.4\npsmux 3.6.1 (abc 2027-01-01)":             "3.6.1",
		"psmux 3.4.0 (20aa1d7 2026-10-13)":                   "3.4.0",
	} {
		v, ok := ParseVersion(in)
		if !ok || v.Major != 3 || v.Minor != 4 || v.String() != want {
			t.Errorf("ParseVersion(%q) = %+v %v, want it shown as %s", in, v, ok, want)
		}
	}
	if _, ok := ParseVersion("psmux x\n"); ok {
		t.Error("psmux with no version parsed")
	}
}

func TestInside(t *testing.T) {
	c := &Client{Socket: "imux"}
	if !c.Inside("/tmp/tmux-501/imux,1234,0") {
		t.Fatal("should be inside")
	}
	if c.Inside("/tmp/tmux-501/default,1234,0") || c.Inside("") {
		t.Fatal("should not be inside")
	}
}

func TestQuote(t *testing.T) {
	cases := map[string]string{
		"%3":            "%3",
		"/usr/bin/imux": "/usr/bin/imux",
		"a b":           "'a b'",
		"":              "''",
		"#{pane_id}":    "'#{pane_id}'",
	}
	for in, want := range cases {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %q, want %q", in, got, want)
		}
	}
	if EscapeFormat("a#b") != "a##b" {
		t.Fatal("EscapeFormat")
	}
	if EscapeMessage("50% #1") != "50%% ##1" {
		t.Fatal("EscapeMessage")
	}
	if d := DelimCmd("%3"); d[2] != Delim+"%%3" {
		t.Fatalf("DelimCmd: %q", d)
	}
}

func FuzzSectionsRows(f *testing.F) {
	f.Add("pre\n\t\timux:%1\na\tb\n\t\timux:%2\n", 3)
	f.Add("", 1)
	f.Fuzz(func(t *testing.T, s string, n int) {
		if n < 1 || n > 16 {
			n = 1 + (n&15+16)%16
		}
		for _, r := range Rows(s, n) {
			if len(r) != n {
				t.Fatalf("row has %d fields, want %d", len(r), n)
			}
		}
		total := 0
		for _, v := range Sections(s) {
			total += len(v)
		}
		if total > len(s) {
			t.Fatalf("sections larger than input")
		}
		_, _ = ParseVersion(s)
	})
}

// --- integration with a real tmux ------------------------------------------

func realClient(t *testing.T) *Client {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("real tmux tests: Windows runs psmux, whose server differs (see psmux_windows_test.go)")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	c := &Client{Bin: "tmux", Socket: "imux-test-" + randHex(t), Config: "/dev/null", Runner: ExecRunner{}}
	t.Cleanup(func() { _, _ = c.Do(Cmd{"kill-server"}) })
	return c
}

func randHex(t *testing.T) string {
	b := make([]byte, 6)
	f, err := os.Open("/dev/urandom")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Read(b); err != nil {
		t.Fatal(err)
	}
	const hex = "0123456789abcdef"
	s := make([]byte, 0, 12)
	for _, x := range b {
		s = append(s, hex[x>>4], hex[x&15])
	}
	return string(s)
}

func TestIntegrationRealTmux(t *testing.T) {
	c := realClient(t)
	ctx := context.Background()
	v, err := c.BinVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("tmux %v", v)
	if c.Running(ctx) {
		t.Fatal("server should not be running yet")
	}
	if _, err := c.Do(Cmd{"list-panes", "-a"}); !errors.Is(err, ErrNoServer) {
		t.Fatalf("want ErrNoServer, got %v", err)
	}
	out, err := c.Do(Cmd{"new-session", "-d", "-s", "it", "-x", "80", "-y", "24", "-P", "-F", "#{pane_id}", "sleep", "30"})
	if err != nil {
		t.Fatal(err)
	}
	pane := strings.TrimSpace(out)
	if !strings.HasPrefix(pane, "%") {
		t.Fatalf("pane id %q", pane)
	}
	// One exec: set options with awkward values, then read them back with delimiters.
	val := `[{"a":"claude"}] it's ; "q" end;`
	out, err = c.Do(
		Cmd{"set-option", "-p", "-t", pane, "@imux_trail", val},
		DelimCmd(pane),
		Cmd{"display-message", "-p", "-t", pane, "#{pane_id}\t#{@imux_trail}"},
		DelimCmd("panes"),
		Cmd{"list-panes", "-a", "-F", "#{pane_id}\t#{session_name}"},
	)
	if err != nil {
		t.Fatal(err)
	}
	sec := Sections(out)
	rows := Rows(sec[pane], 2)
	if len(rows) != 1 || rows[0][0] != pane || rows[0][1] != val {
		t.Fatalf("option round trip: %q (output %q)", sec[pane], out)
	}
	if r := Rows(sec["panes"], 2); len(r) != 1 || r[0][1] != "it" {
		t.Fatalf("panes: %q", sec["panes"])
	}
	if _, err := c.Do(Cmd{"set-option", "-p", "-t", "%999", "@x", "1"}); !errors.Is(err, ErrNoPane) {
		t.Fatalf("want ErrNoPane, got %v", err)
	}
	if _, err := c.Do(Cmd{"has-session", "-t", "=nosuch"}); !errors.Is(err, ErrNoSession) {
		t.Fatalf("want ErrNoSession, got %v", err)
	}
	// Quote produces strings tmux's own parser reads back verbatim.
	conf := filepath.Join(t.TempDir(), "q.conf")
	tricky := `it's "x" $HOME ~ \n`
	if err := os.WriteFile(conf, []byte("set -g @q "+Quote(tricky)+"\nset -g @d "+QuoteDouble(tricky)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Do(Cmd{"source-file", conf}); err != nil {
		t.Fatal(err)
	}
	for _, opt := range []string{"@q", "@d"} {
		out, err = c.Do(Cmd{"show-options", "-gv", opt})
		// tmux ≥ 3.4 reports an unexpanded $ as \$ (3.2 as $). Either way
		// $HOME must not have been expanded to a path.
		got := strings.ReplaceAll(strings.TrimSuffix(out, "\n"), `\$`, `$`)
		if err != nil || got != tricky {
			t.Fatalf("%s round trip: %q %v", opt, out, err)
		}
	}
	if !c.Running(ctx) {
		t.Fatal("server should be running")
	}
}

// envSpawner records what Spawn and SpawnEnv get.
type envSpawner struct {
	args, env []string
}

func (s *envSpawner) Spawn(_ string, args []string) error { s.args = args; return nil }

func (s *envSpawner) SpawnEnv(_ string, args, env []string) error {
	s.args, s.env = args, env
	return nil
}

// TestShowMenu (D54): before tmux 3.5 a menu goes to MenuPopup, which takes
// the mouse; from 3.5 tmux's own menu is spawned as it is.
// TestMessageTakesClient: display-message -c names a client from tmux 3.3;
// unknown versions are taken as new.
func TestMessageTakesClient(t *testing.T) {
	defer func() { msgClient.Once, msgClient.ok = sync.Once{}, false }()
	for _, c := range []struct {
		r    *fakeRunner
		want bool
	}{
		{&fakeRunner{out: "tmux 3.2a\n"}, false},
		{&fakeRunner{out: "tmux 3.3a\n"}, true},
		{&fakeRunner{out: "tmux 3.4\n"}, true},
		{&fakeRunner{err: errors.New("no tmux")}, true},
	} {
		msgClient.Once, msgClient.ok = sync.Once{}, false
		cl := &Client{Bin: "tmux", Socket: "s", Runner: c.r}
		if got := cl.MessageTakesClient(context.Background()); got != c.want {
			t.Errorf("%q %v: %v", c.r.out, c.r.err, got)
		}
	}
}

func TestShowMenu(t *testing.T) {
	defer func(f func(context.Context, *Client, Cmd) error) { MenuPopup = f }(MenuPopup)
	defer func() { menuMouse.Once, menuMouse.ok = sync.Once{}, false }()
	ctx := context.Background()
	var popped Cmd
	MenuPopup = func(_ context.Context, _ *Client, cmd Cmd) error { popped = cmd; return nil }
	for _, c := range []struct {
		version string
		popup   bool
	}{{"tmux 3.2a\n", true}, {"tmux 3.4\n", true}, {"tmux 3.5\n", false}} {
		menuMouse.Once, menuMouse.ok = sync.Once{}, false
		popped = nil
		sp := &envSpawner{}
		cl := &Client{Bin: "tmux", Socket: "s", Runner: &fakeRunner{out: c.version}, Spawner: sp}
		menu := append(cl.Menu(ctx), "-T", "Title", "--", "One", "o", "run-shell true")
		if err := cl.ShowMenu(ctx, menu); err != nil {
			t.Fatal(err)
		}
		if c.popup != (popped != nil) || c.popup == (sp.args != nil) || !cl.MenuTakesMouse(ctx) {
			t.Errorf("%s: popup %q, spawned %q", c.version, popped, sp.args)
		}
	}
	// SpawnEnv hands the environment to a Spawner that takes it.
	sp := &envSpawner{}
	cl := &Client{Bin: "tmux", Socket: "s", Spawner: sp}
	if err := cl.SpawnEnv([]string{"TMUX_PANE=%3"}, Cmd{"source-file", "-q", "/x"}); err != nil || !reflect.DeepEqual(sp.env, []string{"TMUX_PANE=%3"}) {
		t.Fatalf("env %q, %v", sp.env, err)
	}
}
