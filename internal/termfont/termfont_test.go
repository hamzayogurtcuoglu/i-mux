package termfont

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
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
	oldHome, oldRun := Home, Run
	defer func() { Home, Run = oldHome, oldRun }()
	Home = func() string { return home }
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
