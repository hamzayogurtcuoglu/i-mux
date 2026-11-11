// Package termfont tells whether the user's terminal draws Nerd Font icons
// (ARCHITECTURE "Icons, Settings on the rail, updates"): some terminals
// bring the symbols, others draw them only when their font is a Nerd Font.
// On Windows, and under WSL where the terminal runs on Windows, Windows
// Terminal's and VS Code's settings are read in the Windows user's folder.
package termfont

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/hamzayogurtcuoglu/i-mux/internal/jsonc"
)

// nerdRe matches a Nerd Font's name: "JetBrainsMono Nerd Font",
// "JetBrainsMonoNF-Regular", "FiraCode NFM", "MesloLGS NF" (NF, NFM or NFP
// in capitals, not followed by a letter: not "INFO").
var nerdRe = regexp.MustCompile(`(?i:nerd)|NF[MP]?(?:[^A-Za-z]|$)`)

// IsNerdName reports whether a font name is a Nerd Font's.
func IsNerdName(name string) bool { return nerdRe.MatchString(name) }

// Run runs a command and returns its output; tests replace it.
var Run = func(name string, args ...string) ([]byte, error) { return exec.Command(name, args...).Output() }

// Home is the user's home folder; tests replace it.
var Home = func() string { h, _ := os.UserHomeDir(); return h }

// WSL reports Linux running in WSL, whose terminal runs on Windows; tests
// replace it.
var WSL = func() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	b, _ := os.ReadFile("/proc/sys/kernel/osrelease")
	return strings.Contains(strings.ToLower(string(b)), "microsoft")
}

// Native reports native Windows (psmux), where the user folder holds
// Windows Terminal's and VS Code's settings; tests replace it.
var Native = func() bool { return runtime.GOOS == "windows" }

// onWindows reports whether the terminal runs on Windows.
func onWindows() bool { return Native() || WSL() }

// winHomeRe finds a Windows user folder in a WSL path entry.
var winHomeRe = regexp.MustCompile(`^(/mnt/[a-z]/Users/[^/]+)/`)

// WindowsHomes are the Windows user folders WSL sees: $IMUX_WINDOWS_HOME
// when set, else the ones on PATH (WSL adds the Windows PATH:
// /mnt/c/Users/<name>/AppData/...), else every folder in /mnt/*/Users.
// Tests replace it.
var WindowsHomes = func() []string {
	if h := os.Getenv("IMUX_WINDOWS_HOME"); h != "" {
		return []string{h}
	}
	if Native() {
		return []string{Home()}
	}
	var out []string
	seen := map[string]bool{}
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if m := winHomeRe.FindStringSubmatch(p + "/"); m != nil && !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	if len(out) == 0 {
		out, _ = filepath.Glob("/mnt/*/Users/*")
	}
	return out
}

// WTSettings are Windows Terminal's settings files under a user's
// AppData\Local: the Store's release and preview, and an unpackaged install.
var WTSettings = []string{
	"Packages/Microsoft.WindowsTerminal_8wekyb3d8bbwe/LocalState/settings.json",
	"Packages/Microsoft.WindowsTerminalPreview_8wekyb3d8bbwe/LocalState/settings.json",
	"Microsoft/Windows Terminal/settings.json",
}

// WTFont is where Windows Terminal keeps the font of every profile.
var WTFont = []string{"profiles", "defaults", "font", "face"}

// VSCodeFont is VS Code's terminal font setting.
const VSCodeFont = "terminal.integrated.fontFamily"

// NerdFont reports whether terminal program (TERM_PROGRAM, or the client's
// terminal name) draws Nerd Font icons.
func NerdFont(program string) bool {
	p := strings.ToLower(program)
	switch {
	case strings.Contains(p, "ghostty"), strings.Contains(p, "wezterm"), strings.Contains(p, "kitty"):
		return true // they bring the symbols
	case p == "apple_terminal":
		return IsNerdName(terminalFont())
	case strings.Contains(p, "iterm"):
		return IsNerdName(itermFont())
	case p == "vscode":
		return anyNerd(vscodeFonts())
	case p == "windowsterminal":
		return anyNerd(windowsTerminalFonts())
	case p == "" && onWindows():
		// Started from the Start menu, Windows hands WSL to its default
		// terminal and tells it nothing (no TERM_PROGRAM, no WT_SESSION);
		// natively Windows Terminal sets no TERM_PROGRAM either.
		return anyNerd(windowsTerminalFonts()) || anyNerd(vscodeFonts())
	}
	return false
}

func anyNerd(names []string) bool {
	for _, n := range names {
		if IsNerdName(n) {
			return true
		}
	}
	return false
}

// windowsTerminalFonts are the fonts of the Windows Terminals WSL sees ("" for
// one that keeps its default, Cascadia Mono).
func windowsTerminalFonts() []string {
	if !onWindows() {
		return nil
	}
	var out []string
	for _, home := range WindowsHomes() {
		for _, rel := range WTSettings {
			b, err := os.ReadFile(filepath.Join(home, "AppData", "Local", filepath.FromSlash(rel)))
			if err != nil {
				continue
			}
			face, _ := jsonc.Get(b, WTFont...)
			out = append(out, face)
		}
	}
	return out
}

// terminalFont is the font of Terminal.app's default profile: its archived
// NSFont holds the PostScript name as text.
func terminalFont() string {
	plist := filepath.Join(Home(), "Library", "Preferences", "com.apple.Terminal.plist")
	prof, err := Run("plutil", "-extract", "Default Window Settings", "raw", "-o", "-", plist)
	if err != nil {
		return ""
	}
	name := strings.TrimSpace(string(prof))
	if name == "" || strings.Contains(name, ".") {
		return "" // plutil key paths cannot hold a dot
	}
	data, err := Run("plutil", "-extract", "Window Settings."+name+".Font", "raw", "-o", "-", plist)
	if err != nil {
		return ""
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		return ""
	}
	return printable(b)
}

// itermFont is the "Normal Font" of iTerm2's profiles (the first one).
func itermFont() string {
	out, err := Run("defaults", "read", "com.googlecode.iterm2", "New Bookmarks")
	if err != nil {
		return ""
	}
	m := regexp.MustCompile(`"Normal Font"\s*=\s*"([^"]+)"`).FindStringSubmatch(string(out))
	if m == nil {
		return ""
	}
	return m[1]
}

// vscodeFonts are VS Code's terminal fonts: macOS's and Linux's settings,
// and on Windows or under WSL the Windows user's (VS Code runs there).
func vscodeFonts() []string {
	dirs := []string{
		filepath.Join(Home(), "Library", "Application Support", "Code", "User"),
		filepath.Join(Home(), ".config", "Code", "User"),
	}
	if onWindows() {
		for _, home := range WindowsHomes() {
			dirs = append(dirs, filepath.Join(home, "AppData", "Roaming", "Code", "User"))
		}
	}
	var out []string
	for _, dir := range dirs {
		b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
		if err != nil {
			continue
		}
		if f, ok := jsonc.Get(b, VSCodeFont); ok {
			out = append(out, f)
		}
	}
	return out
}

// printable keeps the readable runs of b (font names inside archived data).
func printable(b []byte) string {
	var out strings.Builder
	run := 0
	for _, c := range b {
		if c >= 0x20 && c < 0x7f {
			out.WriteByte(c)
			run++
			continue
		}
		if run > 0 {
			out.WriteByte(' ')
		}
		run = 0
	}
	return out.String()
}
