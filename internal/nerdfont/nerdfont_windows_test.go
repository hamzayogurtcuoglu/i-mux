//go:build windows

package nerdfont

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows/registry"
)

// testConsole is a console setting in a private registry key, removed after
// the test: never the user's HKCU\Console.
func testConsole(t *testing.T) regConsole {
	t.Helper()
	base := fmt.Sprintf(`Software\imux-test-%d`, time.Now().UnixNano())
	c := regConsole{root: registry.CURRENT_USER, path: base + `\Console`}
	t.Cleanup(func() {
		for _, p := range []string{c.path + `\Git Bash`, c.path + `\%SystemRoot%_System32_WindowsPowerShell_v1.0_powershell.exe`, c.path + `\colors`, c.path, base} {
			_ = registry.DeleteKey(registry.CURRENT_USER, p)
		}
	})
	set := func(path string, values map[string]any) {
		k, _, err := registry.CreateKey(c.root, path, registry.ALL_ACCESS)
		if err != nil {
			t.Fatal(err)
		}
		defer k.Close()
		for name, v := range values {
			switch v := v.(type) {
			case string:
				err = k.SetStringValue(name, v)
			case int:
				err = k.SetDWordValue(name, uint32(v))
			}
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	set(c.path, map[string]any{"FaceName": defaultFace, "FontFamily": 0, "FontSize": 16 << 16})
	set(c.path+`\%SystemRoot%_System32_WindowsPowerShell_v1.0_powershell.exe`, map[string]any{"FaceName": "Lucida Console", "FontFamily": 54})
	set(c.path+`\Git Bash`, map[string]any{"FaceName": "Terminal", "FontFamily": 48, "FontSize": 12<<16 | 8})
	set(c.path+`\colors`, map[string]any{"ScreenColors": 7}) // no font of its own
	return c
}

// TestConsoleKeyEnv: $IMUX_CONSOLE_KEY moves the console's setting to
// another key (testutil.Isolate's), so no test reads the user's.
func TestConsoleKeyEnv(t *testing.T) {
	c := testConsole(t)
	t.Setenv("IMUX_CONSOLE_KEY", c.path)
	con := WindowsConsole.(regConsole)
	if con.base() != c.path || con.keys()[0] != c.path || len(con.keys()) != 3 {
		t.Fatalf("keys %q, want under %s", con.keys(), c.path)
	}
	t.Setenv("IMUX_CONSOLE_KEY", "")
	if con.base() != `Console` {
		t.Fatalf("base %q", con.base())
	}
}

// TestRegConsole: the console's defaults and every key with a font of its
// own (PowerShell's wins over the defaults) get the font; a raster font's
// cell size becomes a TrueType height; the backup puts the old values back.
func TestRegConsole(t *testing.T) {
	c := testConsole(t)
	if f := c.Font(); f != "" {
		t.Fatalf("font %q, want the default", f)
	}
	backup := c.Backup()
	if err := c.SetFont(FamilyWindows); err != nil {
		t.Fatal(err)
	}
	if f := c.Font(); f != FamilyWindows {
		t.Fatalf("font %q", f)
	}
	for _, p := range c.keys() {
		if c.face(p) != FamilyWindows {
			t.Fatalf("%s: %q", p, c.face(p))
		}
		if fam, _ := c.value(p, "FontFamily"); fam != uint64(familyTrueType) {
			t.Fatalf("%s: family %v", p, fam)
		}
	}
	if len(c.keys()) != 3 {
		t.Fatalf("keys %q", c.keys())
	}
	if size, _ := c.value(c.path+`\Git Bash`, "FontSize"); size != uint64(size16px) {
		t.Fatalf("raster size kept: %v", size)
	}
	if size, _ := c.value(c.path, "FontSize"); size != uint64(16<<16) {
		t.Fatalf("default size changed: %v", size)
	}
	if _, ok := c.value(c.path+`\colors`, "FaceName"); ok {
		t.Fatal("a key without a font got one")
	}

	if len(backup) < 2 || backup[0] != 0xff || backup[1] != 0xfe {
		t.Fatal("the backup is not UTF-16 as regedit writes it")
	}
	u := make([]uint16, 0, len(backup)/2)
	for i := 2; i+1 < len(backup); i += 2 {
		u = append(u, uint16(backup[i])|uint16(backup[i+1])<<8)
	}
	text := string(utf16.Decode(u))
	for _, want := range []string{
		"Windows Registry Editor Version 5.00",
		`[HKEY_CURRENT_USER\` + c.path + `\%SystemRoot%_System32_WindowsPowerShell_v1.0_powershell.exe]`,
		`"FaceName"="Lucida Console"`, `"FontFamily"=dword:00000036`, `"FontSize"=-`, `"FaceName"="Terminal"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("backup lacks %q:\n%s", want, text)
		}
	}
}
