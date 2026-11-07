// Package config loads ~/.imux/config.toml: built-in defaults, merged user
// values, and validation (regexes, enums, templates).
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// AgentIDs are the agents imux knows, in default order.
var AgentIDs = []string{"claude", "antigravity", "copilot", "codex"}

// RemovedAgents were supported once. A config that still names one loads
// with a warning instead of an error.
var RemovedAgents = map[string]string{
	"gemini": "Gemini CLI support was removed (Google stopped serving personal accounts in it on 2026-06-18); use \"antigravity\"",
}

// Duration is a time.Duration written as a Go duration string ("2.5s").
type Duration struct{ time.Duration }

// UnmarshalText implements encoding.TextUnmarshaler.
func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

// MarshalText implements encoding.TextMarshaler.
func (d Duration) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// D is a helper for building defaults.
func D(v time.Duration) Duration { return Duration{v} }

// Pattern is a user-supplied limit pattern.
type Pattern struct {
	Re       string `toml:"re"`
	Severity string `toml:"severity"` // limit | warn | ignore
}

// Agent is the per-agent configuration ([agents.<id>]).
type Agent struct {
	Bin                    string    `toml:"bin"`
	Args                   []string  `toml:"args"`
	NewArgs                []string  `toml:"new_args"`
	ResumeArgs             []string  `toml:"resume_args"`
	Home                   string    `toml:"home"`
	Resume                 bool      `toml:"resume"`
	Patterns               []Pattern `toml:"patterns"`
	ReplaceDefaultPatterns bool      `toml:"replace_default_patterns"`
}

// normalize turns empty lists into nil so that `args = []` equals the default.
func (a *Agent) normalize() {
	if len(a.Args) == 0 {
		a.Args = nil
	}
	if len(a.Patterns) == 0 {
		a.Patterns = nil
	}
}

// Handoff is the [handoff] section.
type Handoff struct {
	BudgetChars       int    `toml:"budget_chars"`
	MaxArgBytes       int    `toml:"max_arg_bytes"`
	Inject            string `toml:"inject"`     // argv | paste
	OnArrival         string `toml:"on_arrival"` // wait | continue
	RedactSecrets     bool   `toml:"redact_secrets"`
	IncludeGit        bool   `toml:"include_git"`
	IncludeSourcePath bool   `toml:"include_source_path"`
	KeepDays          int    `toml:"keep_days"`
	HeadBytes         int64  `toml:"head_bytes"`
	TailBytes         int64  `toml:"tail_bytes"`
}

// Limits is the [limits] section.
type Limits struct {
	PollInterval  Duration `toml:"poll_interval"`
	IdleInterval  Duration `toml:"idle_interval"`
	Grace         Duration `toml:"grace"`
	Cooldown      Duration `toml:"cooldown"`
	ScanLines     int      `toml:"scan_lines"`
	DefaultExpiry Duration `toml:"default_expiry"`
}

// Bus is the [bus] section.
type Bus struct {
	Enabled         bool     `toml:"enabled"`
	Confirm         bool     `toml:"confirm"`
	IdleAfter       Duration `toml:"idle_after"`
	MaxMessageBytes int      `toml:"max_message_bytes"`
	RatePerMinute   int      `toml:"rate_per_minute"`
}

// Tmux is the [tmux] section.
type Tmux struct {
	MenuKeyRoot     string `toml:"menu_key_root"`
	MenuKeyPrefix   string `toml:"menu_key_prefix"`
	PickerKeyPrefix string `toml:"picker_key_prefix"`
	AgentBar        bool   `toml:"agent_bar"`
	Sidebar         bool   `toml:"sidebar"`
	SidebarWidth    int    `toml:"sidebar_width"`
	SidebarKey      string `toml:"sidebar_key_prefix"`
	SplitRightKey   string `toml:"split_right_key_prefix"`
	SplitDownKey    string `toml:"split_down_key_prefix"`
	Mouse           bool   `toml:"mouse"`
	HistoryLimit    int    `toml:"history_limit"`
}

// Config is the effective configuration.
type Config struct {
	Order        []string          `toml:"order"`
	DefaultAgent string            `toml:"default_agent"`
	OnLimit      string            `toml:"on_limit"` // ask | auto | off
	OnExit       string            `toml:"on_exit"`  // shell | close
	Locale       string            `toml:"locale"`
	Handoff      Handoff           `toml:"handoff"`
	Limits       Limits            `toml:"limits"`
	Bus          Bus               `toml:"bus"`
	Tmux         Tmux              `toml:"tmux"`
	Agents       map[string]*Agent `toml:"agents"`
}

// Agent returns the configuration of agent id (never nil).
func (c *Config) Agent(id string) *Agent {
	if a, ok := c.Agents[id]; ok && a != nil {
		return a
	}
	if a, ok := defaultAgents()[id]; ok {
		return a
	}
	return &Agent{Resume: true}
}

func defaultAgents() map[string]*Agent {
	return map[string]*Agent{
		"claude": {
			NewArgs:    []string{"--session-id", "{uuid}", "{prompt}"},
			ResumeArgs: []string{"--resume", "{id}", "{prompt}"},
			Resume:     true,
		},
		// Antigravity CLI: -i runs an initial prompt and stays interactive,
		// --conversation resumes by id (documented separately; their
		// combination is unverified, and imux rarely knows the id).
		"antigravity": {
			NewArgs:    []string{"-i", "{prompt}"},
			ResumeArgs: []string{"--conversation", "{id}", "-i", "{prompt}"},
			Resume:     true,
		},
		"copilot": {
			NewArgs:    []string{"-i", "{prompt}"},
			ResumeArgs: []string{"--resume", "{id}"},
			Resume:     true,
		},
		"codex": {
			NewArgs:    []string{"{prompt}"},
			ResumeArgs: []string{"resume", "{id}", "{prompt}"},
			Resume:     true,
		},
	}
}

// Default returns the built-in defaults (§16 of ARCHITECTURE.md).
func Default() *Config {
	return &Config{
		Order:   slices.Clone(AgentIDs),
		OnLimit: "ask",
		OnExit:  "shell",
		Handoff: Handoff{
			BudgetChars:       30000,
			MaxArgBytes:       102400,
			Inject:            "argv",
			OnArrival:         "wait",
			RedactSecrets:     true,
			IncludeGit:        true,
			IncludeSourcePath: true,
			KeepDays:          30,
			HeadBytes:         1 << 20,
			TailBytes:         8 << 20,
		},
		Limits: Limits{
			PollInterval:  D(2500 * time.Millisecond),
			IdleInterval:  D(15 * time.Second),
			Grace:         D(20 * time.Second),
			Cooldown:      D(10 * time.Minute),
			ScanLines:     25,
			DefaultExpiry: D(5 * time.Hour),
		},
		Bus: Bus{
			Enabled:         true,
			IdleAfter:       D(5 * time.Second),
			MaxMessageBytes: 16384,
			RatePerMinute:   20,
		},
		Tmux: Tmux{
			MenuKeyRoot:     `C-\`,
			MenuKeyPrefix:   "a",
			PickerKeyPrefix: "S",
			AgentBar:        false,
			Sidebar:         true,
			SidebarWidth:    30,
			SidebarKey:      "b",
			SplitRightKey:   "v",
			SplitDownKey:    "-",
			Mouse:           true,
			HistoryLimit:    50000,
		},
		Agents: defaultAgents(),
	}
}

// Error is a validation error with an optional line reference.
type Error struct {
	Line int // 0 when unknown
	Msg  string
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("config line %d: %s", e.Line, e.Msg)
	}
	return "config: " + e.Msg
}

// Load reads path on top of the defaults. A missing file yields the defaults.
// Warnings (unknown keys, ...) are returned separately; err is set only for
// invalid configurations.
func Load(path string) (*Config, []string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return Parse(data)
}

// Parse decodes TOML data on top of the defaults and validates the result.
func Parse(data []byte) (*Config, []string, error) {
	cfg := Default()
	var raw struct {
		Config
		Agents map[string]toml.Primitive `toml:"agents"`
	}
	raw.Config = *cfg
	raw.Config.Agents = nil
	md, err := toml.Decode(string(data), &raw)
	if err != nil {
		var pe toml.ParseError
		if errors.As(err, &pe) {
			return nil, nil, &Error{Line: pe.Position.Line, Msg: pe.Message}
		}
		return nil, nil, &Error{Msg: err.Error()}
	}
	*cfg = raw.Config
	cfg.Agents = defaultAgents()
	var warnings []string
	agentKeys := make([]string, 0, len(raw.Agents))
	for k := range raw.Agents {
		agentKeys = append(agentKeys, k)
	}
	sort.Strings(agentKeys)
	for _, id := range agentKeys {
		a, ok := cfg.Agents[id]
		if !ok {
			if why, removed := RemovedAgents[id]; removed {
				warnings = append(warnings, fmt.Sprintf("[agents.%s] ignored: %s", id, why))
			} else {
				warnings = append(warnings, fmt.Sprintf("unknown agent [agents.%s] ignored", id))
			}
			continue
		}
		if err := md.PrimitiveDecode(raw.Agents[id], a); err != nil {
			return nil, nil, &Error{Msg: fmt.Sprintf("[agents.%s]: %v", id, err)}
		}
	}
	for _, k := range md.Undecoded() {
		if len(k) >= 2 && k[0] == "agents" && !slices.Contains(AgentIDs, k[1]) {
			continue // already reported
		}
		warnings = append(warnings, fmt.Sprintf("unknown key %q ignored", k.String()))
	}
	for _, a := range cfg.Agents {
		a.normalize()
	}
	// Drop removed agents from order and default_agent, with a warning.
	order := cfg.Order[:0:0]
	for _, id := range cfg.Order {
		if why, removed := RemovedAgents[id]; removed {
			warnings = append(warnings, fmt.Sprintf("order: %q ignored: %s", id, why))
			continue
		}
		order = append(order, id)
	}
	cfg.Order = order
	if why, removed := RemovedAgents[cfg.DefaultAgent]; removed {
		warnings = append(warnings, fmt.Sprintf("default_agent: %q ignored: %s", cfg.DefaultAgent, why))
		cfg.DefaultAgent = ""
	}
	if err := cfg.validate(data); err != nil {
		return nil, warnings, err
	}
	return cfg, warnings, nil
}

var severities = []string{"limit", "warn", "ignore"}

// CompilePattern compiles a limit pattern case-insensitively (RE2). A
// pattern may opt out with a leading (?-i).
func CompilePattern(re string) (*regexp.Regexp, error) {
	return regexp.Compile("(?i)" + re)
}

func (c *Config) validate(src []byte) error {
	for _, id := range c.Order {
		if !slices.Contains(AgentIDs, id) {
			return &Error{Line: lineOf(src, "order"), Msg: fmt.Sprintf("order: unknown agent %q (known: %s)", id, strings.Join(AgentIDs, ", "))}
		}
	}
	if c.DefaultAgent != "" && !slices.Contains(AgentIDs, c.DefaultAgent) {
		return &Error{Line: lineOf(src, "default_agent"), Msg: fmt.Sprintf("default_agent: unknown agent %q", c.DefaultAgent)}
	}
	enums := []struct {
		key, val string
		allowed  []string
	}{
		{"on_limit", c.OnLimit, []string{"ask", "auto", "off"}},
		{"on_arrival", c.Handoff.OnArrival, []string{"wait", "continue"}},
		{"on_exit", c.OnExit, []string{"shell", "close"}},
		{"inject", c.Handoff.Inject, []string{"argv", "paste"}},
		{"locale", c.Locale, []string{"", "en", "tr"}},
	}
	for _, e := range enums {
		if !slices.Contains(e.allowed, e.val) {
			return &Error{Line: lineOf(src, e.key), Msg: fmt.Sprintf("%s: invalid value %q (allowed: %s)", e.key, e.val, strings.Join(e.allowed, ", "))}
		}
	}
	positive := []struct {
		key string
		v   int64
	}{
		{"budget_chars", int64(c.Handoff.BudgetChars)},
		{"max_arg_bytes", int64(c.Handoff.MaxArgBytes)},
		{"head_bytes", c.Handoff.HeadBytes},
		{"tail_bytes", c.Handoff.TailBytes},
		{"poll_interval", int64(c.Limits.PollInterval.Duration)},
		{"idle_interval", int64(c.Limits.IdleInterval.Duration)},
		{"scan_lines", int64(c.Limits.ScanLines)},
		{"default_expiry", int64(c.Limits.DefaultExpiry.Duration)},
		{"max_message_bytes", int64(c.Bus.MaxMessageBytes)},
		{"rate_per_minute", int64(c.Bus.RatePerMinute)},
	}
	for _, p := range positive {
		if p.v <= 0 {
			return &Error{Line: lineOf(src, p.key), Msg: fmt.Sprintf("%s must be positive", p.key)}
		}
	}
	if c.Tmux.Sidebar && (c.Tmux.SidebarWidth < 20 || c.Tmux.SidebarWidth > 80) {
		return &Error{Line: lineOf(src, "sidebar_width"), Msg: "sidebar_width must be between 20 and 80"}
	}
	if c.Handoff.BudgetChars < 2000 {
		return &Error{Line: lineOf(src, "budget_chars"), Msg: "budget_chars must be at least 2000"}
	}
	for _, id := range AgentIDs {
		a := c.Agent(id)
		for _, tpl := range []struct {
			key  string
			args []string
		}{{"new_args", a.NewArgs}, {"resume_args", a.ResumeArgs}} {
			n := 0
			for _, s := range tpl.args {
				n += strings.Count(s, "{prompt}")
			}
			if n > 1 {
				return &Error{Line: lineOf(src, tpl.key), Msg: fmt.Sprintf("agents.%s.%s: {prompt} may appear at most once", id, tpl.key)}
			}
		}
		for _, p := range a.Patterns {
			if _, err := CompilePattern(p.Re); err != nil {
				return &Error{Line: lineOf(src, p.Re), Msg: fmt.Sprintf("agents.%s.patterns: invalid regex %q: %v", id, p.Re, err)}
			}
			if !slices.Contains(severities, p.Severity) {
				return &Error{Line: lineOf(src, p.Re), Msg: fmt.Sprintf("agents.%s.patterns: severity %q must be one of %s", id, p.Severity, strings.Join(severities, ", "))}
			}
		}
	}
	return nil
}

// lineOf returns the 1-based line of the first occurrence of needle in src
// (0 if absent). Used to point validation errors at the user's file.
func lineOf(src []byte, needle string) int {
	if needle == "" {
		return 0
	}
	i := bytes.Index(src, []byte(needle))
	if i < 0 {
		return 0
	}
	return bytes.Count(src[:i], []byte("\n")) + 1
}

// Encode writes the effective configuration as TOML.
func (c *Config) Encode() ([]byte, error) {
	var b bytes.Buffer
	enc := toml.NewEncoder(&b)
	enc.Indent = ""
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
