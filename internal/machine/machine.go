// Package machine is other computers' i-mux, reached over SSH (ARCHITECTURE
// "Machines", D73): the machines the user added, the ssh commands that reach
// them, and what they report, read as untrusted input.
package machine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
)

// Machine is a computer the user added: its name in i-mux and its SSH
// destination.
type Machine struct {
	Name   string    `json:"name"`
	Target string    `json:"target"`
	Added  time.Time `json:"added"`
}

var (
	nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	// A destination: user@host, a ~/.ssh/config host, [::1], or
	// ssh://user@host:port. No spaces, quotes, options or shell characters.
	targetRe = regexp.MustCompile(`^(ssh://)?[A-Za-z0-9._@\[\]:%-]{1,253}$`)
)

// Errors of a machine's name and destination.
var (
	ErrName   = errors.New("a machine's name is lowercase letters, digits and dashes (at most 32), starting with a letter or digit")
	ErrTarget = errors.New("not an SSH destination: user@host, a host from ~/.ssh/config, or ssh://user@host:port")
	ErrExists = errors.New("a machine with that name is already added")
	ErrNone   = errors.New("no machine with that name")
)

// CheckName reports a name i-mux can use for a machine.
func CheckName(name string) error {
	if !nameRe.MatchString(name) {
		return ErrName
	}
	return nil
}

// CheckTarget reports an SSH destination that can only be read as one: it
// never starts with "-" (ssh would take it for an option, such as
// -oProxyCommand=…) and holds no character a shell or ssh treats specially.
func CheckTarget(target string) error {
	if strings.HasPrefix(target, "-") || !targetRe.MatchString(target) || strings.Contains(target, "@-") {
		return ErrTarget
	}
	return nil
}

// NameFor is a machine name made from a destination: its host's first
// label ("ali@sunucu.example.com" → "sunucu").
func NameFor(target string) string {
	h := strings.TrimPrefix(target, "ssh://")
	if i := strings.LastIndex(h, "@"); i >= 0 {
		h = h[i+1:]
	}
	h = strings.Trim(strings.SplitN(strings.TrimPrefix(h, "["), ":", 2)[0], "[]")
	h = strings.ToLower(strings.SplitN(h, ".", 2)[0])
	var b strings.Builder
	for _, r := range h {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			b.WriteRune(r)
		}
	}
	n := strings.Trim(b.String(), "-")
	if len(n) > 32 {
		n = n[:32]
	}
	if n == "" {
		n = "machine"
	}
	return n
}

// File is where the machines are kept.
func File() string { return filepath.Join(paths.Home(), "machines.json") }

// Load reads the machines, by name; none when there is no file.
func Load() ([]Machine, error) {
	b, err := os.ReadFile(File())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ms []Machine
	if err := json.Unmarshal(b, &ms); err != nil {
		return nil, fmt.Errorf("%s: %w", File(), err)
	}
	// What the file says is checked as typed input is.
	out := ms[:0]
	for _, m := range ms {
		if CheckName(m.Name) == nil && CheckTarget(m.Target) == nil {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func save(ms []Machine) error {
	if err := paths.EnsureDir(paths.Home()); err != nil {
		return err
	}
	b, err := json.MarshalIndent(ms, "", "  ")
	if err != nil {
		return err
	}
	return paths.WriteFile(File(), append(b, '\n'))
}

// Add saves a new machine.
func Add(m Machine) error {
	if err := CheckName(m.Name); err != nil {
		return err
	}
	if err := CheckTarget(m.Target); err != nil {
		return err
	}
	ms, err := Load()
	if err != nil {
		return err
	}
	for _, x := range ms {
		if x.Name == m.Name {
			return ErrExists
		}
	}
	if m.Added.IsZero() {
		m.Added = time.Now().UTC().Truncate(time.Second)
	}
	return save(append(ms, m))
}

// Remove forgets a machine and what was last heard from it.
func Remove(name string) error {
	ms, err := Load()
	if err != nil {
		return err
	}
	out := ms[:0]
	found := false
	for _, m := range ms {
		if m.Name == name {
			found = true
			continue
		}
		out = append(out, m)
	}
	if !found {
		return ErrNone
	}
	_ = os.Remove(CacheFile(name))
	return save(out)
}

// Get finds a machine by name.
func Get(name string) (Machine, error) {
	ms, err := Load()
	if err != nil {
		return Machine{}, err
	}
	for _, m := range ms {
		if m.Name == name {
			return m, nil
		}
	}
	return Machine{}, ErrNone
}
