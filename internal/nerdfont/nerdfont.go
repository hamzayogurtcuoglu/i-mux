// Package nerdfont puts the Nerd Font the sidebar's icons need where the
// user's terminal draws text, and points the terminals i-mux can set at it
// (D34; ARCHITECTURE "Icons, Settings on the rail, updates"). The font is
// Nerd Fonts' JetBrainsMono, checked against its release's SHA-256 and
// installed for the user alone (no administrator rights). Under WSL the
// terminal runs on Windows: the font goes to the Windows user's fonts, and
// Windows Terminal's and VS Code's settings there name it.
package nerdfont

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/jsonc"
	"github.com/hamzayogurtcuoglu/i-mux/internal/termfont"
)

// The font i-mux installs.
const (
	Release = "v3.5.1"
	Archive = "JetBrainsMono.tar.xz"
	// Sum is the archive's SHA-256, as the release's SHA-256.txt lists it.
	Sum = "04d5e8f903693f9dd13e16f867e994834e681eb3c72c0d337a770dcda09010cf"
	// SizeMB is about what the download weighs.
	SizeMB = 7
	// Family is the font's name for terminals on macOS and Linux. Windows
	// programs are given its legacy family name, which all of them find.
	Family        = "JetBrainsMono Nerd Font"
	FamilyWindows = "JetBrainsMono NF"
)

// Style is one file of the font and its full name (the Windows registry's).
type Style struct{ File, Name string }

// Styles are the four a terminal uses, of the archive's 96.
var Styles = []Style{
	{"JetBrainsMonoNerdFont-Regular.ttf", "JetBrainsMono NF Regular"},
	{"JetBrainsMonoNerdFont-Bold.ttf", "JetBrainsMono NF Bold"},
	{"JetBrainsMonoNerdFont-Italic.ttf", "JetBrainsMono NF Italic"},
	{"JetBrainsMonoNerdFont-BoldItalic.ttf", "JetBrainsMono NF Bold Italic"},
}

// URL is where the archive is downloaded from; $IMUX_NERD_FONT_URL replaces
// it (a mirror: the file must still match Sum).
func URL() string {
	if u := os.Getenv("IMUX_NERD_FONT_URL"); u != "" {
		return u
	}
	return "https://github.com/ryanoasis/nerd-fonts/releases/download/" + Release + "/" + Archive
}

// Kinds of system.
const (
	MacOS = "macos"
	Linux = "linux"
	WSL   = "wsl" // Linux in WSL: the terminal runs on Windows
)

// Terminal is a terminal whose settings file i-mux edits.
type Terminal struct {
	Name string   // "Windows Terminal", "VS Code"
	File string   // its settings.json
	Key  []string // its font setting
	List bool     // the setting is a font list (CSS): the font goes first
}

// System is where the user's terminal finds its fonts.
type System struct {
	Kind      string
	FontDir   string     // where the font goes (under WSL the Windows user's fonts, under /mnt)
	WinDir    string     // WSL: FontDir as Windows names it (the registry's)
	SystemDir string     // fonts installed for every user (only looked at)
	Family    string     // the font's name for the terminals
	Terminals []Terminal // the ones on this system
}

var (
	// Run runs a helper program (tar, fc-cache; under WSL cmd.exe, wslpath,
	// reg.exe, powershell.exe); tests replace it.
	Run = run
	// Fetch downloads url into file; tests replace it.
	Fetch = fetch
	// Home is the user's home folder; tests replace it.
	Home = func() string { h, _ := os.UserHomeDir(); return h }
	// GOOS and IsWSL tell the system; tests replace them.
	GOOS  = runtime.GOOS
	IsWSL = func() bool { return termfont.WSL() }
)

// SystemFonts are the folders of fonts installed for every user (Windows':
// %SystemRoot%\Fonts); tests replace them.
var SystemFonts = map[string]string{MacOS: "/Library/Fonts", Linux: "/usr/share/fonts"}

// winSystem32 is where Windows' programs are when WSL's PATH lacks them.
var winSystem32 = []string{"/mnt/c/Windows/System32", "/mnt/c/Windows/System32/WindowsPowerShell/v1.0"}

func run(ctx context.Context, name string, args ...string) ([]byte, error) {
	var dir string
	if strings.HasSuffix(name, ".exe") {
		if _, err := exec.LookPath(name); err != nil {
			for _, d := range winSystem32 {
				if p := filepath.Join(d, name); isFile(p) {
					name = p
					break
				}
			}
		}
		dir = "/mnt/c" // Windows programs want a Windows folder to start in
	}
	c := exec.CommandContext(ctx, name, args...)
	c.Dir = dir
	return c.CombinedOutput()
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// winPath is a Windows folder as Windows and WSL name it.
type winPath struct{ win, unix string }

// windowsDirs are the Windows user's AppData\Local and AppData\Roaming and
// the Windows folder (cmd.exe says them, wslpath turns them into WSL paths).
func windowsDirs(ctx context.Context) (local, roaming, sysroot winPath, err error) {
	out, err := Run(ctx, "cmd.exe", "/d", "/c", "echo %LOCALAPPDATA%&echo %APPDATA%&echo %SystemRoot%")
	if err != nil {
		return local, roaming, sysroot, fmt.Errorf("cmd.exe: %w", err)
	}
	var dirs []winPath
	for _, l := range strings.Split(strings.ReplaceAll(string(out), "\r", ""), "\n") {
		l = strings.TrimSpace(l)
		if len(l) < 3 || l[1] != ':' {
			continue // not a path: a warning
		}
		u, err := Run(ctx, "wslpath", "-u", l)
		if err != nil {
			return local, roaming, sysroot, fmt.Errorf("wslpath %s: %w", l, err)
		}
		dirs = append(dirs, winPath{win: l, unix: strings.TrimSpace(string(u))})
	}
	if len(dirs) != 3 {
		return local, roaming, sysroot, fmt.Errorf("cmd.exe did not tell the Windows folders: %q", out)
	}
	return dirs[0], dirs[1], dirs[2], nil
}

// Detect finds where the font goes and the terminals i-mux can set.
func Detect(ctx context.Context) (*System, error) {
	home := Home()
	switch {
	case GOOS == "darwin":
		s := &System{Kind: MacOS, FontDir: filepath.Join(home, "Library", "Fonts"), SystemDir: SystemFonts[MacOS], Family: Family}
		s.vscode(filepath.Join(home, "Library", "Application Support"))
		return s, nil
	case IsWSL():
		local, roaming, sysroot, err := windowsDirs(ctx)
		if err != nil {
			return nil, err
		}
		s := &System{
			Kind: WSL, FontDir: filepath.Join(local.unix, "Microsoft", "Windows", "Fonts"),
			WinDir: local.win + `\Microsoft\Windows\Fonts`, SystemDir: filepath.Join(sysroot.unix, "Fonts"), Family: FamilyWindows,
		}
		for i, rel := range termfont.WTSettings {
			name := "Windows Terminal"
			if i == 1 {
				name = "Windows Terminal Preview"
			}
			s.add(Terminal{Name: name, File: filepath.Join(local.unix, filepath.FromSlash(rel)), Key: termfont.WTFont})
		}
		s.vscode(roaming.unix)
		return s, nil
	}
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	conf := os.Getenv("XDG_CONFIG_HOME")
	if conf == "" {
		conf = filepath.Join(home, ".config")
	}
	s := &System{Kind: Linux, FontDir: filepath.Join(data, "fonts"), SystemDir: SystemFonts[Linux], Family: Family}
	s.vscode(conf)
	return s, nil
}

// add keeps a terminal whose settings file exists.
func (s *System) add(t Terminal) {
	if isFile(t.File) {
		s.Terminals = append(s.Terminals, t)
	}
}

// vscode adds VS Code's user settings under base (its "Code" folder's parent).
func (s *System) vscode(base string) {
	for _, v := range []struct{ name, dir string }{{"VS Code", "Code"}, {"VS Code Insiders", "Code - Insiders"}} {
		s.add(Terminal{Name: v.name, File: filepath.Join(base, v.dir, "User", "settings.json"), Key: []string{termfont.VSCodeFont}, List: true})
	}
}

// Mine reports whether i-mux's font is installed.
func (s *System) Mine() bool {
	for _, st := range Styles {
		if !isFile(filepath.Join(s.FontDir, st.File)) {
			return false
		}
	}
	return true
}

// Present lists the Nerd Font files the terminal can use: i-mux's and any
// other, for the user or for everyone.
func (s *System) Present() []string {
	var out []string
	for _, dir := range []string{s.FontDir, s.SystemDir} {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if strings.Count(strings.TrimPrefix(p, dir), string(filepath.Separator)) > 3 {
					return fs.SkipDir
				}
				return nil
			}
			ext := strings.ToLower(filepath.Ext(d.Name()))
			if (ext == ".ttf" || ext == ".otf") && termfont.IsNerdName(strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))) {
				out = append(out, p)
			}
			return nil
		})
	}
	return out
}

// Ready reports whether nothing is left to do: a Nerd Font is installed and
// every terminal i-mux can set uses one.
func (s *System) Ready() bool {
	if len(s.Present()) == 0 {
		return false
	}
	for _, t := range s.Terminals {
		if !termfont.IsNerdName(t.Font()) {
			return false
		}
	}
	return true
}

// Install puts the font's Styles into FontDir: downloaded, checked against
// Sum and unpacked with tar; registered for the Windows user under WSL, and
// made known to fontconfig on Linux.
func (s *System) Install(ctx context.Context) error {
	tmp, err := os.MkdirTemp("", "imux-font-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	archive := filepath.Join(tmp, Archive)
	if err := Fetch(ctx, URL(), archive); err != nil {
		return err
	}
	if err := check(archive, sum); err != nil {
		return err
	}
	args := []string{"-xJf", archive, "-C", tmp}
	for _, st := range Styles {
		args = append(args, st.File)
	}
	if out, err := Run(ctx, "tar", args...); err != nil {
		return fmt.Errorf("tar: %v: %s", err, bytes.TrimSpace(out))
	}
	if err := os.MkdirAll(s.FontDir, 0o755); err != nil {
		return err
	}
	for _, st := range Styles {
		if err := place(filepath.Join(tmp, st.File), filepath.Join(s.FontDir, st.File)); err != nil {
			return err
		}
	}
	switch s.Kind {
	case WSL:
		return s.register(ctx)
	case Linux:
		_, _ = Run(ctx, "fc-cache", "-f", s.FontDir) // without fontconfig there is nobody to tell
	}
	return nil
}

// fontsKey is where Windows lists the fonts installed for the user.
const fontsKey = `HKCU\Software\Microsoft\Windows NT\CurrentVersion\Fonts`

// register lists the font for the Windows user, as Windows' own "Install"
// does, then loads it in this session and tells running programs.
func (s *System) register(ctx context.Context) error {
	var files []string
	for _, st := range Styles {
		win := s.WinDir + `\` + st.File
		files = append(files, "'"+strings.ReplaceAll(win, "'", "''")+"'")
		if out, err := Run(ctx, "reg.exe", "add", fontsKey, "/v", st.Name+" (TrueType)", "/t", "REG_SZ", "/d", win, "/f"); err != nil {
			return fmt.Errorf("reg.exe: %v: %s", err, bytes.TrimSpace(out))
		}
	}
	// Best effort: programs started before see the font only after this
	// (WM_FONTCHANGE); a terminal may still want a restart.
	script := `Add-Type -Namespace IMux -Name Font -MemberDefinition '` +
		`[DllImport("gdi32.dll", CharSet = CharSet.Unicode)] public static extern int AddFontResource(string f);` +
		`[DllImport("user32.dll")] public static extern IntPtr SendMessageTimeout(IntPtr h, uint m, UIntPtr w, IntPtr l, uint f, uint t, out UIntPtr r);'; ` +
		`foreach ($f in @(` + strings.Join(files, ",") + `)) { [void][IMux.Font]::AddFontResource($f) }; ` +
		`$r = [UIntPtr]::Zero; [void][IMux.Font]::SendMessageTimeout([IntPtr]0xffff, 0x1D, [UIntPtr]::Zero, [IntPtr]::Zero, 2, 1000, [ref]$r)`
	_, _ = Run(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	return nil
}

// place copies a font file to dst unless dst already holds it (Windows
// keeps a font in use locked).
func place(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if old, err := os.ReadFile(dst); err == nil && bytes.Equal(old, b) {
		return nil
	}
	tmp := dst + ".part"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// ErrChecksum means the download is not the release's archive.
var ErrChecksum = errors.New("the downloaded font does not match its checksum")

// sum is the checksum Install wants (Sum; tests replace it).
var sum = Sum

func check(file, want string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return ErrChecksum
	}
	return nil
}

func fetch(ctx context.Context, url, file string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Font is the font the terminal's settings name ("" for its default).
func (t Terminal) Font() string {
	b, err := os.ReadFile(t.File)
	if err != nil {
		return ""
	}
	f, _ := jsonc.Get(b, t.Key...)
	return f
}

// Use sets the terminal's font to family, unless it is a Nerd Font
// already. The first change keeps the settings as they were in
// <file>.imux-backup. It reports whether it changed the file.
func (t Terminal) Use(family string) (bool, error) {
	b, err := os.ReadFile(t.File)
	if err != nil {
		return false, err
	}
	cur, _ := jsonc.Get(b, t.Key...)
	if termfont.IsNerdName(cur) {
		return false, nil
	}
	v := family
	if t.List && strings.TrimSpace(cur) != "" {
		v = family + ", " + cur // the rest stay as fallbacks
	}
	out, err := jsonc.Set(b, v, t.Key...)
	if err != nil {
		return false, err
	}
	backup := t.File + ".imux-backup"
	if _, err := os.Stat(backup); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(backup, b, 0o644); err != nil {
			return false, err
		}
	}
	// Written in place: the file may be a link into the user's dotfiles.
	return true, os.WriteFile(t.File, out, 0o644)
}
