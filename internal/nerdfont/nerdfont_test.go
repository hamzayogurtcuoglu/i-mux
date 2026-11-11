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
	oldRun, oldFetch, oldHome, oldOS, oldWSL, oldSum, oldSys := Run, Fetch, Home, GOOS, IsWSL, sum, SystemFonts[Linux]
	t.Cleanup(func() {
		Run, Fetch, Home, GOOS, IsWSL, sum, SystemFonts[Linux] = oldRun, oldFetch, oldHome, oldOS, oldWSL, oldSum, oldSys
	})
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

// TestInstallWindows: on Windows itself the font goes to
// %LOCALAPPDATA%\Microsoft\Windows\Fonts, listed under HKCU like Windows'
// own per-user "Install"; Windows Terminal and VS Code are found in
// %LOCALAPPDATA% and %APPDATA%.
func TestInstallWindows(t *testing.T) {
	f := fake(t, false)
	GOOS = "windows"
	local := filepath.Join(f.root, "Local")
	roaming := filepath.Join(f.root, "Roaming")
	t.Setenv("LOCALAPPDATA", local)
	t.Setenv("APPDATA", roaming)
	t.Setenv("SystemRoot", filepath.Join(f.root, "WINDOWS"))
	writeFile(t, filepath.Join(local, "Microsoft", "Windows Terminal", "settings.json"), `{"profiles": {"defaults": {}}}`)
	writeFile(t, filepath.Join(roaming, "Code", "User", "settings.json"), `{"terminal.integrated.fontFamily": "Consolas"}`)
	s, err := Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fonts := filepath.Join(local, "Microsoft", "Windows", "Fonts")
	if s.Kind != Windows || s.FontDir != fonts || s.WinDir != local+`\Microsoft\Windows\Fonts` ||
		s.SystemDir != filepath.Join(f.root, "WINDOWS", "Fonts") || s.Family != FamilyWindows {
		t.Fatalf("%+v", s)
	}
	var names []string
	for _, term := range s.Terminals {
		names = append(names, term.Name)
	}
	if strings.Join(names, ",") != "Windows Terminal,VS Code" {
		t.Fatalf("terminals %v", names)
	}
	if len(f.ran("cmd.exe")) != 0 || len(f.ran("wslpath")) != 0 {
		t.Fatalf("WSL helpers on Windows: %q", f.calls)
	}
	if err := s.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !s.Mine() {
		t.Fatal("not installed")
	}
	reg := f.ran("reg.exe")
	if len(reg) != 4 || reg[0][4] != "JetBrainsMono NF Regular (TrueType)" || reg[0][8] != local+`\Microsoft\Windows\Fonts\JetBrainsMonoNerdFont-Regular.ttf` {
		t.Fatalf("reg.exe: %q", reg)
	}
	if len(f.ran("powershell.exe")) != 1 {
		t.Fatalf("powershell: %q", f.calls)
	}
	for _, term := range s.Terminals {
		if _, err := term.Use(s.Family); err != nil {
			t.Fatal(err)
		}
	}
	if !s.Ready() {
		t.Fatal("not ready after the terminals were set")
	}
	if got := s.Terminals[1].Font(); got != "JetBrainsMono NF, Consolas" {
		t.Fatalf("VS Code font %q", got)
	}
}
