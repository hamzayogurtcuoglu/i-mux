package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetLine(t *testing.T) {
	src := "# top comment\non_limit = \"ask\"\n\n[handoff]\n# How it arrives.\non_arrival          = \"wait\"\n\n[tmux]\nicons = \"auto\" # theme\n"
	for _, c := range []struct {
		section, key, value, want string
	}{
		{"handoff", "on_arrival", `"continue"`, "on_arrival          = \"continue\"\n"},
		{"tmux", "icons", `"nerd"`, "[tmux]\nicons = \"nerd\"\n"},
		{"", "on_limit", `"off"`, "# top comment\non_limit = \"off\"\n"},
		{"", "locale", `"tr"`, "on_limit = \"ask\"\nlocale = \"tr\"\n\n[handoff]"},
		{"handoff", "deliver", `"inline"`, "on_arrival          = \"wait\"\ndeliver = \"inline\"\n"},
		{"completion", "enabled", "true", "icons = \"auto\" # theme\n\n[completion]\nenabled = true\n"},
	} {
		got := setLine(src, c.section, c.key, c.value)
		if !strings.Contains(got, c.want) {
			t.Errorf("%s.%s:\n%s", c.section, c.key, got)
		}
		if !strings.Contains(got, "# top comment") || !strings.Contains(got, "# How it arrives.") {
			t.Errorf("comments lost:\n%s", got)
		}
	}
	// The same key in another section is not touched.
	two := "[a]\nx = 1\n[b]\nx = 2\n"
	if got := setLine(two, "b", "x", "3"); got != "[a]\nx = 1\n[b]\nx = 3\n" {
		t.Errorf("section-scoped: %q", got)
	}
	// A commented-out key is not the key.
	if got := setLine("[a]\n# x = 1\n", "a", "x", "2"); got != "[a]\n# x = 1\nx = 2\n" {
		t.Errorf("commented key: %q", got)
	}
}

func TestSet(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := Set(p, "handoff", "on_arrival", Str("continue")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(b), Template[:40]) || !strings.Contains(string(b), `on_arrival          = "continue"`) {
		t.Fatalf("written from the template with the key set:\n%.400s", b)
	}
	cfg, _, err := Load(p)
	if err != nil || cfg.Handoff.OnArrival != "continue" {
		t.Fatalf("load: %v %+v", err, cfg)
	}
	if err := Set(p, "git", "commit_agent", Str("nobody")); err == nil {
		t.Fatal("an invalid value was written")
	}
	if b2, _ := os.ReadFile(p); string(b2) != string(b) {
		t.Fatal("a failed Set changed the file")
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	if ModTime(p).IsZero() || !ModTime(filepath.Join(t.TempDir(), "none")).IsZero() {
		t.Fatal("ModTime")
	}
}
