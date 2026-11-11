//go:build windows

package tmux

import (
	"reflect"
	"strings"
	"testing"
)

func TestShQuotePowerShell(t *testing.T) {
	for in, want := range map[string]string{
		"":                       "''",
		"plain":                  "plain",
		`C:\Program\imux.exe`:    `C:\Program\imux.exe`,
		"C:/x/imux.exe":          "C:/x/imux.exe",
		"--pane":                 "--pane",
		"%3~proj-1a2b":           "'%3~proj-1a2b'", // ~ is not among the safe marks
		"a=b+c":                  "a=b+c",
		"a b":                    "'a b'",
		"it's":                   "'it''s'",
		"'":                      "''''",
		"$env:HOME":              "'$env:HOME'",
		"a;b":                    "'a;b'",
		"a,b":                    "'a,b'",
		"@x":                     "'@x'",
		"{x}":                    "'{x}'",
		"a`b":                    "'a`b'",
		`say "hi"`:               `'say "hi"'`,
		"#{pane_id}":             "'#{pane_id}'",
		"ğüş":                    "'ğüş'",
		"C:\\a b\\it's here.txt": `'C:\a b\it''s here.txt'`,
	} {
		if got := ShQuote(in); got != want {
			t.Errorf("ShQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestShCallPowerShell(t *testing.T) {
	for in, want := range map[string]string{
		`C:\imux\imux.exe`:          "& C:/imux/imux.exe",
		`C:\Program Files\imux.exe`: "& 'C:/Program Files/imux.exe'",
		"/opt/imux/bin/imux":        "& /opt/imux/bin/imux",
		`C:\it's\imux.exe`:          "& 'C:/it''s/imux.exe'",
	} {
		if got := ShCall(in); got != want {
			t.Errorf("ShCall(%q) = %s, want %s", in, got, want)
		}
	}
}

// psArg writes the arguments Windows PowerShell 5.1 would mangle (an empty
// one, one with a double quote) in the form that reaches the program intact.
func TestPsArg(t *testing.T) {
	for in, want := range map[string]string{
		"":           `'""'`,
		"x":          "x",
		"a b":        "'a b'",
		"it's":       "'it''s'",
		`say "hi"`:   `'say \"hi\"'`,
		`"`:          `'\"'`,
		`it's "x"`:   `'it''s \"x\"'`,
		"--client":   "--client",
		"#{pane_id}": "'#{pane_id}'",
	} {
		if got := psArg(in); got != want {
			t.Errorf("psArg(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestShJoinPowerShell(t *testing.T) {
	if got := ShJoin(); got != "" {
		t.Errorf("ShJoin() = %q", got)
	}
	if got, want := ShJoin(`C:\a b\imux.exe`), "& 'C:/a b/imux.exe'"; got != want {
		t.Errorf("program only: %s, want %s", got, want)
	}
	got := ShJoin(`C:\a b\imux.exe`, "__menu", "it's", "", `q"x`, "--pane", "%3")
	want := `& 'C:/a b/imux.exe' __menu 'it''s' '""' 'q\"x' --pane %3`
	if got != want {
		t.Errorf("ShJoin = %s\nwant      %s", got, want)
	}
	// The command line runs silenced in a binding.
	if !strings.HasPrefix(Silent, " ") || !strings.Contains(Silent, "Out-Null") || strings.Contains(Silent, "/dev/null") {
		t.Errorf("Silent %q is not PowerShell", Silent)
	}
}

func TestShEnvPowerShell(t *testing.T) {
	if got, want := ShEnv("IMUX_PANE", "%3"), "$env:IMUX_PANE=%3; "; got != want {
		t.Errorf("ShEnv = %q, want %q", got, want)
	}
	if got, want := ShEnv("X", "a b"), "$env:X='a b'; "; got != want {
		t.Errorf("ShEnv = %q, want %q", got, want)
	}
}

// psmux does not join single-quoted pieces into one word, so a word with a
// single quote in it is double quoted.
func TestQuoteSingleQuotePsmux(t *testing.T) {
	for in, want := range map[string]string{
		"it's":  `"it's"`,
		`a"b'c`: `"a\"b'c"`,
		"a b":   "'a b'",
	} {
		if got := Quote(in); got != want {
			t.Errorf("Quote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestLoginShellPowerShell(t *testing.T) {
	for in, want := range map[string][]string{
		`C:\Program Files\PowerShell\7\pwsh.exe`:                    {`C:\Program Files\PowerShell\7\pwsh.exe`, "-NoLogo"},
		`C:\Windows\System32\WindowsPowerShell\v1.0\PowerShell.exe`: {`C:\Windows\System32\WindowsPowerShell\v1.0\PowerShell.exe`, "-NoLogo"},
		`C:\Program Files\Git\bin\bash.exe`:                         {`C:\Program Files\Git\bin\bash.exe`},
	} {
		if got := LoginShell(in); !reflect.DeepEqual(got, want) {
			t.Errorf("LoginShell(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNativeTabs(t *testing.T) {
	cases := map[string]bool{
		"tmux 3.4.0\r\npsmux 3.4.0 (20aa1d7 2026-10-13)": true,  // the 3.4.0 release
		"tmux 3.4.0\r\npsmux 3.4.0 (32aa803 2026-11-10)": false, // a later build of main
		"tmux 3.4.0":                             true, // no build date
		"tmux 3.3":                               true,
		"tmux 3.5\npsmux 3.5.0 (abc 2026-12-01)": false,
	}
	for in, want := range cases {
		v, ok := ParseVersion(in)
		if !ok {
			t.Fatalf("ParseVersion(%q) failed", in)
		}
		if got := nativeTabs(v); got != want {
			t.Errorf("nativeTabs(%q) = %v, want %v (built %q)", in, got, want, v.Built)
		}
	}
}
