// Package termfont tells whether the user's terminal draws Nerd Font icons
// (ARCHITECTURE "Icons, Settings on the rail, updates"): some terminals
// bring the symbols, others draw them only when their font is a Nerd Font.
package termfont

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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
		return IsNerdName(vscodeFont())
	}
	return false
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

// vscodeFont is VS Code's terminal font family.
func vscodeFont() string {
	for _, dir := range []string{
		filepath.Join(Home(), "Library", "Application Support", "Code", "User"),
		filepath.Join(Home(), ".config", "Code", "User"),
	} {
		b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
		if err != nil {
			continue
		}
		m := regexp.MustCompile(`"terminal\.integrated\.fontFamily"\s*:\s*"([^"]*)"`).FindSubmatch(b)
		if m != nil {
			return string(m[1])
		}
	}
	return ""
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
