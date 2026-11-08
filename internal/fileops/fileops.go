// Package fileops does what the explorer's menus ask (§9.3 "Explorer menus
// and hover"): new files and folders, rename, the Trash, copy / move with
// free names, the system clipboard for paths, and revealing a file in the
// desktop's file manager.
package fileops

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/clipimage"
)

// ErrExists is a name that is taken.
var ErrExists = errors.New("a file or folder with this name already exists")

// ErrBadName is an empty name, an absolute one, or one leaving the folder.
var ErrBadName = errors.New("not a valid name")

// ErrIntoItself is a folder pasted into itself.
var ErrIntoItself = errors.New("a folder cannot be pasted into itself")

// cleanName checks a typed name: relative, inside the folder ("a/b.txt"
// makes folder a), not empty.
func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.TrimRight(name, "/")
	if name == "" || filepath.IsAbs(name) {
		return "", ErrBadName
	}
	c := filepath.Clean(name)
	if c == "." || c == ".." || strings.HasPrefix(c, "../") {
		return "", ErrBadName
	}
	return c, nil
}

// Create makes file (or folder) name in dir, with the folders on the way;
// it returns the new path.
func Create(dir, name string, folder bool) (string, error) {
	n, err := cleanName(name)
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, n)
	if _, err := os.Lstat(p); err == nil {
		return "", ErrExists
	}
	if folder {
		return p, os.MkdirAll(p, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", ErrExists
		}
		return "", err
	}
	return p, f.Close()
}

// Rename gives path the name newName (in its folder; "sub/x" moves it into
// sub, made when missing) and returns the new path.
func Rename(path, newName string) (string, error) {
	n, err := cleanName(newName)
	if err != nil {
		return "", err
	}
	to := filepath.Join(filepath.Dir(path), n)
	if to == path {
		return path, nil
	}
	// A case-only rename on a case-insensitive disk is the same file.
	if _, err := os.Lstat(to); err == nil && !sameFile(path, to) {
		return "", ErrExists
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return "", err
	}
	return to, os.Rename(path, to)
}

func sameFile(a, b string) bool {
	fa, err1 := os.Lstat(a)
	fb, err2 := os.Lstat(b)
	return err1 == nil && err2 == nil && os.SameFile(fa, fb)
}

// FreeName is name in dir, or the first free "name copy.ext", "name copy
// 2.ext" ... (a folder: "name copy").
func FreeName(dir, name string, folder bool) string {
	if _, err := os.Lstat(filepath.Join(dir, name)); err != nil {
		return name
	}
	base, ext := name, ""
	if !folder {
		ext = filepath.Ext(name)
		if ext == name { // ".env"
			ext = ""
		}
		base = strings.TrimSuffix(name, ext)
	}
	for i := 1; ; i++ {
		n := base + " copy" + ext
		if i > 1 {
			n = base + " copy " + strconv.Itoa(i) + ext
		}
		if _, err := os.Lstat(filepath.Join(dir, n)); err != nil {
			return n
		}
	}
}

// Paste copies (or, move, moves) src into folder dir under a free name and
// returns the new path.
func Paste(src, dir string, move bool) (string, error) {
	fi, err := os.Lstat(src)
	if err != nil {
		return "", err
	}
	if fi.IsDir() {
		rel, err := filepath.Rel(src, dir)
		if err == nil && (rel == "." || !strings.HasPrefix(rel, "..")) {
			return "", ErrIntoItself
		}
	}
	if move && filepath.Dir(src) == filepath.Clean(dir) {
		return src, nil // cut and pasted where it is
	}
	to := filepath.Join(dir, FreeName(dir, filepath.Base(src), fi.IsDir()))
	if move {
		err := os.Rename(src, to)
		if err == nil {
			return to, nil
		}
		if !errors.Is(err, syscall.EXDEV) {
			return "", err
		}
		// Another disk: copy, then remove.
		if err := copyAll(src, to); err != nil {
			_ = os.RemoveAll(to)
			return "", err
		}
		return to, os.RemoveAll(src)
	}
	if err := copyAll(src, to); err != nil {
		_ = os.RemoveAll(to)
		return "", err
	}
	return to, nil
}

// copyAll copies a file, a link or a folder tree, keeping modes.
func copyAll(src, dst string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		t, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(t, dst)
	case fi.IsDir():
		if err := os.Mkdir(dst, fi.Mode().Perm()|0o700); err != nil {
			return err
		}
		ents, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range ents {
			if err := copyAll(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return os.Chmod(dst, fi.Mode().Perm())
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fi.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// System is what the platform functions need; the zero value is the real
// machine.
type System struct {
	GOOS     string
	Getenv   func(string) string
	LookPath func(string) (string, error)
	// Run runs a command with stdin and waits; nil = os/exec.
	Run func(ctx context.Context, stdin string, name string, args ...string) error
	// Output runs a command and returns its stdout; nil = os/exec.
	Output func(ctx context.Context, name string, args ...string) ([]byte, error)
	Now    func() time.Time
	// ReadFile reads /proc/version (WSL); nil = os.ReadFile.
	ReadFile func(string) ([]byte, error)
}

func (s *System) goos() string {
	if s.GOOS != "" {
		return s.GOOS
	}
	return runtime.GOOS
}

func (s *System) env(k string) string {
	if s.Getenv != nil {
		return s.Getenv(k)
	}
	return os.Getenv(k)
}

func (s *System) look(n string) bool {
	if s.LookPath != nil {
		_, err := s.LookPath(n)
		return err == nil
	}
	_, err := exec.LookPath(n)
	return err == nil
}

func (s *System) run(ctx context.Context, stdin, name string, args ...string) error {
	if s.Run != nil {
		return s.Run(ctx, stdin, name, args...)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	// No pipes on stdout: xclip and wl-copy stay behind to serve the
	// clipboard, and a pipe would keep Wait waiting.
	cmd.Stdout, cmd.Stderr = nil, nil
	return cmd.Run()
}

func (s *System) output(ctx context.Context, name string, args ...string) ([]byte, error) {
	if s.Output != nil {
		return s.Output(ctx, name, args...)
	}
	return exec.CommandContext(ctx, name, args...).Output()
}

func (s *System) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *System) wsl() bool {
	r := &clipimage.Reader{GOOS: s.GOOS, Getenv: s.Getenv, ReadFile: s.ReadFile}
	return s.goos() == "linux" && r.WSL()
}

// desktop reports a Linux graphical session.
func (s *System) desktop() bool {
	return s.env("WAYLAND_DISPLAY") != "" || s.env("DISPLAY") != ""
}

// CopyText puts text on the system clipboard. IMUX_COPY_COMMAND (an sh
// command reading stdin) replaces it.
func (s *System) CopyText(ctx context.Context, text string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if c := s.env("IMUX_COPY_COMMAND"); c != "" {
		return s.run(ctx, text, "/bin/sh", "-c", c)
	}
	switch {
	case s.goos() == "darwin":
		return s.run(ctx, text, "pbcopy")
	case s.wsl() && s.look("powershell.exe"):
		// clip.exe reads the console code page; Set-Clipboard keeps any
		// character. The text goes as UTF-8 bytes in base64.
		ps := "Set-Clipboard -Value ([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('" +
			b64(text) + "')))"
		return s.run(ctx, "", "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", ps)
	case s.env("WAYLAND_DISPLAY") != "" && s.look("wl-copy"):
		return s.run(ctx, text, "wl-copy")
	case s.env("DISPLAY") != "" && s.look("xclip"):
		return s.run(ctx, text, "xclip", "-selection", "clipboard", "-i")
	case s.env("DISPLAY") != "" && s.look("xsel"):
		return s.run(ctx, text, "xsel", "--clipboard", "--input")
	}
	return errors.New("no clipboard tool")
}

// Reveal is how this machine shows a file in its file manager: an i18n key
// for the menu ("" when there is no desktop).
func (s *System) Reveal() string {
	switch {
	case s.goos() == "darwin":
		return "file.reveal_finder"
	case s.wsl():
		return "file.reveal_explorer"
	case s.goos() == "linux" && s.desktop():
		return "file.reveal_folder"
	}
	return ""
}

// RevealFile shows path in the file manager, selected where it can.
func (s *System) RevealFile(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	switch {
	case s.goos() == "darwin":
		return s.run(ctx, "", "open", "-R", path)
	case s.wsl():
		w, err := s.output(ctx, "wslpath", "-w", path)
		if err != nil {
			return err
		}
		// explorer.exe exits 1 even when it worked.
		_ = s.run(ctx, "", "explorer.exe", "/select,"+strings.TrimSpace(string(w)))
		return nil
	}
	if s.look("gdbus") {
		u := (&url.URL{Scheme: "file", Path: path}).String()
		err := s.run(ctx, "", "gdbus", "call", "--session", "--dest", "org.freedesktop.FileManager1",
			"--object-path", "/org/freedesktop/FileManager1", "--method", "org.freedesktop.FileManager1.ShowItems",
			"['"+strings.ReplaceAll(u, "'", "%27")+"']", "")
		if err == nil {
			return nil
		}
	}
	dir := path
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		dir = filepath.Dir(path)
	}
	return s.run(ctx, "", "xdg-open", dir)
}

// Trash moves path to the Trash. IMUX_TRASH_DIR replaces the Trash (a
// plain folder; tests).
func (s *System) Trash(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if d := s.env("IMUX_TRASH_DIR"); d != "" {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
		fi, err := os.Lstat(path)
		if err != nil {
			return err
		}
		return os.Rename(path, filepath.Join(d, FreeName(d, filepath.Base(path), fi.IsDir())))
	}
	if s.goos() == "darwin" {
		// NSFileManager's trash: the Finder's Put Back works.
		js := `ObjC.import('Foundation');
function run(argv) {
  var err = $();
  var ok = $.NSFileManager.defaultManager.trashItemAtURLResultingItemURLError($.NSURL.fileURLWithPath(argv[0]), null, err);
  if (!ok) { throw new Error(err.localizedDescription ? ObjC.unwrap(err.localizedDescription) : 'trash failed'); }
}`
		out, err := exec.CommandContext(ctx, "osascript", "-l", "JavaScript", "-e", js, path).CombinedOutput()
		if err != nil {
			return errors.New(osaError(string(out), err))
		}
		return nil
	}
	if s.look("gio") {
		if err := s.run(ctx, "", "gio", "trash", "--", path); err == nil {
			return nil
		}
	}
	return s.homeTrash(path)
}

// homeTrash is the freedesktop.org Trash in the home folder: the file in
// files/, a .trashinfo with its old place in info/.
func (s *System) homeTrash(path string) error {
	data := s.env("XDG_DATA_HOME")
	if data == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		data = filepath.Join(home, ".local", "share")
	}
	root := filepath.Join(data, "Trash")
	files, info := filepath.Join(root, "files"), filepath.Join(root, "info")
	for _, d := range []string{files, info} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return err
		}
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	name := FreeName(files, filepath.Base(path), fi.IsDir())
	for {
		if _, err := os.Lstat(filepath.Join(info, name+".trashinfo")); err != nil {
			break
		}
		name = FreeName(files, name+"_", fi.IsDir())
	}
	abs, _ := filepath.Abs(path)
	u := (&url.URL{Path: abs}).EscapedPath()
	ti := "[Trash Info]\nPath=" + u + "\nDeletionDate=" + s.now().Format("2006-01-02T15:04:05") + "\n"
	infoFile := filepath.Join(info, name+".trashinfo")
	if err := os.WriteFile(infoFile, []byte(ti), 0o600); err != nil {
		return err
	}
	if err := os.Rename(path, filepath.Join(files, name)); err != nil {
		_ = os.Remove(infoFile)
		return err
	}
	return nil
}

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// osaError is osascript's message without its decoration: "execution
// error: Error: Error: <message> (-2700)".
func osaError(out string, err error) string {
	m := strings.TrimSpace(out)
	if i := strings.LastIndex(m, "Error: "); i >= 0 {
		m = m[i+len("Error: "):]
	}
	m = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(m), "(-2700)"))
	if m == "" {
		return err.Error()
	}
	return m
}
