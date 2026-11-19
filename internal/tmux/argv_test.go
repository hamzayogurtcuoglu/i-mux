package tmux

import (
	"os"
	"reflect"
	"testing"
)

// TestMain runs the tests in the sh form whatever the system, so the
// goldens are the same everywhere; the argv form's tests ask for it.
func TestMain(m *testing.M) {
	ArgvForm = false
	os.Exit(m.Run())
}

// withArgvForm gives the test the command lines of a system without a
// POSIX shell (§9.2a).
func withArgvForm(t *testing.T) {
	t.Helper()
	old := ArgvForm
	ArgvForm = true
	t.Cleanup(func() { ArgvForm = old })
}

// UICommand's args are sh words: the argv form splits them as sh would.
func TestSplitWords(t *testing.T) {
	cases := map[string][]string{
		"--pane #{pane_id} --client #{client_name}": {"--pane", "#{pane_id}", "--client", "#{client_name}"},
		ShQuote("/home/test/my bus/it's.txt"):       {"/home/test/my bus/it's.txt"},
		`"a \"b\" \$c" d\ e`:                        {`a "b" $c`, "d e"},
		"#{?#{==:#{a},b},x y,z} w":                  {"#{?#{==:#{a},b},x y,z}", "w"},
		"''":                                        {""},
		"  ":                                        nil,
	}
	for in, want := range cases {
		if got := splitWords(in); !reflect.DeepEqual(got, want) {
			t.Errorf("splitWords(%q) = %q, want %q", in, got, want)
		}
	}
}

const winSelf = `C:\Users\test\AppData\Local\Programs\i-mux\imux.exe`

func TestArgvFormCommands(t *testing.T) {
	withArgvForm(t)
	cases := []struct{ got, want string }{
		{
			RunShell(winSelf, "__menu", "--pane #{pane_id} --client #{client_name}"),
			`run-shell -b '{"argv":["C:\\Users\\test\\AppData\\Local\\Programs\\i-mux\\imux.exe","__menu","--pane","#{pane_id}","--client","#{client_name}"],"quiet":true}'`,
		},
		{
			ShellLine(winSelf, "__quickpick", "--run", "a<b"),
			`["C:\\Users\\test\\AppData\\Local\\Programs\\i-mux\\imux.exe","__quickpick","--run","a<b"]`,
		},
		{
			QuoteProgram(winSelf, "__history", "--pane", "%3", "it's"),
			`'C:\Users\test\AppData\Local\Programs\i-mux\imux.exe' __history --pane %3 'it'\''s'`,
		},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("got  %s\nwant %s", c.got, c.want)
		}
	}
	if got := Program(winSelf, "limits", "--pager"); !reflect.DeepEqual(got, []string{winSelf, "limits", "--pager"}) {
		t.Errorf("Program = %q", got)
	}
}

// The generated config without a POSIX shell: every run-shell in the argv
// form, the Windows paths quoted for tmux.
func TestGenerateConfArgvGolden(t *testing.T) {
	withArgvForm(t)
	in := confInput("3.8")
	in.Self = winSelf
	in.LocalConf = `C:\Users\test\.imux\tmux.local.conf`
	golden(t, "tmux-3.8-argv.conf.golden", GenerateConf(in))
}
