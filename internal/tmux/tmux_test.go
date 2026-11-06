package tmux

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
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
	want := []string{"-L", "s", "-f", "/conf", "set", "-p", "@x", `a\;`, ";", "display", "-p", "mid;dle"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	c.Config = ""
	if got := c.Argv(Cmd{"ls"}); !reflect.DeepEqual(got, []string{"-L", "s", "ls"}) {
		t.Fatalf("no config: %q", got)
	}
}

func TestRunClassifiesErrors(t *testing.T) {
	cases := []struct {
		stderr string
		want   error
	}{
		{"no server running on /tmp/tmux-501/ajan\n", ErrNoServer},
		{"error connecting to /tmp/tmux-1000/ajan (No such file or directory)\n", ErrNoServer},
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
	if !reflect.DeepEqual(got[1].Args, []string{"-L", "s", "display", "-p", "x", ";", "set", "-g", "@y", "1"}) {
		t.Fatalf("args %q", got[1].Args)
	}
	if got[0].Pid != os.Getpid() || got[0].T == 0 {
		t.Fatalf("record metadata: %+v", got[0])
	}
	fi, _ := os.Stat(rec)
	if fi.Mode().Perm() != 0o600 {
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
}

func TestInside(t *testing.T) {
	c := &Client{Socket: "ajan"}
	if !c.Inside("/tmp/tmux-501/ajan,1234,0") {
		t.Fatal("should be inside")
	}
	if c.Inside("/tmp/tmux-501/default,1234,0") || c.Inside("") {
		t.Fatal("should not be inside")
	}
}

func TestQuote(t *testing.T) {
	cases := map[string]string{
		"%3":            "%3",
		"/usr/bin/ajan": "/usr/bin/ajan",
		"a b":           "'a b'",
		"it's":          `'it'\''s'`,
		"":              "''",
		"#{pane_id}":    "'#{pane_id}'",
	}
	for in, want := range cases {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %q, want %q", in, got, want)
		}
	}
	if got := ShJoin("/a b/ajan", "__menu", "it's"); got != `'/a b/ajan' __menu 'it'"'"'s'` {
		t.Fatalf("ShJoin: %s", got)
	}
	if EscapeFormat("a#b") != "a##b" {
		t.Fatal("EscapeFormat")
	}
}

func FuzzSectionsRows(f *testing.F) {
	f.Add("pre\n\x1eajan:%1\na\tb\n\x1eajan:%2\n", 3)
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
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	c := &Client{Bin: "tmux", Socket: "ajan-test-" + randHex(t), Config: "/dev/null", Runner: ExecRunner{}}
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
		Cmd{"set-option", "-p", "-t", pane, "@ajan_trail", val},
		DelimCmd(pane),
		Cmd{"display-message", "-p", "-t", pane, "#{pane_id}\t#{@ajan_trail}"},
		DelimCmd("panes"),
		Cmd{"list-panes", "-a", "-F", "#{pane_id}\t#{session_name}"},
	)
	if err != nil {
		t.Fatal(err)
	}
	sec := Sections(out)
	rows := Rows(sec[pane], 2)
	if len(rows) != 1 || rows[0][0] != pane || rows[0][1] != val {
		t.Fatalf("option round trip: %q", sec[pane])
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
		if err != nil || strings.TrimSuffix(out, "\n") != tricky {
			t.Fatalf("%s round trip: %q %v", opt, out, err)
		}
	}
	if !c.Running(ctx) {
		t.Fatal("server should be running")
	}
}
