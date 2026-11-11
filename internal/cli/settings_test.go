package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestSetEditor (D50): the Editor row saves `editor` and turns AI
// completion off; Neovim + Copilot turns it on; Auto clears the editor.
func TestSetEditor(t *testing.T) {
	testutil.Isolate(t)
	load := func() *config.Config {
		t.Helper()
		cfg, _, err := config.Load(paths.Config())
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	if err := setSetting("editor", "copilot"); err != nil || !load().Completion.Enabled {
		t.Fatalf("copilot: %v", err)
	}
	if err := setSetting("editor", "nano"); err != nil {
		t.Fatal(err)
	}
	if cfg := load(); cfg.Editor != "nano" || cfg.Completion.Enabled {
		t.Errorf("nano: %q %v", cfg.Editor, cfg.Completion.Enabled)
	}
	if err := setSetting("editor", "auto"); err != nil || load().Editor != "" {
		t.Errorf("auto: %v %q", err, load().Editor)
	}
	if err := setSetting("editor", "emacs"); err == nil {
		t.Error("an unknown editor saved")
	}
	b, _ := os.ReadFile(paths.Config())
	if !strings.Contains(string(b), "# ") {
		t.Error("the config lost its comments")
	}
}
