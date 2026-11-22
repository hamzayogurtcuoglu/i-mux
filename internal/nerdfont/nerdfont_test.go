package nerdfont

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeWindows replaces the helpers: cmd.exe names folders under root,
// wslpath maps them there, tar "unpacks" the styles, reg.exe and
// powershell.exe are recorded.
type fakeWindows struct {
	root  string
	calls [][]string
}

func (f *fakeWindows) run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	switch name {
	case "cmd.exe":
		return []byte("C:\\Users\\o'me\\AppData\\Local\r\nC:\\Users\\o'me\\AppData\\Roaming\r\nC:\\WINDOWS\r\n"), nil
	case "wslpath":
		p := strings.ReplaceAll(strings.TrimPrefix(args[1], `C:\`), `\`, "/")
		return []byte(filepath.Join(f.root, "c", p) + "\n"), nil
	case "tar":
		dir := args[slices.Index(args, "-C")+1]
		for _, file := range args[4:] {
			if err := os.WriteFile(filepath.Join(dir, file), []byte("font "+file), 0o644); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	return nil, nil
}

func (f *fakeWindows) ran(name string) [][]string {
	var out [][]string
	for _, c := range f.calls {
		if c[0] == name {
			out = append(out, c)
		}
	}
	return out
}

func writeFile(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fake(t *testing.T, wsl bool) *fakeWindows {
	t.Helper()
	f := &fakeWindows{root: t.TempDir()}
	oldRun, oldFetch, oldHome, oldOS, oldWSL, oldSum, oldSys, oldCon := Run, Fetch, Home, GOOS, IsWSL, sum, SystemFonts[Linux], WindowsConsole
	t.Cleanup(func() {
		Run, Fetch, Home, GOOS, IsWSL, sum, SystemFonts[Linux], WindowsConsole = oldRun, oldFetch, oldHome, oldOS, oldWSL, oldSum, oldSys, oldCon
	})
	WindowsConsole = nil // never the real registry
	oldExe := Executable
	exe := filepath.Join(f.root, "bin", "imux")
	Executable = func() (string, error) { return exe, nil } // no font beside it
	t.Cleanup(func() { Executable = oldExe })
	t.Setenv("IMUX_HOME", filepath.Join(f.root, "imux"))
	SystemFonts[Linux] = filepath.Join(f.root, "usr-share-fonts")
	archive := []byte("the archive")
	h := sha256.Sum256(archive)
	sum = hex.EncodeToString(h[:])
	Run, GOOS = f.run, "linux"
	Fetch = func(_ context.Context, _, file string) error { return os.WriteFile(file, archive, 0o644) }
	home := filepath.Join(f.root, "home")
	Home = func() string { return home }
	IsWSL = func() bool { return wsl }
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	return f
}

// TestInstallWSL (M43): under WSL the font goes to the Windows user's fonts
// and is registered for that user; Windows Terminal and VS Code are found
// in the Windows user's folders.
func TestInstallWSL(t *testing.T) {
	f := fake(t, true)
	local := filepath.Join(f.root, "c", "Users", "o'me", "AppData", "Local")
	wt := filepath.Join(local, "Packages", "Microsoft.WindowsTerminal_8wekyb3d8bbwe", "LocalState", "settings.json")
	writeFile(t, wt, `{"profiles": {"defaults": {}, "list": []}}`)
	writeFile(t, filepath.Join(f.root, "c", "Users", "o'me", "AppData", "Roaming", "Code", "User", "settings.json"), "{}")
	s, err := Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind != WSL || s.FontDir != filepath.Join(local, "Microsoft", "Windows", "Fonts") || s.WinDir != `C:\Users\o'me\AppData\Local\Microsoft\Windows\Fonts` ||
		s.SystemDir != filepath.Join(f.root, "c", "WINDOWS", "Fonts") || s.Family != "JetBrainsMono NF" {
		t.Fatalf("%+v", s)
	}
	var names []string
	for _, term := range s.Terminals {
		names = append(names, term.Name)
	}
	if strings.Join(names, ",") != "Windows Terminal,VS Code" {
		t.Fatalf("terminals %v", names)
	}
	if s.Mine() || s.Ready() {
		t.Fatal("nothing installed yet")
	}
	if err := s.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !s.Mine() || len(s.Present()) != 4 {
		t.Fatalf("installed: %v", s.Present())
	}
	reg := f.ran("reg.exe")
	if len(reg) != 4 || strings.Join(reg[0], "|") != `reg.exe|add|HKCU\Software\Microsoft\Windows NT\CurrentVersion\Fonts|/v|JetBrainsMono NF Regular (TrueType)|/t|REG_SZ|/d|C:\Users\o'me\AppData\Local\Microsoft\Windows\Fonts\JetBrainsMonoNerdFont-Regular.ttf|/f` {
		t.Fatalf("reg.exe: %q", reg)
	}
	ps := f.ran("powershell.exe")
	if len(ps) != 1 || !strings.Contains(ps[0][4], `'C:\Users\o''me\AppData\Local\Microsoft\Windows\Fonts\JetBrainsMonoNerdFont-BoldItalic.ttf'`) {
		t.Fatalf("powershell: %q", ps)
	}
	// Installing again over the same files is fine (Windows locks a font in use).
	if err := s.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Ready() {
		t.Fatal("ready while the terminals keep their fonts")
	}
	for _, term := range s.Terminals {
		if _, err := term.Use(s.Family); err != nil {
			t.Fatal(err)
		}
	}
	if !s.Ready() {
		t.Fatal("not ready after the terminals were set")
	}
}

// TestInstallChecksum: a download that is not the release's is refused and
// nothing is installed.
func TestInstallChecksum(t *testing.T) {
	fake(t, false)
	Fetch = func(_ context.Context, _, file string) error {
		return os.WriteFile(file, []byte("something else"), 0o644)
	}
	s, err := Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind != Linux || s.Family != "JetBrainsMono Nerd Font" || !strings.HasSuffix(s.FontDir, filepath.Join("home", ".local", "share", "fonts")) {
		t.Fatalf("%+v", s)
	}
	if err := s.Install(context.Background()); !errors.Is(err, ErrChecksum) {
		t.Fatalf("err %v", err)
	}
	if s.Mine() {
		t.Fatal("installed anyway")
	}
}

// TestUse: Windows Terminal's font is set and the settings kept once; a
// Nerd Font already there stays; VS Code keeps its fonts as fallbacks.
func TestUse(t *testing.T) {
	dir := t.TempDir()
	wt := Terminal{Name: "Windows Terminal", File: filepath.Join(dir, "wt.json"), Key: []string{"profiles", "defaults", "font", "face"}}
	orig := `{"profiles": {"defaults": {"font": {"face": "Cascadia Mono"}}}}`
	writeFile(t, wt.File, orig)
	if changed, err := wt.Use("JetBrainsMono NF"); err != nil || !changed || wt.Font() != "JetBrainsMono NF" {
		t.Fatalf("changed %v err %v font %q", changed, err, wt.Font())
	}
	if b, _ := os.ReadFile(wt.File + ".imux-backup"); string(b) != orig {
		t.Fatalf("backup %q", b)
	}
	writeFile(t, wt.File, `{"profiles": {"defaults": {"font": {"face": "FiraCode Nerd Font"}}}}`)
	if changed, err := wt.Use("JetBrainsMono NF"); err != nil || changed || wt.Font() != "FiraCode Nerd Font" {
		t.Fatalf("a Nerd Font is left alone: %v %v %q", changed, err, wt.Font())
	}
	if b, _ := os.ReadFile(wt.File + ".imux-backup"); string(b) != orig {
		t.Fatal("the first backup is kept")
	}
	vs := Terminal{Name: "VS Code", File: filepath.Join(dir, "vs.json"), Key: []string{"terminal.integrated.fontFamily"}, List: true}
	writeFile(t, vs.File, "{\n    // mine\n    \"terminal.integrated.fontFamily\": \"Consolas, monospace\",\n}\n")
	if _, err := vs.Use("JetBrainsMono NF"); err != nil || vs.Font() != "JetBrainsMono NF, Consolas, monospace" {
		t.Fatalf("err %v font %q", err, vs.Font())
	}
	if b, _ := os.ReadFile(vs.File); !strings.Contains(string(b), "// mine") {
		t.Fatalf("comment lost:\n%s", b)
	}
	bad := Terminal{File: filepath.Join(dir, "bad.json"), Key: []string{"profiles", "defaults", "font", "face"}}
	writeFile(t, bad.File, `{"profiles": [ ]}`)
	if _, err := bad.Use("F"); err == nil {
		t.Fatal("an old Windows Terminal format (profiles as a list) was changed")
	}
}

// TestInstallWindows (T90.6): on native Windows the font goes to the user's fonts and is
// registered as under WSL, the folders from the environment, with no cmd.exe or wslpath.
func TestInstallWindows(t *testing.T) {
	f := fake(t, false)
	GOOS = "windows"
	local, roaming, sysroot := filepath.Join(f.root, "Local"), filepath.Join(f.root, "Roaming"), filepath.Join(f.root, "WINDOWS")
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("APPDATA", roaming)
	t.Setenv("SystemRoot", sysroot)
	writeFile(t, filepath.Join(local, "Packages", "Microsoft.WindowsTerminal_8wekyb3d8bbwe", "LocalState", "settings.json"), `{"profiles": {"defaults": {}}}`)
	s, err := Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if s.Kind != Windows || s.FontDir != filepath.Join(local, "Microsoft", "Windows", "Fonts") || s.WinDir != local+`\Microsoft\Windows\Fonts` ||
		s.SystemDir != filepath.Join(sysroot, "Fonts") || len(s.Terminals) != 1 || s.Terminals[0].Name != "Windows Terminal" {
		t.Fatalf("%+v", s)
	}
	if err := s.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if reg := f.ran("reg.exe"); len(reg) != 4 || reg[0][8] != local+`\Microsoft\Windows\Fonts\JetBrainsMonoNerdFont-Regular.ttf` {
		t.Fatalf("reg.exe: %q", reg)
	}
	if len(f.ran("cmd.exe")) != 0 || len(f.ran("wslpath")) != 0 {
		t.Fatal("cmd.exe or wslpath on native Windows")
	}
}

// fakeConsole is the Windows console's font setting in memory.
type fakeConsole struct{ face string }

func (c *fakeConsole) Font() string                { return c.face }
func (c *fakeConsole) SetFont(family string) error { c.face = family; return nil }
func (c *fakeConsole) Backup() []byte              { return []byte("old: " + c.face) }

// TestConsoleWindows: on native Windows the console window (conhost) is one
// of the terminals: its font is set, the old one kept in a .reg file once,
// and the system is ready only then.
func TestConsoleWindows(t *testing.T) {
	f := fake(t, false)
	GOOS = "windows"
	con := &fakeConsole{face: "Lucida Console"}
	WindowsConsole = con
	local := filepath.Join(f.root, "Local")
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("APPDATA", filepath.Join(f.root, "Roaming"))
	t.Setenv("SystemRoot", filepath.Join(f.root, "WINDOWS"))
	s, err := Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Terminals) != 1 || s.Terminals[0].Name != ConsoleName || s.Terminals[0].Font() != "Lucida Console" {
		t.Fatalf("terminals %+v", s.Terminals)
	}
	if err := s.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s.Ready() {
		t.Fatal("ready while the console keeps Lucida Console")
	}
	term := s.Terminals[0]
	if changed, err := term.Use(s.Family); err != nil || !changed || con.face != FamilyWindows {
		t.Fatalf("changed %v err %v face %q", changed, err, con.face)
	}
	if b, _ := os.ReadFile(term.Backup()); string(b) != "old: Lucida Console" {
		t.Fatalf("backup %q", b)
	}
	if !strings.HasPrefix(term.Backup(), filepath.Join(f.root, "imux")) {
		t.Fatalf("backup outside IMUX_HOME: %s", term.Backup())
	}
	if !s.Ready() {
		t.Fatal("not ready with the console set")
	}
	if changed, _ := term.Use(s.Family); changed {
		t.Fatal("a Nerd Font is left alone")
	}
	// On macOS and Linux there is no console.
	GOOS = "linux"
	if s, _ := Detect(context.Background()); len(s.Terminals) != 0 {
		t.Fatalf("console off Windows: %+v", s.Terminals)
	}
}

// bundle puts the styles beside a fake imux in dir (D94), with sums that
// match them; bad names a style written wrong.
func bundle(t *testing.T, dir, bad string) {
	t.Helper()
	old := slices.Clone(Styles)
	t.Cleanup(func() { Styles = old })
	for i, st := range Styles {
		body := "bundled " + st.File
		h := sha256.Sum256([]byte(body))
		Styles[i].Sum = hex.EncodeToString(h[:])
		if st.File == bad {
			body = "tampered"
		}
		writeFile(t, filepath.Join(dir, st.File), body)
	}
}

// TestInstallBundled (D94): the font that came with imux is used, checked
// file by file, before any download; beside imux, then in
// ../share/imux/fonts; a wrong file makes it download instead.
func TestInstallBundled(t *testing.T) {
	f := fake(t, false)
	bin := filepath.Join(f.root, "prefix", "bin")
	Executable = func() (string, error) { return filepath.Join(bin, "imux"), nil }
	share := filepath.Join(f.root, "prefix", "share", "imux", "fonts")
	bundle(t, share, "")
	fetched := false
	Fetch = func(_ context.Context, _, file string) error {
		fetched = true
		return os.WriteFile(file, []byte("the archive"), 0o644)
	}
	if got := Bundled(); got != filepath.Join(bin, "..", "share", "imux", "fonts") {
		t.Fatalf("Bundled = %q", got)
	}
	s, err := Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(s.FontDir, Styles[0].File))
	if fetched || err != nil || string(b) != "bundled "+Styles[0].File {
		t.Fatalf("fetched %v, installed %q %v", fetched, b, err)
	}
	// fonts/ beside imux comes first.
	bundle(t, filepath.Join(bin, "fonts"), "")
	if got := Bundled(); got != filepath.Join(bin, "fonts") {
		t.Fatalf("Bundled = %q, want fonts/ beside imux", got)
	}
	// A file that does not match its sum is not used: the archive is.
	bundle(t, filepath.Join(bin, "fonts"), Styles[1].File)
	bundle(t, share, Styles[2].File)
	if got := Bundled(); got != "" {
		t.Fatalf("Bundled = %q with wrong files", got)
	}
	if err := s.Install(context.Background()); err != nil || !fetched {
		t.Fatalf("download: %v, fetched %v", err, fetched)
	}
}

// TestUseWindowsTerminalProfiles (D94): Windows Terminal's defaults and
// every profile with a font of its own are set; it is ready only when all
// of them are Nerd Fonts.
func TestUseWindowsTerminalProfiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "settings.json")
	writeFile(t, file, `{
  "profiles": {
    "defaults": {},
    "list": [
      {"name": "PowerShell"},
      {"name": "Command Prompt", "font": {"face": "Consolas"}},
      {"name": "Old", "fontFace": "Lucida Console"},
      {"name": "Mine", "font": {"face": "FiraCode Nerd Font"}}
    ]
  }
}`)
	wt := Terminal{Name: "Windows Terminal", File: file, Key: []string{"profiles", "defaults", "font", "face"}, Profiles: true}
	if wt.Nerd() {
		t.Fatal("ready before")
	}
	changed, err := wt.Use("JetBrainsMono NF")
	if err != nil || !changed {
		t.Fatalf("Use: %v %v", changed, err)
	}
	b, _ := os.ReadFile(file)
	for _, want := range []string{"\"defaults\": {\n      \"font\": {\"face\": \"JetBrainsMono NF\"}\n    }", `"name": "Command Prompt", "font": {"face": "JetBrainsMono NF"}`, `"fontFace": "JetBrainsMono NF"`, `"FiraCode Nerd Font"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("settings lack %s:\n%s", want, b)
		}
	}
	if !wt.Nerd() {
		t.Fatal("not ready after Use")
	}
	if changed, err := wt.Use("JetBrainsMono NF"); err != nil || changed {
		t.Fatalf("a second Use changed it: %v %v", changed, err)
	}
	if bk, err := os.ReadFile(file + ".imux-backup"); err != nil || !strings.Contains(string(bk), "Consolas") {
		t.Fatalf("backup %q %v", bk, err)
	}
}
