//go:build windows

package nerdfont

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows/registry"

	"github.com/hamzayogurtcuoglu/i-mux/internal/termfont"
)

func init() { WindowsConsole = regConsole{root: registry.CURRENT_USER} }

// consoleKey is the console's settings under HKCU: Console, or
// $IMUX_CONSOLE_KEY (tests: never the user's).
func consoleKey() string {
	if k := os.Getenv("IMUX_CONSOLE_KEY"); k != "" {
		return k
	}
	return `Console`
}

// program is the helper to run for name: Windows' own tar (bsdtar, which
// unpacks .tar.xz and takes C:\ paths), not one that comes first on PATH
// (Git for Windows' GNU tar reads "C:" as a remote host).
func program(name string) string {
	if name == "tar" {
		if p := filepath.Join(os.Getenv("SystemRoot"), "System32", "tar.exe"); isFile(p) {
			return p
		}
	}
	return name
}

// regConsole is the Windows console's font (conhost) in the registry, under
// HKCU\Console: the defaults, and the keys of programs and shortcuts that
// name a font of their own (PowerShell's, Git Bash's), which win over them.
type regConsole struct {
	root registry.Key
	path string // "" for consoleKey()
}

// base is the key holding the defaults.
func (c regConsole) base() string {
	if c.path != "" {
		return c.path
	}
	return consoleKey()
}

// defaultFace is FaceName for the console's default TrueType font.
const defaultFace = "__DefaultTTFont__"

// The console's font values, as SetFont writes them.
const (
	familyTrueType = 0x36       // FF_MODERN | TMPF_TRUETYPE | TMPF_VECTOR
	weightNormal   = 400        // FW_NORMAL
	size16px       = 16 << 16   // FontSize: height in the high word
	rasterCell     = 0x0000ffff // a raster font's size has a width
)

var fontValues = []string{"FaceName", "FontFamily", "FontSize", "FontWeight"}

// keys are the console's keys that name a font, the defaults first.
func (c regConsole) keys() []string {
	base := c.base()
	out := []string{base}
	k, err := registry.OpenKey(c.root, base, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return out
	}
	defer k.Close()
	names, _ := k.ReadSubKeyNames(-1)
	sort.Strings(names)
	for _, n := range names {
		if _, ok := c.value(base+`\`+n, "FaceName"); ok {
			out = append(out, base+`\`+n)
		}
	}
	return out
}

// value reads a string or DWORD value of key path.
func (c regConsole) value(path, name string) (any, bool) {
	k, err := registry.OpenKey(c.root, path, registry.QUERY_VALUE)
	if err != nil {
		return nil, false
	}
	defer k.Close()
	if s, _, err := k.GetStringValue(name); err == nil {
		return s, true
	}
	if n, _, err := k.GetIntegerValue(name); err == nil {
		return n, true
	}
	return nil, false
}

func (c regConsole) face(path string) string {
	v, _ := c.value(path, "FaceName")
	f, _ := v.(string)
	if f == defaultFace {
		return ""
	}
	return f
}

// Font is the font a console window opens with: the first of the keys'
// that is not a Nerd Font ("" for the default), else the defaults'.
func (c regConsole) Font() string {
	keys := c.keys()
	for _, p := range keys {
		if f := c.face(p); !termfont.IsNerdName(f) {
			return f
		}
	}
	return c.face(keys[0])
}

// SetFont makes family the font of the defaults and of every key with a
// font of its own. A raster font's cell size becomes a 16 px height.
func (c regConsole) SetFont(family string) error {
	for _, p := range c.keys() {
		k, _, err := registry.CreateKey(c.root, p, registry.QUERY_VALUE|registry.SET_VALUE)
		if err != nil {
			return fmt.Errorf(`HKCU\%s: %w`, p, err)
		}
		err = k.SetStringValue("FaceName", family)
		if err == nil {
			err = k.SetDWordValue("FontFamily", familyTrueType)
		}
		if err == nil {
			err = k.SetDWordValue("FontWeight", weightNormal)
		}
		if size, _, e := k.GetIntegerValue("FontSize"); e == nil && err == nil && (size == 0 || size&rasterCell != 0) {
			err = k.SetDWordValue("FontSize", size16px)
		}
		k.Close()
		if err != nil {
			return fmt.Errorf(`HKCU\%s: %w`, p, err)
		}
	}
	return nil
}

// Backup is the keys' font values as a .reg file (UTF-16, as regedit
// writes it): opening it puts them back, absent values deleted.
func (c regConsole) Backup() []byte {
	var b strings.Builder
	b.WriteString("Windows Registry Editor Version 5.00\r\n")
	for _, p := range c.keys() {
		fmt.Fprintf(&b, "\r\n[HKEY_CURRENT_USER\\%s]\r\n", p)
		for _, name := range fontValues {
			v, ok := c.value(p, name)
			switch v := v.(type) {
			case string:
				fmt.Fprintf(&b, "%q=\"%s\"\r\n", name, strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v))
			case uint64:
				fmt.Fprintf(&b, "%q=dword:%08x\r\n", name, uint32(v))
			default:
				if !ok {
					fmt.Fprintf(&b, "%q=-\r\n", name)
				}
			}
		}
	}
	u := utf16.Encode([]rune(b.String()))
	out := []byte{0xff, 0xfe}
	for _, r := range u {
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}
