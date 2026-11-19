// Package envfile keeps the user's environments (D84): named sets of
// variables in ~/.imux/env/<name>.env (0600) that a pane's programs run
// with. It parses them, lists them, and writes the script that makes a
// shell take one. Values are never logged or shown.
package envfile

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/i18n"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
)

// Ext is an environment file's extension.
const Ext = ".env"

// applyPrefix starts the name of a shell's apply file (dot files are not
// environments).
const applyPrefix = ".apply-"

var (
	nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	keyRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// ErrName is returned for a name that is not `[a-z0-9][a-z0-9_-]{0,31}`.
var ErrName = errors.New("envfile: bad name")

// ValidName reports whether name can name an environment.
func ValidName(name string) bool { return nameRe.MatchString(name) }

// Dir is the folder of the environments.
func Dir() string { return filepath.Join(paths.Home(), "env") }

// Path is environment name's file.
func Path(name string) string { return filepath.Join(Dir(), name+Ext) }

// Reserved reports whether key is i-mux's or the shell's own: an
// environment never sets it.
func Reserved(key string) bool {
	switch key {
	case "PATH", "HOME", "SHELL", "PWD", "OLDPWD", "TMUX", "TMUX_PANE", "SHLVL", "USER", "LOGNAME":
		return true
	}
	return strings.HasPrefix(key, "IMUX_")
}

// Var is one variable.
type Var struct {
	Key, Value string
}

// Problem is a line that was skipped.
type Problem struct {
	Line   int // 1-based
	Reason string
}

// Env is a parsed environment.
type Env struct {
	Name     string
	Vars     []Var // in the file's order; a key set twice keeps its last value
	Problems []Problem
}

// Keys are the environment's keys, in order.
func (e *Env) Keys() []string {
	out := make([]string, len(e.Vars))
	for i, v := range e.Vars {
		out[i] = v.Key
	}
	return out
}

// Map is the environment as a map.
func (e *Env) Map() map[string]string {
	m := make(map[string]string, len(e.Vars))
	for _, v := range e.Vars {
		m[v.Key] = v.Value
	}
	return m
}

// Reasons of a skipped line.
const (
	BadLine     = "not KEY=VALUE"
	BadKey      = "not a variable name"
	ReservedKey = "set by i-mux or the shell"
	BadQuote    = "unclosed quote"
	BadValue    = "NUL byte"
)

// Parse reads an environment: KEY=VALUE lines, optionally after `export `;
// blank lines and # comments are skipped. A value in single quotes is
// taken as it is; in double quotes \" and \\ are unescaped. There is no $
// expansion. Lines that are not understood, and reserved keys, are
// reported and skipped.
func Parse(name, text string) *Env {
	e := &Env{Name: name}
	at := map[string]int{}
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimSpace(strings.TrimSuffix(sc.Text(), "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "export"); ok && rest != "" && (rest[0] == ' ' || rest[0] == '\t') {
			line = strings.TrimSpace(rest)
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			e.Problems = append(e.Problems, Problem{n, BadLine})
			continue
		}
		k = strings.TrimSpace(k)
		switch {
		case !keyRe.MatchString(k):
			e.Problems = append(e.Problems, Problem{n, BadKey})
			continue
		case Reserved(k):
			e.Problems = append(e.Problems, Problem{n, ReservedKey})
			continue
		}
		val, ok := unquote(strings.TrimSpace(v))
		if !ok {
			e.Problems = append(e.Problems, Problem{n, BadQuote})
			continue
		}
		if strings.IndexByte(val, 0) >= 0 {
			e.Problems = append(e.Problems, Problem{n, BadValue})
			continue
		}
		if i, ok := at[k]; ok {
			e.Vars[i].Value = val
			continue
		}
		at[k] = len(e.Vars)
		e.Vars = append(e.Vars, Var{k, val})
	}
	if err := sc.Err(); err != nil {
		e.Problems = append(e.Problems, Problem{n + 1, BadLine})
	}
	return e
}

// unquote takes a value out of its quotes. An unquoted value ends at a
// " #" comment.
func unquote(v string) (string, bool) {
	if v == "" {
		return "", true
	}
	switch v[0] {
	case '\'':
		end := strings.IndexByte(v[1:], '\'')
		if end < 0 {
			return "", false
		}
		return v[1 : 1+end], trailing(v[2+end:])
	case '"':
		var b strings.Builder
		for i := 1; i < len(v); i++ {
			c := v[i]
			switch {
			case c == '\\' && i+1 < len(v) && (v[i+1] == '"' || v[i+1] == '\\'):
				b.WriteByte(v[i+1])
				i++
			case c == '"':
				return b.String(), trailing(v[i+1:])
			default:
				b.WriteByte(c)
			}
		}
		return "", false
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v, true
}

// trailing reports whether what follows a closing quote is nothing or a
// comment.
func trailing(s string) bool {
	s = strings.TrimSpace(s)
	return s == "" || strings.HasPrefix(s, "#")
}

// Read loads environment name.
func Read(name string) (*Env, error) {
	if !ValidName(name) {
		return nil, ErrName
	}
	b, err := os.ReadFile(Path(name))
	if err != nil {
		return nil, err
	}
	return Parse(name, string(b)), nil
}

// List reads every environment, by name.
func List() []*Env {
	des, err := os.ReadDir(Dir())
	if err != nil {
		return nil
	}
	var out []*Env
	for _, d := range des {
		name, ok := strings.CutSuffix(d.Name(), Ext)
		if !ok || d.IsDir() || !ValidName(name) {
			continue
		}
		if e, err := Read(name); err == nil {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Exists reports whether environment name has a file.
func Exists(name string) bool {
	if !ValidName(name) {
		return false
	}
	_, err := os.Stat(Path(name))
	return err == nil
}

// Create writes environment name with the commented template; an existing
// one is left as it is. It returns the file's path.
func Create(name string) (string, error) {
	if !ValidName(name) {
		return "", ErrName
	}
	if err := paths.EnsureDir(Dir()); err != nil {
		return "", err
	}
	p := Path(name)
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	return p, paths.WriteFile(p, []byte(Template(name)))
}

// Template is a new environment's text: what it is, and examples.
func Template(name string) string {
	var b strings.Builder
	for _, l := range strings.Split(i18n.T("env.template", name), "\n") {
		b.WriteString(strings.TrimRight("# "+l, " ") + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

// Delete removes environment name.
func Delete(name string) error {
	if !ValidName(name) {
		return ErrName
	}
	return os.Remove(Path(name))
}

// Fish reports whether shell (a path or a name) is fish.
func Fish(shell string) bool { return filepath.Base(shell) == "fish" }

// Script is the text a shell sources to take env (nil: none) instead of
// the keys in prev (the pane's previous environment): its variables
// exported, prev's other keys unset, and the file deleted by its last line.
func Script(env *Env, prev []string, fish bool, self string) string {
	var b strings.Builder
	set := map[string]bool{}
	if env != nil {
		for _, v := range env.Vars {
			set[v.Key] = true
		}
	}
	for _, k := range prev {
		if set[k] || !keyRe.MatchString(k) || Reserved(k) {
			continue
		}
		if fish {
			fmt.Fprintf(&b, "set -e %s\n", k)
		} else {
			fmt.Fprintf(&b, "unset %s\n", k)
		}
	}
	if env != nil {
		for _, v := range env.Vars {
			if fish {
				fmt.Fprintf(&b, "set -gx %s %s\n", v.Key, FishQuote(v.Value))
			} else {
				fmt.Fprintf(&b, "export %s=%s\n", v.Key, ShQuote(v.Value))
			}
		}
	}
	fmt.Fprintf(&b, "command rm -f -- %s\n", ShQuote(self))
	return b.String()
}

// ShQuote quotes s for a POSIX shell: in single quotes, a quote as '\”.
func ShQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// FishQuote quotes s for fish: in single quotes, \ and ' escaped.
func FishQuote(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

// WriteScript writes the apply file for a shell (Script) and returns its
// path.
func WriteScript(env *Env, prev []string, fish bool) (string, error) {
	if err := paths.EnsureDir(Dir()); err != nil {
		return "", err
	}
	var r [8]byte
	if _, err := rand.Read(r[:]); err != nil {
		return "", err
	}
	p := filepath.Join(Dir(), applyPrefix+hex.EncodeToString(r[:]))
	if err := paths.WriteFile(p, []byte(Script(env, prev, fish, p))); err != nil {
		return "", err
	}
	return p, nil
}

// SweepScripts deletes apply files older than age: a shell that never ran
// one (it was busy after all, or closed).
func SweepScripts(age time.Duration) {
	des, err := os.ReadDir(Dir())
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-age)
	for _, d := range des {
		if !strings.HasPrefix(d.Name(), applyPrefix) {
			continue
		}
		if fi, err := d.Info(); err == nil && fi.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(Dir(), d.Name()))
		}
	}
}
