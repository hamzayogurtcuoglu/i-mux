// Package launch starts agent CLIs from launch specs.
//
// A spec is written to ~/.ajan/launch/<rand>.json (0600) and consumed by
// `ajan __exec <spec>`, which deletes it, sets PATH/AJAN_PANE/AJAN_AGENT,
// changes directory and replaces itself with the agent via execve. argv is
// never passed through a shell, and the prompt never touches a tmux command
// line (no quoting, no size limit there).
package launch

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/session"
)

// Defaults.
const (
	DefaultMaxArgBytes = 102400
	// MaxTotalArgBytes bounds argv + environment (macOS ARG_MAX is 1 MiB).
	MaxTotalArgBytes = 900 << 10
)

// Spec describes one agent launch.
type Spec struct {
	Agent       session.AgentID   `json:"agent"`
	Bin         string            `json:"bin"`  // absolute path
	Args        []string          `json:"args"` // fully expanded (includes the prompt in argv mode)
	Dir         string            `json:"dir"`
	Env         map[string]string `json:"env,omitempty"` // at least PATH
	Pane        string            `json:"pane,omitempty"`
	MaxArgBytes int               `json:"max_arg_bytes,omitempty"`
	Fallback    *Spec             `json:"fallback,omitempty"` // used by the pane-died hook when a resume fails fast
}

// ErrTooLarge is returned when an argument or the whole argv is too big.
var ErrTooLarge = errors.New("launch: argument list too large")

// Validate checks the spec before exec.
func (s *Spec) Validate() error {
	if s.Bin == "" || !filepath.IsAbs(s.Bin) {
		return fmt.Errorf("launch: binary path %q is not absolute", s.Bin)
	}
	max := s.MaxArgBytes
	if max <= 0 {
		max = DefaultMaxArgBytes
	}
	total := len(s.Bin) + 1
	for _, a := range s.Args {
		if len(a) > max {
			return fmt.Errorf("%w: one argument is %d bytes (limit %d)", ErrTooLarge, len(a), max)
		}
		if strings.IndexByte(a, 0) >= 0 {
			return fmt.Errorf("launch: argument contains a NUL byte")
		}
		total += len(a) + 1
	}
	if total > MaxTotalArgBytes {
		return fmt.Errorf("%w: %d bytes in total", ErrTooLarge, total)
	}
	return nil
}

// Argv returns the full argument vector (argv[0] is the binary path).
func (s *Spec) Argv() []string { return append([]string{s.Bin}, s.Args...) }

// Environ returns base with the spec's overrides plus AJAN_PANE and
// AJAN_AGENT. Later duplicates are removed so overrides win.
func (s *Spec) Environ(base []string) []string {
	over := map[string]string{}
	for k, v := range s.Env {
		over[k] = v
	}
	if s.Pane != "" {
		over["AJAN_PANE"] = s.Pane
	}
	if s.Agent != "" {
		over["AJAN_AGENT"] = string(s.Agent)
	}
	out := make([]string, 0, len(base)+len(over))
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if _, ok := over[k]; ok {
			continue
		}
		out = append(out, kv)
	}
	keys := make([]string, 0, len(over))
	for k := range over {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, k+"="+over[k])
	}
	return out
}

// Write stores the spec in ~/.ajan/launch with mode 0600 and returns its path.
func Write(s *Spec) (string, error) {
	if err := paths.EnsureDir(paths.Launch()); err != nil {
		return "", err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	var r [8]byte
	if _, err := rand.Read(r[:]); err != nil {
		return "", err
	}
	p := filepath.Join(paths.Launch(), hex.EncodeToString(r[:])+".json")
	if err := paths.WriteFile(p, b); err != nil {
		return "", err
	}
	return p, nil
}

// Read loads a spec. With remove, the file is deleted after reading.
func Read(path string, remove bool) (*Spec, error) {
	b, err := os.ReadFile(path)
	if remove {
		os.Remove(path)
	}
	if err != nil {
		return nil, err
	}
	var s Spec
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("launch spec %s: %w", filepath.Base(path), err)
	}
	return &s, nil
}

// Sweep deletes leftover specs older than age.
func Sweep(age time.Duration) {
	ents, err := os.ReadDir(paths.Launch())
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-age)
	for _, e := range ents {
		if fi, err := e.Info(); err == nil && fi.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(paths.Launch(), e.Name()))
		}
	}
}

// execFunc is syscall.Exec; tests replace it.
var execFunc = syscall.Exec

// Exec validates the spec, changes directory and replaces the current
// process with the agent. It returns only on error.
func Exec(s *Spec) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.Dir != "" {
		if err := os.Chdir(s.Dir); err != nil {
			return fmt.Errorf("launch: %w", err)
		}
	}
	env := s.Environ(os.Environ())
	if s.Dir != "" {
		env = setEnv(env, "PWD", s.Dir)
	}
	return execFunc(s.Bin, s.Argv(), env)
}

func setEnv(env []string, k, v string) []string {
	out := env[:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, k+"=") {
			out = append(out, kv)
		}
	}
	return append(out, k+"="+v)
}
