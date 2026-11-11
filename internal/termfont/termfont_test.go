package termfont

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIsNerdName(t *testing.T) {
	for name, want := range map[string]bool{
		"JetBrainsMonoNF-Regular": true, "JetBrainsMono Nerd Font": true, "FiraCode NFM": true,
		"MesloLGS NF": true, "Hack Nerd Font Mono": true, "CaskaydiaCove NFP-Regular": true,
		"SFMono-Regular": false, "Menlo-Regular": false, "INFORMAL": false, "Monaco": false, "": false,
	} {
		if got := IsNerdName(name); got != want {
			t.Errorf("%q: %v", name, got)
		}
	}
}

func TestNerdFont(t *testing.T) {
	home := t.TempDir()
	oldHome, oldRun, oldWSL, oldNative := Home, Run, WSL, Native
	defer func() { Home, Run, WSL, Native = oldHome, oldRun, oldWSL, oldNative }()
	Home = func() string { return home }
	WSL = func() bool { return false }
	Native = func() bool { return false }
	font := "SFMono-Regular"
	Run = func(name string, args ...string) ([]byte, error) {
		a := strings.Join(args, " ")
		switch {
		case name == "plutil" && strings.Contains(a, "Default Window Settings"):
			return []byte("Basic\n"), nil
		case name == "plutil" && strings.Contains(a, "Window Settings.Basic.Font"):
			blob := append([]byte{0x04, 0x0b, 's', 't', 'r', 0x81}, []byte(font)...)
			return []byte(base64.StdEncoding.EncodeToString(append(blob, 0x86, 0x84))), nil
		case name == "defaults":
			return []byte(`( { "Normal Font" = "` + font + ` 12"; Name = Default; } )`), nil
		}
		return nil, errors.New("unexpected " + name)
	}
	for _, prog := range []string{"Apple_Terminal", "iTerm.app"} {
		font = "SFMono-Regular"
		if NerdFont(prog) {
			t.Errorf("%s with SF Mono", prog)
		}
		font = "JetBrainsMonoNF-Regular"
		if !NerdFont(prog) {
			t.Errorf("%s with a Nerd Font", prog)
		}
	}
	for prog, want := range map[string]bool{"ghostty": true, "WezTerm": true, "xterm-kitty": true, "": false, "tmux": false} {
		if NerdFont(prog) != want {
			t.Errorf("%q", prog)
		}
	}
	dir := filepath.Join(home, "Library", "Application Support", "Code", "User")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"terminal.integrated.fontFamily": "JetBrainsMono Nerd Font"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !NerdFont("vscode") {
		t.Error("VS Code with a Nerd Font")
	}
}

// TestNerdFontWSL (M43): under WSL the terminal runs on Windows. Windows
// Terminal's font (its profiles' default) and the Windows user's VS Code
// settings decide; started from the Start menu (no TERM_PROGRAM) either
// does.
func TestNerdFontWSL(t *testing.T) {
	win := filepath.Join(t.TempDir(), "Users", "me")
	oldHome, oldWSL, oldHomes, oldNative := Home, WSL, WindowsHomes, Native
	defer func() { Home, WSL, WindowsHomes, Native = oldHome, oldWSL, oldHomes, oldNative }()
	Home = func() string { return t.TempDir() }
	WSL = func() bool { return true }
	Native = func() bool { return false }
	WindowsHomes = func() []string { return []string{win} }
	write := func(rel, body string) {
		t.Helper()
		p := filepath.Join(win, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	wt := "AppData/Local/Packages/Microsoft.WindowsTerminal_8wekyb3d8bbwe/LocalState/settings.json"
	write(wt, `{"profiles": {"defaults": {}, "list": []}}`)
	if NerdFont("") || NerdFont("WindowsTerminal") {
		t.Fatal("Windows Terminal with its default font (Cascadia Mono)")
	}
	write(wt, "{\n  // mine\n  \"profiles\": {\"defaults\": {\"font\": {\"face\": \"JetBrainsMono NF\"}}, \"list\": [],},\n}")
	if !NerdFont("") || !NerdFont("WindowsTerminal") {
		t.Fatal("Windows Terminal with a Nerd Font")
	}
	if NerdFont("vscode") {
		t.Fatal("VS Code without a font set")
	}
	write("AppData/Roaming/Code/User/settings.json", `{"terminal.integrated.fontFamily": "CaskaydiaCove Nerd Font"}`)
	if !NerdFont("vscode") {
		t.Fatal("the Windows user's VS Code with a Nerd Font")
	}
	WSL = func() bool { return false }
	if NerdFont("") {
		t.Fatal("outside WSL an unknown terminal draws no icons")
	}
}

// TestNerdFontNative: on Windows itself the user folder holds Windows
// Terminal's and VS Code's settings; Windows Terminal sets no TERM_PROGRAM.
func TestNerdFontNative(t *testing.T) {
	home := t.TempDir()
	oldHome, oldWSL, oldNative := Home, WSL, Native
	defer func() { Home, WSL, Native = oldHome, oldWSL, oldNative }()
	Home = func() string { return home }
	WSL = func() bool { return false }
	Native = func() bool { return true }
	t.Setenv("IMUX_WINDOWS_HOME", "")
	if got := WindowsHomes(); len(got) != 1 || got[0] != home {
		t.Fatalf("homes %q", got)
	}
	if NerdFont("") {
		t.Fatal("no settings: no icons")
	}
	wt := filepath.Join(home, "AppData", "Local", "Microsoft", "Windows Terminal", "settings.json")
	if err := os.MkdirAll(filepath.Dir(wt), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wt, []byte(`{"profiles": {"defaults": {"font": {"face": "JetBrainsMono NF"}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !NerdFont("") || !NerdFont("WindowsTerminal") {
		t.Fatal("Windows Terminal with a Nerd Font")
	}
}

func TestWindowsHomes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("WSL's PATH is ':'-separated; Go on Windows splits PATH at ';'")
	}
	t.Setenv("PATH", "/usr/bin:/mnt/c/Users/hamza.y/AppData/Local/Programs/Microsoft VS Code/bin:/mnt/c/Users/hamza.y/.dotnet/tools:/mnt/c/WINDOWS")
	if got := WindowsHomes(); len(got) != 1 || got[0] != "/mnt/c/Users/hamza.y" {
		t.Fatalf("%q", got)
	}
	t.Setenv("IMUX_WINDOWS_HOME", "/elsewhere/me")
	if got := WindowsHomes(); len(got) != 1 || got[0] != "/elsewhere/me" {
		t.Fatalf("IMUX_WINDOWS_HOME: %q", got)
	}
}
