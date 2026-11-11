//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/config"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestSettings (T29.4, T57.1): 🔧 on the rail opens Settings; a row opens
// the menu of its values (D50), a choice changes the config file and
// Settings opens again with the new value; the sidebar redraws with the new
// icons; the commit-agent menu sets the agent.
func TestSettings(t *testing.T) {
	h := newH(t, sidebarConfig)
	agent, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	load := func() *config.Config {
		cfg, _, err := config.Load(filepath.Join(h.Homes.Imux, "config.toml"))
		if err != nil {
			t.Fatal(err)
		}
		return cfg
	}

	x, y := h.rowOf(bar, "🔧")
	mark := c.Mark()
	c.Click(0, x, y)
	c.waitScreenAfter(h, mark, "i-mux · Settings", 5*time.Second)
	c.waitScreenAfter(h, mark, "Icons: Auto", 5*time.Second)
	time.Sleep(300 * time.Millisecond)

	mark = c.Mark()
	c.Write("w")
	c.waitScreenAfter(h, mark, "continues at once", 5*time.Second)
	if load().Handoff.OnArrival == "continue" {
		t.Fatal("the row changed the value without asking")
	}
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("2")
	c.waitScreenAfter(h, mark, "After a handoff the new agent: continues at once", 5*time.Second)
	if load().Handoff.OnArrival != "continue" {
		t.Fatal("on_arrival not written")
	}
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("m")
	c.waitScreenAfter(h, mark, "Commit messages by", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("x") // Codex
	c.waitScreenAfter(h, mark, "Commit messages by: Codex CLI", 5*time.Second)
	if load().Git.CommitAgent != "codex" {
		t.Fatal("commit_agent not written")
	}
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("i")
	c.waitScreenAfter(h, mark, "● Auto", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("2")
	c.waitScreenAfter(h, mark, "Icons: Nerd Font", 5*time.Second)
	if load().Tmux.Icons != "nerd" {
		t.Fatal("icons not written")
	}
	// The sidebar reads the changed config: the editor's icons on the rail.
	h.waitFor("nerd icons on the rail", 10*time.Second, func() bool {
		return strings.Contains(h.screen(bar), "")
	})
	c.Write("\x1b")
	// Only those lines changed: the rest of the file stays.
	b, _ := os.ReadFile(filepath.Join(h.Homes.Imux, "config.toml"))
	if !strings.Contains(string(b), "sidebar = true") || !strings.Contains(string(b), "agent_bar = false") {
		t.Fatalf("other settings lost:\n%s", b)
	}

	// Install the Nerd Font for the icons… (M43) runs `imux font install` in
	// a popup: it shows what is there and asks first (no is the answer
	// here), or says all is set.
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	h.mustImux("settings", "--pane", agent)
	c.waitScreenAfter(h, mark, "Install the Nerd Font for the icons", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	mark = c.Mark()
	c.Write("f")
	var out string
	h.waitFor("the font popup", 20*time.Second, func() bool {
		out = stripANSI(c.Output()[mark:])
		return strings.Contains(out, "Go ahead? [Y/n]") || strings.Contains(out, "Press Enter to close.")
	})
	if !strings.Contains(out, "Sidebar icons: nerd") || !strings.Contains(out, "JetBrainsMono Nerd Font") {
		t.Fatalf("font popup:\n%s", out)
	}
	if strings.Contains(out, "Go ahead? [Y/n]") {
		mark = c.Mark()
		c.Write("n\r")
		c.waitScreenAfter(h, mark, "Nothing changed.", 5*time.Second)
	}
	c.Write("\r")
}

// TestCompletionEdit (T29.4): with AI completion on and set up, a file
// opened from the explorer opens in Neovim with the imux-nvim profile.
func TestCompletionEdit(t *testing.T) {
	h := newH(t, "[completion]\nenabled = true\n"+sidebarConfig)
	// The profile and copilot.vim as setup leaves them.
	data := filepath.Join(h.Homes.Home, ".local", "share", "imux-nvim", "site", "pack", "imux", "start", "copilot.vim", "plugin", "copilot.vim")
	testutil.WriteFile(t, data, []byte("\" fake\n"))
	testutil.WriteFile(t, filepath.Join(h.Homes.Home, ".config", "imux-nvim", "init.lua"), []byte("-- fake\n"))
	testutil.WriteFile(t, filepath.Join(h.Homes.Project, "main.go"), []byte("package main\n"))
	t.Setenv("IMUX_NVIM", filepath.Join(h.Fakes, "nvim")) // not a Neovim on the PATH
	_, bar := h.startWithSidebar()
	h.run("claude", 1)
	c, _ := h.attach(140, 40)
	c.waitScreen(h, "AGENTS", 10*time.Second)
	x, y := h.rowOf(bar, "📁")
	c.Click(0, x, y)
	time.Sleep(500 * time.Millisecond)
	x, y = h.rowOf(bar, "main.go")
	c.Click(0, x, y)
	var argv []string
	h.waitFor("nvim started", 5*time.Second, func() bool {
		b, err := os.ReadFile(filepath.Join(h.Log, "nvim.argv"))
		argv = splitNUL(b)
		return err == nil
	})
	env, _ := os.ReadFile(filepath.Join(h.Log, "nvim.env"))
	if len(argv) != 1 || !strings.HasSuffix(argv[0], "main.go") || strings.TrimSpace(string(env)) != "NVIM_APPNAME=imux-nvim" {
		t.Fatalf("nvim argv %q env %q", argv, env)
	}
}
