package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigDocUpToDate fails when docs/CONFIG.md no longer matches the
// built-in defaults (T8.2). Fix: go generate ./internal/config
func TestConfigDocUpToDate(t *testing.T) {
	want := Doc()
	got, err := os.ReadFile(filepath.Join("..", "..", "docs", "CONFIG.md"))
	if err != nil {
		t.Fatalf("%v (run go generate ./internal/config)", err)
	}
	if string(got) != want {
		t.Fatal("docs/CONFIG.md is stale; run: go generate ./internal/config")
	}
	for _, key := range []string{"`on_limit`", "`handoff.budget_chars`", "`limits.poll_interval`", "`bus.rate_per_minute`", "`tmux.menu_key_root`", "`agents.claude.new_args`", "`agents.codex.resume_args`"} {
		if !strings.Contains(want, key) {
			t.Errorf("CONFIG.md lacks %s", key)
		}
	}
}
