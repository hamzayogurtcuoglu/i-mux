package completion

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestMain runs the fake tools on Windows, where they are this test binary
// (testutil.WriteFake) instead of shell scripts.
func TestMain(m *testing.M) {
	testutil.FakeMain(nil)
	os.Exit(m.Run())
}

// fakeHome points HOME and the XDG folders at a temp dir and LookPath at
// fake tools in bin.
func fakeHome(t *testing.T, tools ...string) (home, bin string) {
	t.Helper()
	home = t.TempDir()
	bin = filepath.Join(home, "bin")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	for _, tool := range tools {
		fakeTool(t, filepath.Join(bin, tool), "v22.15.0")
	}
	old, oldNvim, oldNode := LookPath, NvimPaths, NodePaths
	NvimPaths, NodePaths = nil, nil
	LookPath = func(name string) (string, error) {
		p := testutil.FakeName(filepath.Join(bin, name))
		if executable(p) {
			return p, nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { LookPath, NvimPaths, NodePaths = old, oldNvim, oldNode })
	return home, bin
}

func script(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// fakeTool writes a program at p that prints version (if any), and returns its path: a
// shell script on Unix, a fake program (p.cmd) on Windows.
func fakeTool(t *testing.T, p, version string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return testutil.WriteFake(t, p, "print", version+"\n")
	}
	body := "#!/bin/sh\n"
	if version != "" {
		body += "echo " + version + "\n"
	}
	script(t, p, body)
	return p
}

func TestInitLua(t *testing.T) {
	s := InitLua("/opt/node/bin/node")
	for _, want := range []string{
		`vim.g.copilot_node_command = "/opt/node/bin/node"`, `vim.opt.mouse = "a"`, `"<C-s>", "<Cmd>write<CR>"`,
		`"<C-q>", "<Cmd>confirm quit<CR>"`, "startinsert", "Tab: accept the suggestion",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("init.lua lacks %q", want)
		}
	}
}

func TestPathsAndNode(t *testing.T) {
	home, _ := fakeHome(t)
	if ProfileDir() != filepath.Join(home, ".config", "imux-nvim") ||
		PluginDir() != filepath.Join(home, ".local", "share", "imux-nvim", "site", "pack", "imux", "start", "copilot.vim") {
		t.Fatalf("dirs %s %s", ProfileDir(), PluginDir())
	}
	if runtime.GOOS == "windows" {
		return // nvm's layout (versions/node/v*/bin/node) is Unix's; Node does not look for nvm-windows
	}
	// No node on PATH: the newest one nvm installed.
	for _, v := range []string{"v18.20.0", "v22.15.0", "v20.11.1"} {
		script(t, filepath.Join(home, ".nvm", "versions", "node", v, "bin", "node"), "#!/bin/sh\necho "+v+"\n")
	}
	if n := Node(); !strings.Contains(n, "v22.15.0") {
		t.Fatalf("node %s", n)
	}
	if NodeVersion(Node()) != 22 {
		t.Fatalf("version %d", NodeVersion(Node()))
	}
}

func TestReadyAndEditor(t *testing.T) {
	_, _ = fakeHome(t, "nvim")
	if Ready() {
		t.Fatal("ready without the plugin")
	}
	if argv, _ := Editor("/p/a.go", 3); argv != nil {
		t.Fatal("an editor while not ready")
	}
	script(t, filepath.Join(PluginDir(), "plugin", "copilot.vim"), "")
	script(t, InitFile(), "")
	if !Ready() {
		t.Fatal("not ready")
	}
	argv, env := Editor("/p/a.go", 3)
	if len(argv) != 3 || filepath.Base(argv[0]) != testutil.FakeName("nvim") || argv[1] != "+3" || argv[2] != "/p/a.go" || len(env) != 1 || env[0] != "NVIM_APPNAME=imux-nvim" {
		t.Fatalf("editor %q %q", argv, env)
	}
	if v := Visual(); !strings.HasPrefix(v, "env NVIM_APPNAME=imux-nvim ") || !strings.HasSuffix(v, string(filepath.Separator)+testutil.FakeName("nvim")) {
		t.Fatalf("visual %q", v)
	}
}

func TestSetup(t *testing.T) {
	_, bin := fakeHome(t, "node")
	var ran []string
	var out bytes.Buffer
	s := &Setup{
		Out: &out, In: bufio.NewReader(strings.NewReader("\n")), Brew: "/opt/homebrew/bin/brew",
		Run: func(name string, args ...string) error {
			ran = append(ran, name+" "+strings.Join(args, " "))
			switch {
			case strings.HasSuffix(name, "brew"):
				fakeTool(t, filepath.Join(bin, "nvim"), "")
			case name == "git" && args[0] == "clone":
				script(t, filepath.Join(args[len(args)-1], "plugin", "copilot.vim"), "")
			}
			return nil
		},
	}
	if err := s.Do(); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	if len(ran) != 2 || ran[0] != "/opt/homebrew/bin/brew install neovim" || !strings.HasPrefix(ran[1], "git clone --depth 1 -q "+PluginURL) {
		t.Fatalf("commands %q", ran)
	}
	if b, _ := os.ReadFile(InitFile()); !strings.Contains(string(b), "vim.g.copilot_node_command = "+strconv.Quote(testutil.FakeName(filepath.Join(bin, "node")))) {
		t.Fatalf("init.lua %s", b)
	}
	if !Ready() || !strings.Contains(out.String(), "Install it with Homebrew") {
		t.Fatalf("not ready:\n%s", out.String())
	}

	// Without Homebrew and Neovim it says how to install and stops.
	_, _ = fakeHome(t, "node")
	out.Reset()
	s = &Setup{Out: &out, In: bufio.NewReader(strings.NewReader("")), Run: func(string, ...string) error { t.Fatal("ran a command"); return nil }}
	if err := s.Do(); !errors.Is(err, ErrNotReady) || !strings.Contains(out.String(), "brew install neovim") {
		t.Fatalf("no brew: %v\n%s", err, out.String())
	}

	// A "no" stops before installing.
	_, _ = fakeHome(t, "node")
	out.Reset()
	s = &Setup{Out: &out, In: bufio.NewReader(strings.NewReader("n\n")), Brew: "brew", Run: func(string, ...string) error { t.Fatal("ran a command"); return nil }}
	if err := s.Do(); !errors.Is(err, ErrNotReady) {
		t.Fatalf("declined: %v", err)
	}
}

func TestNvimOverride(t *testing.T) {
	home, _ := fakeHome(t)
	p := fakeTool(t, filepath.Join(home, "elsewhere", "nvim"), "")
	t.Setenv("IMUX_NVIM", p)
	if Nvim() != p {
		t.Fatalf("nvim %s", Nvim())
	}
}
