// Package clipimage takes the image on the system clipboard (or copied
// image files) as files an agent CLI can read (ARCHITECTURE "Pasting
// images"): macOS, Linux on Wayland or X11, Windows, and WSL (the Windows
// clipboard).
package clipimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/winx"
)

// ErrNoImage means the clipboard holds no image (and no image files).
var ErrNoImage = errors.New("no image on the clipboard")

// ErrNoTool means nothing here can read images from the clipboard.
type ErrNoTool struct{ Hint string }

func (e *ErrNoTool) Error() string { return "cannot read images from the clipboard: " + e.Hint }

// KeepFor is how long saved images are kept.
const KeepFor = 30 * 24 * time.Hour

// imageExt are the file types the agents read as images.
var imageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".bmp": true, ".tif": true, ".tiff": true, ".heic": true}

// IsImage reports whether path names an image file by its extension.
func IsImage(path string) bool { return imageExt[strings.ToLower(filepath.Ext(path))] }

// Reader reads the clipboard. The zero value works; tests replace the
// fields.
type Reader struct {
	Dir      string // where pasted images are saved
	GOOS     string
	Getenv   func(string) string
	LookPath func(string) (string, error)
	// Output runs a command and returns its standard output.
	Output   func(ctx context.Context, name string, args ...string) ([]byte, error)
	ReadFile func(string) ([]byte, error)
	Now      func() time.Time
}

func (r *Reader) goos() string {
	if r.GOOS != "" {
		return r.GOOS
	}
	return runtime.GOOS
}

func (r *Reader) getenv(k string) string {
	if r.Getenv != nil {
		return r.Getenv(k)
	}
	return os.Getenv(k)
}

func (r *Reader) has(name string) (string, bool) {
	lp := r.LookPath
	if lp == nil {
		lp = exec.LookPath
	}
	p, err := lp(name)
	return p, err == nil
}

func (r *Reader) output(ctx context.Context, name string, args ...string) ([]byte, error) {
	if r.Output != nil {
		return r.Output(ctx, name, args...)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	return winx.Hidden(exec.CommandContext(ctx, name, args...)).Output()
}

func (r *Reader) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// WSL reports whether this Linux runs under Windows (WSL 1 or 2).
func (r *Reader) WSL() bool {
	if r.goos() != "linux" {
		return false
	}
	if r.getenv("WSL_DISTRO_NAME") != "" {
		return true
	}
	rf := r.ReadFile
	if rf == nil {
		rf = os.ReadFile
	}
	b, err := rf("/proc/version")
	return err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft")
}

// Source names how the clipboard is read here ("" with a hint when it
// cannot be).
func (r *Reader) Source() (string, string) {
	switch {
	case r.goos() == "darwin":
		return "osascript", ""
	case r.goos() == "windows":
		if _, ok := r.powershell(); ok {
			return "powershell.exe (Windows clipboard)", ""
		}
		return "", "powershell.exe is missing"
	case r.WSL():
		if _, ok := r.powershell(); ok {
			return "powershell.exe (Windows clipboard)", ""
		}
		return "", "powershell.exe is not reachable from WSL (Windows interop is off?)"
	case r.getenv("WAYLAND_DISPLAY") != "":
		if _, ok := r.has("wl-paste"); ok {
			return "wl-paste", ""
		}
		return "", "install wl-clipboard (sudo apt install wl-clipboard)"
	case r.getenv("DISPLAY") != "":
		if _, ok := r.has("xclip"); ok {
			return "xclip", ""
		}
		return "", "install xclip (sudo apt install xclip)"
	}
	return "", "no desktop here (over SSH the clipboard is on your own computer)"
}

func (r *Reader) powershell() (string, bool) {
	if p, ok := r.has("powershell.exe"); ok {
		return p, true
	}
	p := "/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe"
	if r.goos() == "windows" {
		p = winx.WindowsPowerShell()
	}
	if _, err := os.Stat(p); err == nil {
		return p, true
	}
	return "", false
}

// Read returns the clipboard's image as a new file in Dir, or the copied
// image files where they are.
func (r *Reader) Read(ctx context.Context) ([]string, error) {
	r.prune()
	switch {
	case r.goos() == "darwin":
		return r.mac(ctx)
	case r.goos() == "windows":
		if ps, ok := r.powershell(); ok {
			return r.windows(ctx, ps)
		}
	case r.WSL():
		if ps, ok := r.powershell(); ok {
			return r.windows(ctx, ps)
		}
	case r.getenv("WAYLAND_DISPLAY") != "":
		if _, ok := r.has("wl-paste"); ok {
			return r.mime(ctx, func(t string) []string { return []string{"wl-paste", "--no-newline", "--type", t} }, []string{"wl-paste", "--list-types"})
		}
	case r.getenv("DISPLAY") != "":
		if _, ok := r.has("xclip"); ok {
			return r.mime(ctx, func(t string) []string { return []string{"xclip", "-selection", "clipboard", "-t", t, "-o"} }, []string{"xclip", "-selection", "clipboard", "-t", "TARGETS", "-o"})
		}
	}
	_, hint := r.Source()
	return nil, &ErrNoTool{Hint: hint}
}

// newFile is the path of the next pasted image.
func (r *Reader) newFile(ext string) (string, error) {
	if err := os.MkdirAll(r.Dir, 0o700); err != nil {
		return "", err
	}
	stamp := r.now().Format("20060102-150405")
	for n := 1; ; n++ {
		p := filepath.Join(r.Dir, fmt.Sprintf("%s-%d%s", stamp, n, ext))
		if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
			return p, nil
		}
	}
}

func (r *Reader) save(data []byte, ext string) ([]string, error) {
	if len(data) == 0 {
		return nil, ErrNoImage
	}
	p, err := r.newFile(ext)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return nil, err
	}
	return []string{p}, nil
}

// prune deletes saved images older than KeepFor.
func (r *Reader) prune() {
	entries, err := os.ReadDir(r.Dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if fi, err := e.Info(); err == nil && r.now().Sub(fi.ModTime()) > KeepFor && IsImage(e.Name()) {
			_ = os.Remove(filepath.Join(r.Dir, e.Name()))
		}
	}
}

// macScript (JavaScript for Automation) prints FILE:<path> lines for
// copied files, else saves the image (PNG, or TIFF turned into PNG) to the
// path in argv and prints IMAGE, else NONE.
const macScript = `ObjC.import('AppKit');
function run(argv) {
  const pb = $.NSPasteboard.generalPasteboard;
  const opts = $.NSDictionary.dictionaryWithObjectForKey(true, 'NSPasteboardURLReadingFileURLsOnlyKey');
  const urls = pb.readObjectsForClassesOptions($.NSArray.arrayWithObject($.NSURL), opts);
  const out = [];
  if (urls && !urls.isNil()) {
    for (let i = 0; i < urls.count; i++) out.push('FILE:' + urls.objectAtIndex(i).path.js);
  }
  if (out.length > 0) return out.join('\n');
  let d = pb.dataForType('public.png');
  if (d.isNil()) {
    const t = pb.dataForType('public.tiff');
    if (t.isNil()) return 'NONE';
    d = $.NSBitmapImageRep.imageRepWithData(t).representationUsingTypeProperties($.NSBitmapImageFileTypePNG, $.NSDictionary.dictionary);
  }
  return d.writeToFileAtomically(argv[0], true) ? 'IMAGE' : 'NONE';
}`

func (r *Reader) mac(ctx context.Context) ([]string, error) {
	p, err := r.newFile(".png")
	if err != nil {
		return nil, err
	}
	out, err := r.output(ctx, "osascript", "-l", "JavaScript", "-e", macScript, p)
	if err != nil {
		return nil, fmt.Errorf("osascript: %w", err)
	}
	return r.answer(out, p)
}

// answer reads a script's FILE:/IMAGE/NONE answer.
func (r *Reader) answer(out []byte, saved string) ([]string, error) {
	var files []string
	for _, l := range strings.Split(strings.TrimSpace(strings.ReplaceAll(string(out), "\r", "")), "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "FILE:"):
			if f := strings.TrimPrefix(l, "FILE:"); IsImage(f) {
				files = append(files, f)
			}
		case l == "IMAGE":
			if fi, err := os.Stat(saved); err == nil && fi.Size() > 0 {
				_ = os.Chmod(saved, 0o600)
				return []string{saved}, nil
			}
		}
	}
	if len(files) > 0 {
		return files, nil
	}
	return nil, ErrNoImage
}

// mime reads a Wayland or X11 clipboard by type: copied files first
// (text/uri-list), else an image type.
func (r *Reader) mime(ctx context.Context, get func(t string) []string, list []string) ([]string, error) {
	out, err := r.output(ctx, list[0], list[1:]...)
	if err != nil {
		return nil, ErrNoImage // an empty clipboard makes them fail
	}
	types := strings.Fields(string(out))
	has := func(t string) bool {
		for _, x := range types {
			if x == t {
				return true
			}
		}
		return false
	}
	if has("text/uri-list") {
		a := get("text/uri-list")
		if b, err := r.output(ctx, a[0], a[1:]...); err == nil {
			if files := uriImages(string(b)); len(files) > 0 {
				return files, nil
			}
		}
	}
	pick := ""
	for _, t := range append([]string{"image/png"}, types...) {
		if strings.HasPrefix(t, "image/") && has(t) {
			pick = t
			break
		}
	}
	if pick == "" {
		return nil, ErrNoImage
	}
	a := get(pick)
	b, err := r.output(ctx, a[0], a[1:]...)
	if err != nil {
		return nil, err
	}
	ext := "." + strings.TrimPrefix(pick, "image/")
	if ext == ".jpeg" {
		ext = ".jpg"
	}
	return r.save(b, ext)
}

// uriImages are the image files of a text/uri-list.
func uriImages(list string) []string {
	var files []string
	for _, l := range strings.Split(strings.ReplaceAll(list, "\r", ""), "\n") {
		u, err := url.Parse(strings.TrimSpace(l))
		if err != nil || u.Scheme != "file" {
			continue
		}
		if IsImage(u.Path) {
			if _, err := os.Stat(u.Path); err == nil {
				files = append(files, u.Path)
			}
		}
	}
	return files
}

// winScript reads the Windows clipboard: copied files as FILE:<path>
// lines, else the image as PNG:<base64>, else NONE.
const winScript = `$ErrorActionPreference='Stop'
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$f=[System.Windows.Forms.Clipboard]::GetFileDropList()
if ($f.Count -gt 0) { foreach ($x in $f) { 'FILE:' + $x }; exit }
$i=[System.Windows.Forms.Clipboard]::GetImage()
if ($i -eq $null) { 'NONE'; exit }
$m=New-Object System.IO.MemoryStream
$i.Save($m,[System.Drawing.Imaging.ImageFormat]::Png)
'PNG:' + [Convert]::ToBase64String($m.ToArray())`

// nativeScript is winScript for Windows itself: the copied files' paths go
// as UTF-8 in base64 (FILE64:), whatever the console's code page.
var nativeScript = strings.Replace(winScript, "'FILE:' + $x",
	"'FILE64:' + [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($x))", 1)

func (r *Reader) windows(ctx context.Context, ps string) ([]string, error) {
	native := r.goos() == "windows"
	script := winScript
	if native {
		script = nativeScript
	}
	out, err := r.output(ctx, ps, "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	if err != nil {
		return nil, fmt.Errorf("powershell.exe: %w", err)
	}
	var files []string
	for _, l := range strings.Split(strings.ReplaceAll(string(out), "\r", ""), "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "PNG:"):
			b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(l, "PNG:"))
			if err != nil {
				return nil, err
			}
			return r.save(b, ".png")
		case native && strings.HasPrefix(l, "FILE64:"):
			b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(l, "FILE64:"))
			if err == nil && IsImage(string(b)) {
				files = append(files, string(b))
			}
		case !native && strings.HasPrefix(l, "FILE:"):
			win := strings.TrimPrefix(l, "FILE:")
			if !IsImage(win) {
				continue
			}
			p, err := r.output(ctx, "wslpath", "-u", win)
			if err == nil {
				files = append(files, strings.TrimSpace(string(bytes.TrimSpace(p))))
			}
		}
	}
	if len(files) > 0 {
		return files, nil
	}
	return nil, ErrNoImage
}
