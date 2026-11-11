// Package completion keeps the editor profile for AI completion (ARCHITECTURE
// D30): Neovim with GitHub's copilot.vim, in a profile of its own
// (NVIM_APPNAME=imux-nvim) so the user's Neovim setup stays untouched.
package completion

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
)

// AppName is the Neovim profile (NVIM_APPNAME).
const AppName = "imux-nvim"

// PluginURL is GitHub's Copilot plugin for Vim and Neovim.
const PluginURL = "https://github.com/github/copilot.vim"

// MinNode is the oldest Node.js major version copilot.vim runs on.
const MinNode = 20

// LookPath is exec.LookPath; tests replace it and the folders below.
var LookPath = exec.LookPath

// Where Neovim and Node.js are looked for when they are not on the PATH.
var (
	NvimPaths = []string{"/opt/homebrew/bin/nvim", "/usr/local/bin/nvim"}
	NodePaths = []string{"/opt/homebrew/bin/node", "/usr/local/bin/node"}
)

func home() string { h, _ := os.UserHomeDir(); return h }

func xdg(env, def string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return filepath.Join(home(), def)
}

// ProfileDir is the profile's config folder (~/.config/imux-nvim).
func ProfileDir() string { return filepath.Join(xdg("XDG_CONFIG_HOME", ".config"), AppName) }

// InitFile is the profile's init.lua, which i-mux writes.
func InitFile() string { return filepath.Join(ProfileDir(), "init.lua") }

// PluginDir is where copilot.vim is kept: a start package of the profile.
func PluginDir() string {
	return filepath.Join(xdg("XDG_DATA_HOME", ".local/share"), AppName, "site", "pack", "imux", "start", "copilot.vim")
}

// Nvim finds Neovim: $IMUX_NVIM (one outside the PATH), the PATH, then
// Homebrew's folders.
func Nvim() string {
	if p := os.Getenv("IMUX_NVIM"); p != "" && executable(p) {
		return p
	}
	if p, err := LookPath("nvim"); err == nil {
		return p
	}
	for _, p := range NvimPaths {
		if executable(p) {
			return p
		}
	}
	return ""
}

// Node finds Node.js: PATH, then the newest one nvm installed, then
// Homebrew's.
func Node() string {
	if p, err := LookPath("node"); err == nil {
		return p
	}
	nvm, _ := filepath.Glob(filepath.Join(home(), ".nvm", "versions", "node", "v*", "bin", "node"))
	sort.Slice(nvm, func(i, j int) bool { return nodeMajor(nvm[i]) > nodeMajor(nvm[j]) })
	for _, p := range append(nvm, NodePaths...) {
		if executable(p) {
			return p
		}
	}
	return ""
}

// nodeMajor reads the major version from an nvm path (…/node/v22.15.0/bin/node).
func nodeMajor(p string) int {
	v := filepath.Base(filepath.Dir(filepath.Dir(p)))
	n, _ := strconv.Atoi(strings.SplitN(strings.TrimPrefix(v, "v"), ".", 2)[0])
	return n
}

// NodeVersion runs node --version and returns its major version (0 when
// unknown).
func NodeVersion(node string) int {
	out, err := exec.Command(node, "--version").Output()
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.SplitN(strings.TrimPrefix(strings.TrimSpace(string(out)), "v"), ".", 2)[0])
	return n
}

func executable(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular() && sysx.Executable(fi)
}

// Ready reports whether edits can open in the profile: Neovim is there,
// copilot.vim is in the profile and init.lua is written.
func Ready() bool {
	if Nvim() == "" {
		return false
	}
	if _, err := os.Stat(filepath.Join(PluginDir(), "plugin", "copilot.vim")); err != nil {
		return false
	}
	_, err := os.Stat(InitFile())
	return err == nil
}

// Editor is the argv and the extra environment that edit path at line in
// the profile; nil when it is not Ready.
func Editor(path string, line int) ([]string, []string) {
	if !Ready() {
		return nil, nil
	}
	argv := []string{Nvim()}
	if line > 0 {
		argv = append(argv, "+"+strconv.Itoa(line))
	}
	return append(argv, path), []string{"NVIM_APPNAME=" + AppName}
}

// Visual is the editor as a shell command for $VISUAL (less's v), "" when
// it is not Ready.
func Visual() string {
	if !Ready() {
		return ""
	}
	return "env NVIM_APPNAME=" + AppName + " " + Nvim()
}

// InitLua is the profile's init.lua: Copilot's Node.js, the mouse, Ctrl+S
// saves, Ctrl+Q quits, typing starts at once, and the status line names the
// keys. Tab accepts a suggestion (copilot.vim's own mapping).
func InitLua(node string) string {
	return `-- Written by i-mux for AI completion in the editor (Settings, ⚙ on the
-- rail). i-mux writes this file again; keep your own Neovim settings in
-- ~/.config/nvim, which this profile does not read.
vim.g.copilot_node_command = ` + strconv.Quote(node) + `
vim.opt.mouse = "a"
vim.opt.number = true
vim.opt.clipboard = "unnamedplus"
vim.opt.termguicolors = true
vim.opt.laststatus = 2
vim.opt.statusline = " %f %m%=Tab: accept the suggestion · Ctrl+S: save · Ctrl+Q: quit "
vim.keymap.set({ "n", "i", "v" }, "<C-s>", "<Cmd>write<CR>", { silent = true })
vim.keymap.set({ "n", "i", "v" }, "<C-q>", "<Cmd>confirm quit<CR>", { silent = true })
-- Ready to type at once, like other editors.
vim.api.nvim_create_autocmd("VimEnter", {
  callback = function()
    if vim.fn.argc() > 0 and vim.bo.modifiable then
      vim.cmd.startinsert()
    end
  end,
})
`
}

// ErrNotReady ends a setup that could not finish.
var ErrNotReady = errors.New("AI completion is not set up")

// Setup installs and writes what the profile needs, talking to the user on
// Out and In (a popup's terminal).
type Setup struct {
	Out  io.Writer
	In   *bufio.Reader
	Yes  bool                                    // answer yes to every question
	Run  func(name string, args ...string) error // runs a command on the terminal
	Brew string                                  // Homebrew's brew ("" when there is none)
}

func (s *Setup) say(format string, a ...any) { fmt.Fprintf(s.Out, format+"\n", a...) }

// Ask asks a yes / no question (yes by default).
func (s *Setup) Ask(q string) bool {
	if s.Yes {
		s.say("%s [Y/n] y", q)
		return true
	}
	fmt.Fprintf(s.Out, "%s [Y/n] ", q)
	line, _ := s.In.ReadString('\n')
	a := strings.ToLower(strings.TrimSpace(line))
	return a == "" || a == "y" || a == "yes" || a == "e" || a == "evet"
}

// Do runs the steps: Neovim, Node.js, copilot.vim, init.lua.
func (s *Setup) Do() error {
	nvim := Nvim()
	if nvim == "" {
		if s.Brew == "" {
			s.say("Neovim is needed: install it (macOS: brew install neovim; Debian/Ubuntu: sudo apt install neovim), then turn AI completion on again.")
			return ErrNotReady
		}
		if !s.Ask("Neovim is needed. Install it with Homebrew (brew install neovim)?") {
			return ErrNotReady
		}
		if err := s.Run(s.Brew, "install", "neovim"); err != nil {
			return fmt.Errorf("brew install neovim: %w", err)
		}
		if nvim = Nvim(); nvim == "" {
			return ErrNotReady
		}
	}
	s.say("✓ Neovim: %s", nvim)
	node := Node()
	if node == "" || NodeVersion(node) < MinNode {
		s.say("GitHub Copilot needs Node.js %d or newer (found: %s). Install it (brew install node), then turn AI completion on again.", MinNode, orNone(node))
		return ErrNotReady
	}
	s.say("✓ Node.js: %s", node)
	dir := PluginDir()
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		s.say("Updating copilot.vim…")
		if err := s.Run("git", "-C", dir, "pull", "--ff-only", "-q"); err != nil {
			s.say("(could not update it; the copy there is used)")
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
			return err
		}
		s.say("Getting copilot.vim from %s…", PluginURL)
		if err := s.Run("git", "clone", "--depth", "1", "-q", PluginURL, dir); err != nil {
			return fmt.Errorf("git clone: %w", err)
		}
	}
	s.say("✓ copilot.vim: %s", dir)
	if err := os.MkdirAll(ProfileDir(), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(InitFile(), []byte(InitLua(node)), 0o644); err != nil {
		return err
	}
	s.say("✓ Editor profile: %s", InitFile())
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
