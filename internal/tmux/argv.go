package tmux

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
)

// ArgvForm says how the command lines i-mux gives its engine run (§9.2a):
// false, as POSIX sh lines (macOS, Linux); true, in i-mux-core's argv form,
// with no shell (Windows, D85). IMUX_ARGV_FORM=1 asks for it on macOS and
// Linux too, where i-mux-core runs it and tmux does not. Tests set it.
var ArgvForm = !sysx.POSIXShell || os.Getenv("IMUX_ARGV_FORM") == "1"

// ShellLine is the command run-shell runs for argv with its output shown:
// one sh line, or the argv form (["prog", "arg", …]).
func ShellLine(argv ...string) string {
	if ArgvForm {
		return argvJSON(argv, false)
	}
	return ShJoin(argv...)
}

// Program is argv as a popup's or a pane's program, the arguments after
// display-popup's or split-window's options: one sh line, or, in the argv
// form, the words themselves, which the engine runs without a shell. argv
// holds the program and at least one argument (one word alone is a shell
// command to the engine).
func Program(argv ...string) []string {
	if ArgvForm {
		return append([]string(nil), argv...)
	}
	return []string{ShJoin(argv...)}
}

// QuoteProgram is Program as words of a tmux command string.
func QuoteProgram(argv ...string) string {
	p := Program(argv...)
	for i, a := range p {
		p[i] = Quote(a)
	}
	return strings.Join(p, " ")
}

// argvJSON is the argv form of i-mux-core's run-shell and if-shell (its X1):
// a JSON array, or with quiet an object that also drops the output and the
// exit status, as `>/dev/null 2>&1 || true` does.
func argvJSON(argv []string, quiet bool) string {
	var v any = argv
	if quiet {
		v = struct {
			Argv  []string `json:"argv"`
			Quiet bool     `json:"quiet"`
		}{argv, true}
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimSuffix(b.String(), "\n")
}

// shWords splits words the way sh does i-mux's (UICommand's args are sh
// words, some of them several in one string): blanks separate them, quotes
// and a backslash are taken away, and a format (#{…}) stays whole, its
// blanks too, since run-shell expands it before sh sees the line. The words
// use no other sh syntax (expansions, globs, operators).
func shWords(args []string) []string {
	var out []string
	for _, a := range args {
		out = append(out, splitWords(a)...)
	}
	return out
}

func splitWords(s string) []string {
	var (
		words []string
		cur   strings.Builder
		in    bool // a word has started
		depth int  // formats open
	)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case depth > 0:
			cur.WriteByte(c)
			switch {
			case c == '{' && s[i-1] == '#':
				depth++
			case c == '}':
				depth--
			}
		case c == ' ' || c == '\t' || c == '\n':
			if in {
				words = append(words, cur.String())
				cur.Reset()
				in = false
			}
		case c == '\'':
			in = true
			j := strings.IndexByte(s[i+1:], '\'')
			if j < 0 {
				j = len(s) - i - 1
			}
			cur.WriteString(s[i+1 : i+1+j])
			i += j + 1
		case c == '"':
			in = true
			for i++; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' && i+1 < len(s) && strings.IndexByte("\\\"$`", s[i+1]) >= 0 {
					i++
				}
				cur.WriteByte(s[i])
			}
		case c == '\\' && i+1 < len(s):
			in = true
			i++
			cur.WriteByte(s[i])
		case c == '#' && i+1 < len(s) && s[i+1] == '{':
			in = true
			depth++
			cur.WriteString("#{")
			i++
		default:
			in = true
			cur.WriteByte(c)
		}
	}
	if in {
		words = append(words, cur.String())
	}
	return words
}
