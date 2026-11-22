package termfont

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
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
	oldHome, oldRun, oldWSL, oldWin := Home, Run, WSL, Windows
	defer func() { Home, Run, WSL, Windows = oldHome, oldRun, oldWSL, oldWin }()
	Home = func() string { return home }
	// macOS's and Linux's rules: on Windows "" would read the real Windows
	// Terminal's settings.
	WSL = func() bool { return false }
	Windows = func() bool { return false }
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
	testutil.SkipOnWindows(t, "WSL's Windows Terminal; native Windows has its own tests")
	win := filepath.Join(t.TempDir(), "Users", "me")
	oldHome, oldWSL, oldHomes, oldInst := Home, WSL, WindowsHomes, nerdInstalled
	defer func() { Home, WSL, WindowsHomes, nerdInstalled = oldHome, oldWSL, oldHomes, oldInst }()
	nerdInstalled = func() bool { return true }
	Home = func() string { return t.TempDir() }
	WSL = func() bool { return true }
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

func TestWindowsHomes(t *testing.T) {
	testutil.SkipOnWindows(t, "WSL's /mnt/c homes")
	t.Setenv("PATH", "/usr/bin:/mnt/c/Users/hamza.y/AppData/Local/Programs/Microsoft VS Code/bin:/mnt/c/Users/hamza.y/.dotnet/tools:/mnt/c/WINDOWS")
	if got := WindowsHomes(); len(got) != 1 || got[0] != "/mnt/c/Users/hamza.y" {
		t.Fatalf("%q", got)
	}
	t.Setenv("IMUX_WINDOWS_HOME", "/elsewhere/me")
	if got := WindowsHomes(); len(got) != 1 || got[0] != "/elsewhere/me" {
		t.Fatalf("IMUX_WINDOWS_HOME: %q", got)
	}
}

// On native Windows Windows Terminal's and VS Code's settings are the user's own, in
// %LOCALAPPDATA% and %APPDATA% (§7.10).
func TestNerdFontWindows(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", filepath.Join(root, "Local"))
	t.Setenv("APPDATA", filepath.Join(root, "Roaming"))
	oldHome, oldWSL, oldWin, oldInst := Home, WSL, Windows, nerdInstalled
	defer func() { Home, WSL, Windows, nerdInstalled = oldHome, oldWSL, oldWin, oldInst }()
	nerdInstalled = func() bool { return true }
	Home = func() string { return t.TempDir() }
	WSL = func() bool { return false }
	Windows = func() bool { return true }
	p := filepath.Join(root, "Local", "Packages", "Microsoft.WindowsTerminal_8wekyb3d8bbwe", "LocalState", "settings.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if NerdFont("") {
		t.Fatal("no Windows Terminal settings")
	}
	if err := os.WriteFile(p, []byte(`{"profiles": {"defaults": {"font": {"face": "JetBrainsMono NF"}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !NerdFont("") || !NerdFont("WindowsTerminal") {
		t.Fatal("Windows Terminal with a Nerd Font")
	}
	// A console window (conhost) tells its own font, whatever Windows
	// Terminal's settings say.
	if NerdFont(Conhost+"Lucida Console") || NerdFont(Conhost) {
		t.Fatal("conhost without a Nerd Font")
	}
	if !NerdFont(Conhost+"JetBrainsMono NF") || !NerdFont(Conhost+"FiraCode Nerd Font Mono") {
		t.Fatal("conhost with a Nerd Font")
	}
}

// TestWindowsTerminalProfiles (D94): the font of the profile the client runs
// in decides, its own over the defaults'; no Nerd Font installed, no icons.
func TestWindowsTerminalProfiles(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", filepath.Join(root, "Local"))
	t.Setenv("APPDATA", filepath.Join(root, "Roaming"))
	oldHome, oldWSL, oldWin, oldInst := Home, WSL, Windows, nerdInstalled
	defer func() { Home, WSL, Windows, nerdInstalled = oldHome, oldWSL, oldWin, oldInst }()
	Home = func() string { return t.TempDir() }
	WSL = func() bool { return false }
	Windows = func() bool { return true }
	installed := true
	nerdInstalled = func() bool { return installed }
	settings := `{
  "defaultProfile": "{0caa0dad-35be-5f56-a8ff-afceeeaa6101}",
  "profiles": {
    "defaults": {"font": {"face": "JetBrainsMono NF"}},
    "list": [
      {"guid": "{61c54bbd-c2c6-5271-96e7-009a87ff44bf}", "name": "Windows PowerShell"},
      {"guid": "{0caa0dad-35be-5f56-a8ff-afceeeaa6101}", "name": "Command Prompt", "font": {"face": "Consolas"}},
      {"guid": "{11111111-2222-3333-4444-555555555555}", "name": "Old", "fontFace": "Lucida Console"},
    ]
  }
}`
	p := filepath.Join(root, "Local", "Packages", "Microsoft.WindowsTerminal_8wekyb3d8bbwe", "LocalState", "settings.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	b := []byte(settings)
	for guid, want := range map[string]string{
		"":                                       "Consolas", // the default profile
		"{61C54BBD-C2C6-5271-96E7-009A87FF44BF}": "JetBrainsMono NF",
		"{0caa0dad-35be-5f56-a8ff-afceeeaa6101}": "Consolas",
		"{11111111-2222-3333-4444-555555555555}": "Lucida Console",
		"{99999999-0000-0000-0000-000000000000}": "JetBrainsMono NF",
	} {
		if got := WTFace(b, guid); got != want {
			t.Errorf("WTFace(%q) = %q, want %q", guid, got, want)
		}
	}
	if NerdFont("WindowsTerminal") || NerdFont("WindowsTerminal:{0caa0dad-35be-5f56-a8ff-afceeeaa6101}") {
		t.Error("Command Prompt keeps Consolas: no icons")
	}
	if !NerdFont("WindowsTerminal:{61c54bbd-c2c6-5271-96e7-009a87ff44bf}") {
		t.Error("PowerShell takes the defaults' Nerd Font")
	}
	installed = false
	if NerdFont("WindowsTerminal:{61c54bbd-c2c6-5271-96e7-009a87ff44bf}") {
		t.Error("icons with no Nerd Font installed")
	}
	var paths []string
	for _, f := range WTFontSettings(b) {
		paths = append(paths, strings.Join(f.Path, ".")+"="+f.Face)
	}
	if got := strings.Join(paths, "|"); got != "profiles.defaults.font.face=JetBrainsMono NF|profiles.list.#1.font.face=Consolas|profiles.list.#2.fontFace=Lucida Console" {
		t.Errorf("settings %s", got)
	}
	// The older profiles list: every profile, a face or not.
	old := []byte(`{"defaultProfile": "{a}", "profiles": [{"guid": "{a}"}, {"guid": "{b}", "fontFace": "Consolas"}]}`)
	paths = nil
	for _, f := range WTFontSettings(old) {
		paths = append(paths, strings.Join(f.Path, ".")+"="+f.Face)
	}
	if got := strings.Join(paths, "|"); got != "profiles.#0.font.face=|profiles.#1.fontFace=Consolas" {
		t.Errorf("older list %s", got)
	}
	if WTFace(old, "") != "Cascadia Mono" || WTFace(old, "{b}") != "Consolas" {
		t.Error("older list faces")
	}
}
