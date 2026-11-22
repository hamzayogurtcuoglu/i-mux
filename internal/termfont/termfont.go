// Package termfont tells whether the user's terminal draws Nerd Font icons
// (ARCHITECTURE "Icons, Settings on the rail, updates"): some terminals
// bring the symbols, others draw them only when their font is a Nerd Font.
// Under WSL the terminal runs on Windows: its settings are read there.
package termfont

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
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

// Windows reports native Windows, where Windows Terminal's and VS Code's
// settings are the user's own (§7.10); tests replace it.
var Windows = func() bool { return runtime.GOOS == "windows" }

// windowsAppData is the user's AppData\Local and AppData\Roaming folders, for
// each Windows user i-mux can see: under WSL theirs, on Windows the user's.
func windowsAppData() (locals, roamings []string) {
	if Windows() {
		return []string{os.Getenv("LOCALAPPDATA")}, []string{os.Getenv("APPDATA")}
	}
	if !WSL() {
		return nil, nil
	}
	for _, home := range WindowsHomes() {
		locals = append(locals, filepath.Join(home, "AppData", "Local"))
		roamings = append(roamings, filepath.Join(home, "AppData", "Roaming"))
	}
	return locals, roamings
}

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

// Conhost starts the terminal name of a Windows console window (conhost),
// which tells no TERM_PROGRAM: the font follows ("conhost:Consolas"), as
// the console itself reports it to the client.
const Conhost = "conhost:"

// NerdFont reports whether terminal program (TERM_PROGRAM, or the client's
// terminal name) draws Nerd Font icons.
func NerdFont(program string) bool {
	p := strings.ToLower(program)
	switch {
	case strings.HasPrefix(program, Conhost):
		return IsNerdName(strings.TrimPrefix(program, Conhost))
	case strings.Contains(p, "ghostty"), strings.Contains(p, "wezterm"), strings.Contains(p, "kitty"):
		return true // they bring the symbols
	case p == "apple_terminal":
		return IsNerdName(terminalFont())
	case strings.Contains(p, "iterm"):
		return IsNerdName(itermFont())
	case p == "vscode":
		return anyNerd(vscodeFonts())
	case p == "vscode" && (WSL() || Windows()):
		return anyNerd(vscodeFonts()) && nerdInstalled()
	case p == "vscode":
		return anyNerd(vscodeFonts())
	case p == "windowsterminal" || strings.HasPrefix(p, "windowsterminal:"):
		// The profile the client runs in (WT_PROFILE_ID, D94), else the
		// default one; its font must be a Nerd Font that is installed.
		_, guid, _ := strings.Cut(program, ":")
		return anyNerd(windowsTerminalFaces(guid)) && nerdInstalled()
	case p == "" && (WSL() || Windows()):
		// Started from the Start menu, Windows hands WSL to its default
		// terminal and tells it nothing (no TERM_PROGRAM, no WT_SESSION);
		// on Windows itself the terminal is Windows Terminal or VS Code too.
		return (anyNerd(windowsTerminalFaces("")) || anyNerd(vscodeFonts())) && nerdInstalled()
	}
	return false
}

// WTFontSetting is a place in Windows Terminal's settings that sets the font
// of some profiles: its path (for jsonc) and the face it names ("" for
// none, which is Cascadia Mono).
type WTFontSetting struct {
	Path []string
	Face string
}

// WTFontSettings are the font settings Windows Terminal's profiles use
// (D94): with a profiles object, the defaults' face and every profile's own
// (font.face, or the older fontFace); with the older profiles list, every
// profile's, including those that name none.
func WTFontSettings(b []byte) []WTFontSetting {
	var out []WTFontSetting
	add := func(path ...string) bool {
		face, ok := jsonc.Get(b, path...)
		if ok {
			out = append(out, WTFontSetting{Path: path, Face: face})
		}
		return ok
	}
	if jsonc.IsArray(b, "profiles") {
		n, _ := jsonc.Len(b, "profiles")
		for i := range n {
			e := "#" + strconv.Itoa(i)
			a := add("profiles", e, "font", "face")
			o := add("profiles", e, "fontFace")
			if !a && !o {
				out = append(out, WTFontSetting{Path: []string{"profiles", e, "font", "face"}})
			}
		}
		return out
	}
	if !add(WTFont...) {
		out = append(out, WTFontSetting{Path: WTFont})
	}
	n, _ := jsonc.Len(b, "profiles", "list")
	for i := range n {
		e := "#" + strconv.Itoa(i)
		add("profiles", "list", e, "font", "face")
		add("profiles", "list", e, "fontFace")
	}
	return out
}

// WTFace is the font Windows Terminal draws profile guid with ("" for the
// default profile): its own, else the defaults', else Cascadia Mono.
func WTFace(b []byte, guid string) string {
	if guid == "" {
		guid, _ = jsonc.Get(b, "defaultProfile")
	}
	list := []string{"profiles", "list"}
	if jsonc.IsArray(b, "profiles") {
		list = []string{"profiles"}
	}
	n, _ := jsonc.Len(b, list...)
	for i := range n {
		p := append(slices.Clone(list), "#"+strconv.Itoa(i))
		if g, _ := jsonc.Get(b, append(slices.Clone(p), "guid")...); guid == "" || !strings.EqualFold(g, guid) {
			continue
		}
		if f, ok := jsonc.Get(b, append(slices.Clone(p), "font", "face")...); ok && f != "" {
			return f
		}
		if f, ok := jsonc.Get(b, append(slices.Clone(p), "fontFace")...); ok && f != "" {
			return f
		}
		break
	}
	if f, ok := jsonc.Get(b, WTFont...); ok && f != "" {
		return f
	}
	return "Cascadia Mono"
}

// nerdInstalled reports whether a Nerd Font is installed where Windows
// draws text (Windows and WSL): in the user's or the system's fonts.
// Tests replace it.
var nerdInstalled = func() bool {
	var dirs []string
	locals, _ := windowsAppData()
	for _, l := range locals {
		dirs = append(dirs, filepath.Join(l, "Microsoft", "Windows", "Fonts"))
	}
	if Windows() {
		dirs = append(dirs, filepath.Join(os.Getenv("SystemRoot"), "Fonts"))
	} else if WSL() {
		sys, _ := filepath.Glob("/mnt/*/Windows/Fonts")
		dirs = append(dirs, sys...)
	}
	for _, d := range dirs {
		ents, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range ents {
			name := e.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if (ext == ".ttf" || ext == ".otf") && IsNerdName(strings.TrimSuffix(name, filepath.Ext(name))) {
				return true
			}
		}
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

// windowsTerminalFaces are the fonts the Windows Terminals WSL or Windows
// sees draw profile guid with ("" for the default profile).
func windowsTerminalFaces(guid string) []string {
	locals, _ := windowsAppData()
	var out []string
	for _, local := range locals {
		for _, rel := range WTSettings {
			b, err := os.ReadFile(filepath.Join(local, filepath.FromSlash(rel)))
			if err != nil {
				continue
			}
			out = append(out, WTFace(b, guid))
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
// and under WSL the Windows user's (VS Code runs there).
func vscodeFonts() []string {
	dirs := []string{
		filepath.Join(Home(), "Library", "Application Support", "Code", "User"),
		filepath.Join(Home(), ".config", "Code", "User"),
	}
	_, roamings := windowsAppData()
	for _, r := range roamings {
		dirs = append(dirs, filepath.Join(r, "Code", "User"))
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
