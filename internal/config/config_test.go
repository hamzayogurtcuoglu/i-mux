package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDefaultsOnly(t *testing.T) {
	cfg, warns, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Fatal("missing file should yield defaults")
	}
	if cfg.Limits.PollInterval.Duration != 2500*time.Millisecond {
		t.Fatalf("poll interval %v", cfg.Limits.PollInterval)
	}
}

func TestTemplateMatchesDefaults(t *testing.T) {
	cfg, warns, err := Parse([]byte(Template))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Fatalf("template has unknown keys: %v", warns)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		got, _ := cfg.Encode()
		want, _ := Default().Encode()
		t.Fatalf("template differs from Default()\n--- template\n%s\n--- default\n%s", got, want)
	}
}

func TestPartialOverride(t *testing.T) {
	cfg, warns, err := Parse([]byte(`
on_limit = "auto"
[handoff]
budget_chars = 12000
[limits]
poll_interval = "1s"
`))
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if cfg.OnLimit != "auto" || cfg.Handoff.BudgetChars != 12000 {
		t.Fatalf("override not applied: %+v", cfg)
	}
	if cfg.Handoff.MaxArgBytes != 102400 || !cfg.Handoff.RedactSecrets {
		t.Fatal("untouched keys in an overridden section must keep their defaults")
	}
	if cfg.Limits.PollInterval.Duration != time.Second || cfg.Limits.Grace.Duration != 20*time.Second {
		t.Fatalf("limits %+v", cfg.Limits)
	}
	if cfg.OnExit != "shell" {
		t.Fatal("on_exit default lost")
	}
}

func TestPerAgentOverride(t *testing.T) {
	cfg, _, err := Parse([]byte(`
[agents.claude]
args = ["--model", "opus"]
patterns = [{ re = 'my limit', severity = "limit" }]
[agents.codex]
bin = "/opt/codex"
`))
	if err != nil {
		t.Fatal(err)
	}
	c := cfg.Agent("claude")
	if !reflect.DeepEqual(c.Args, []string{"--model", "opus"}) {
		t.Fatalf("args %v", c.Args)
	}
	if !reflect.DeepEqual(c.NewArgs, []string{"--session-id", "{uuid}", "{prompt}"}) {
		t.Fatalf("new_args default lost: %v", c.NewArgs)
	}
	if len(c.Patterns) != 1 || c.Patterns[0].Severity != "limit" {
		t.Fatalf("patterns %v", c.Patterns)
	}
	if !c.Resume {
		t.Fatal("resume default lost")
	}
	x := cfg.Agent("codex")
	if x.Bin != "/opt/codex" || !reflect.DeepEqual(x.ResumeArgs, []string{"resume", "{id}", "{prompt}"}) {
		t.Fatalf("codex %+v", x)
	}
	if cfg.Agent("antigravity").NewArgs[0] != "-i" {
		t.Fatal("antigravity defaults lost")
	}
}

func TestUnknownKeyWarns(t *testing.T) {
	_, warns, err := Parse([]byte("bogus = 1\n[handoff]\nnope = true\n[agents.vim]\nbin = \"x\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(warns, "\n")
	for _, w := range []string{`"bogus"`, `"handoff.nope"`, "agents.vim"} {
		if !strings.Contains(joined, w) {
			t.Errorf("missing warning for %s in %q", w, joined)
		}
	}
}

func TestInvalidRegexHasLine(t *testing.T) {
	_, _, err := Parse([]byte("on_limit = \"ask\"\n\n[agents.antigravity]\npatterns = [{ re = 'a(b', severity = \"limit\" }]\n"))
	var ce *Error
	if !errors.As(err, &ce) {
		t.Fatalf("want *Error, got %v", err)
	}
	if ce.Line != 4 || !strings.Contains(ce.Msg, "invalid regex") {
		t.Fatalf("got line %d msg %q", ce.Line, ce.Msg)
	}
}

func TestPromptAtMostOnce(t *testing.T) {
	_, _, err := Parse([]byte(`[agents.codex]
new_args = ["{prompt}", "--x", "{prompt}"]
`))
	if err == nil || !strings.Contains(err.Error(), "{prompt} may appear at most once") {
		t.Fatalf("got %v", err)
	}
}

func TestInvalidEnumsAndOrder(t *testing.T) {
	for _, src := range []string{
		`on_limit = "sometimes"`,
		`order = ["claude", "vim"]`,
		"[handoff]\ninject = \"telepathy\"",
		`[agents.claude]
patterns = [{ re = 'x', severity = "panic" }]`,
		"[handoff]\nbudget_chars = 0",
	} {
		if _, _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%q should be rejected", src)
		}
	}
}

func TestSyntaxErrorHasLine(t *testing.T) {
	_, _, err := Parse([]byte("a = 1\nb = = 2\n"))
	var ce *Error
	if !errors.As(err, &ce) || ce.Line != 2 {
		t.Fatalf("got %v", err)
	}
}

func TestWriteTemplateRefusesOverwrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := WriteTemplate(p, false); err != nil {
		t.Fatal(err)
	}
	if err := WriteTemplate(p, false); !errors.Is(err, ErrExists) {
		t.Fatalf("want ErrExists, got %v", err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteTemplate(p, true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != Template {
		t.Fatal("force did not overwrite")
	}
	fi, _ := os.Stat(p)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	b, err := Default().Encode()
	if err != nil {
		t.Fatal(err)
	}
	cfg, warns, err := Parse(b)
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v\n%s", err, warns, b)
	}
	if !reflect.DeepEqual(cfg, Default()) {
		t.Fatal("encode/parse round trip changed the config")
	}
}

// A config written for a version that still had Gemini CLI loads: the removed
// agent is dropped from order and default_agent, and its table is ignored,
// each with a warning (never an error that would stop imux from starting).
func TestRemovedAgentTolerated(t *testing.T) {
	cfg, warns, err := Parse([]byte(`order = ["claude", "gemini", "codex"]
default_agent = "gemini"

[agents.gemini]
bin = "/usr/local/bin/gemini"
`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(cfg.Order, ",") != "claude,codex" || cfg.DefaultAgent != "" {
		t.Fatalf("order %v default %q", cfg.Order, cfg.DefaultAgent)
	}
	if len(warns) != 3 {
		t.Fatalf("want 3 warnings, got %q", warns)
	}
	for _, w := range warns {
		if !strings.Contains(w, "gemini") || !strings.Contains(w, "antigravity") {
			t.Errorf("warning %q should name gemini and point to antigravity", w)
		}
	}
}
