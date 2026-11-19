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
	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
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

// maxName is the longest name.
const maxName = 32

// accents are the letters Slug takes the accents from, before ToLower (which
// makes "İ" an "i" and a combining dot) and after it ("É").
var accents = strings.NewReplacer("ç", "c", "ğ", "g", "ı", "i", "ö", "o", "ş", "s", "ü", "u",
	"Ç", "c", "Ğ", "g", "İ", "i", "Ö", "o", "Ş", "s", "Ü", "u",
	"ä", "a", "ß", "ss", "é", "e", "è", "e", "á", "a", "ñ", "n")

// Slug makes a name of free text, as the New environment dialog does:
// "GLM Coding" → "glm-coding", "İş_proxy" → "is_proxy" ("" when nothing is
// left). Letters lose their case and accents, `_` stays, anything else is
// a `-`; at most 32.
func Slug(text string) string {
	var b strings.Builder
	dash := false
	for _, r := range accents.Replace(strings.ToLower(accents.Replace(text))) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' && b.Len() > 0 {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		} else {
			dash = true
		}
	}
	s := b.String()
	if len(s) > maxName {
		s = s[:maxName]
	}
	return strings.TrimRight(s, "-_")
}

// Dir is the folder of the environments.
func Dir() string { return filepath.Join(paths.Home(), "env") }

// Path is environment name's file.
func Path(name string) string { return filepath.Join(Dir(), name+Ext) }

// Reserved reports whether key is i-mux's or the shell's own: an
// environment never sets it. Windows' names are not case-sensitive (Path is
// PATH), and Windows has its own folders and shell to keep.
func Reserved(key string) bool {
	if !sysx.POSIXShell {
		key = strings.ToUpper(key)
		switch key {
		case "USERPROFILE", "APPDATA", "LOCALAPPDATA", "SYSTEMROOT", "COMSPEC", "TEMP", "TMP", "PATHEXT":
			return true
		}
	}
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

// Shell is the kind of shell an apply file is written for.
type Shell int

// The shells i-mux types an apply file into.
const (
	ShellSh         Shell = iota // a POSIX shell: bash, zsh, sh…
	ShellFish                    // fish
	ShellPowerShell              // pwsh or Windows PowerShell
	ShellCmd                     // cmd.exe
)

// ShellOf is the kind of shell (a path or a name, with or without .exe).
func ShellOf(shell string) Shell {
	switch strings.TrimSuffix(strings.ToLower(filepath.Base(shell)), ".exe") {
	case "fish":
		return ShellFish
	case "pwsh", "powershell":
		return ShellPowerShell
	case "cmd":
		return ShellCmd
	}
	return ShellSh
}

// Fish reports whether shell (a path or a name) is fish.
func Fish(shell string) bool { return ShellOf(shell) == ShellFish }

// ext is the apply file's extension for sh: cmd.exe runs only a .cmd as a batch file.
func (sh Shell) ext() string {
	switch sh {
	case ShellPowerShell:
		return ".ps1"
	case ShellCmd:
		return ".cmd"
	}
	return ""
}

// Line is what is typed into the shell to run the apply file at path: only its path, never a
// value. PowerShell reads it into a script block, which no execution policy stops; cmd.exe calls
// it, so it runs in the same shell.
func (sh Shell) Line(path string) string {
	switch sh {
	case ShellFish:
		return " source " + ShQuote(path)
	case ShellPowerShell:
		return " . ([scriptblock]::Create([IO.File]::ReadAllText(" + PSQuote(path) + ")))"
	case ShellCmd:
		return ` call "` + path + `"`
	}
	return " . " + ShQuote(path)
}

// Script is the text a shell sources to take env (nil: none) instead of
// the keys in prev (the pane's previous environment): its variables
// exported, prev's other keys unset, and the file deleted by its last line.
// cmd.exe cannot hold a line break in a variable, so a value with one is
// left out there.
func Script(env *Env, prev []string, sh Shell, self string) string {
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
		switch sh {
		case ShellFish:
			fmt.Fprintf(&b, "set -e %s\n", k)
		case ShellPowerShell:
			fmt.Fprintf(&b, "Remove-Item Env:%s -ErrorAction SilentlyContinue\n", k)
		case ShellCmd:
			fmt.Fprintf(&b, "@set \"%s=\"\r\n", k)
		default:
			fmt.Fprintf(&b, "unset %s\n", k)
		}
	}
	if env != nil {
		for _, v := range env.Vars {
			switch sh {
			case ShellFish:
				fmt.Fprintf(&b, "set -gx %s %s\n", v.Key, FishQuote(v.Value))
			case ShellPowerShell:
				fmt.Fprintf(&b, "$env:%s = %s\n", v.Key, PSQuote(v.Value))
			case ShellCmd:
				if !strings.ContainsAny(v.Value, "\r\n") {
					fmt.Fprintf(&b, "@set \"%s=%s\"\r\n", v.Key, CmdQuote(v.Value))
				}
			default:
				fmt.Fprintf(&b, "export %s=%s\n", v.Key, ShQuote(v.Value))
			}
		}
	}
	switch sh {
	case ShellPowerShell:
		fmt.Fprintf(&b, "Remove-Item -LiteralPath %s -Force -ErrorAction SilentlyContinue\n", PSQuote(self))
	case ShellCmd:
		// A batch file deleting itself: the goto ends it first, so cmd.exe reads no more of it.
		// Every line starts with @, so cmd.exe echoes none (a value would show in the pane),
		// without turning echo off for the shell.
		b.WriteString("@(goto) 2>nul & del \"%~f0\"\r\n")
	default:
		fmt.Fprintf(&b, "command rm -f -- %s\n", ShQuote(self))
	}
	return b.String()
}

// PSQuote quotes s for PowerShell: in single quotes, a quote doubled.
func PSQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// CmdQuote is s inside a batch file's set "K=…": % doubled, which a batch file would expand;
// the quotes keep & | < > ^ as they are, and set takes everything to the last quote.
func CmdQuote(s string) string { return strings.ReplaceAll(s, "%", "%%") }

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
func WriteScript(env *Env, prev []string, sh Shell) (string, error) {
	if err := paths.EnsureDir(Dir()); err != nil {
		return "", err
	}
	var r [8]byte
	if _, err := rand.Read(r[:]); err != nil {
		return "", err
	}
	p := filepath.Join(Dir(), applyPrefix+hex.EncodeToString(r[:])+sh.ext())
	if err := paths.WriteFile(p, []byte(Script(env, prev, sh, p))); err != nil {
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
