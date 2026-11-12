package clipimage

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var png = []byte("\x89PNG\r\n\x1a\nfake")

func reader(t *testing.T, goos string, env map[string]string, tools []string, answer func(name string, args []string) ([]byte, error)) *Reader {
	t.Helper()
	return &Reader{
		Dir: t.TempDir(), GOOS: goos,
		Getenv: func(k string) string { return env[k] },
		LookPath: func(n string) (string, error) {
			for _, x := range tools {
				if x == n {
					return "/usr/bin/" + n, nil
				}
			}
			return "", errors.New("no")
		},
		Output:   func(_ context.Context, name string, args ...string) ([]byte, error) { return answer(name, args) },
		ReadFile: func(string) ([]byte, error) { return []byte("Linux version 6.1 (gcc)"), nil },
		Now:      func() time.Time { return time.Date(2026, 11, 8, 15, 30, 0, 0, time.UTC) },
	}
}

func TestX11AndWayland(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		env  map[string]string
		tool string
	}{{map[string]string{"DISPLAY": ":0"}, "xclip"}, {map[string]string{"WAYLAND_DISPLAY": "wayland-0"}, "wl-paste"}} {
		var asked []string
		r := reader(t, "linux", c.env, []string{c.tool}, func(name string, args []string) ([]byte, error) {
			a := strings.Join(args, " ")
			asked = append(asked, name+" "+a)
			switch {
			case strings.Contains(a, "TARGETS") || strings.Contains(a, "--list-types"):
				return []byte("TARGETS\nimage/png\ntext/plain\n"), nil
			case strings.Contains(a, "image/png"):
				return png, nil
			}
			return nil, errors.New("unexpected")
		})
		if src, _ := r.Source(); src != c.tool {
			t.Fatalf("%s: source %q", c.tool, src)
		}
		got, err := r.Read(ctx)
		if err != nil || len(got) != 1 || filepath.Base(got[0]) != "20261108-153000-1.png" {
			t.Fatalf("%s: %v %v (asked %q)", c.tool, got, err, asked)
		}
		// A second paste of the same second is another file.
		got2, _ := r.Read(ctx)
		if len(got2) != 1 || filepath.Base(got2[0]) != "20261108-153000-2.png" {
			t.Fatalf("%s: second paste %v", c.tool, got2)
		}
		if b, _ := os.ReadFile(got[0]); string(b) != string(png) {
			t.Fatalf("%s: content", c.tool)
		}
		if fi, _ := os.Stat(got[0]); fi.Mode().Perm() != 0o600 {
			t.Fatalf("%s: mode %v", c.tool, fi.Mode().Perm())
		}
	}
	// Copied files (text/uri-list) are used where they are; text alone is no image.
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a b.png"), filepath.Join(dir, "notes.txt")
	for _, p := range []string{a, b} {
		_ = os.WriteFile(p, png, 0o644)
	}
	r := reader(t, "linux", map[string]string{"DISPLAY": ":0"}, []string{"xclip"}, func(name string, args []string) ([]byte, error) {
		a2 := strings.Join(args, " ")
		if strings.Contains(a2, "TARGETS") {
			return []byte("TARGETS\ntext/uri-list\n"), nil
		}
		return []byte("file://" + strings.ReplaceAll(a, " ", "%20") + "\r\nfile://" + b + "\n"), nil
	})
	if got, err := r.Read(ctx); err != nil || len(got) != 1 || got[0] != a {
		t.Fatalf("uri list: %v %v", got, err)
	}
	r = reader(t, "linux", map[string]string{"DISPLAY": ":0"}, []string{"xclip"}, func(string, []string) ([]byte, error) {
		return []byte("TARGETS\nUTF8_STRING\ntext/plain\n"), nil
	})
	if _, err := r.Read(ctx); !errors.Is(err, ErrNoImage) {
		t.Fatalf("text only: %v", err)
	}
}

func TestWSL(t *testing.T) {
	ctx := context.Background()
	env := map[string]string{"WSL_DISTRO_NAME": "Ubuntu", "DISPLAY": ":0"}
	answer := "PNG:" + base64.StdEncoding.EncodeToString(png) + "\r\n"
	r := reader(t, "linux", env, []string{"powershell.exe", "xclip"}, func(name string, args []string) ([]byte, error) {
		if name == "/usr/bin/powershell.exe" && strings.Contains(strings.Join(args, " "), "-STA") {
			return []byte(answer), nil
		}
		if name == "wslpath" {
			return []byte("/mnt/c/Users/me/Pictures/shot.png\n"), nil
		}
		return nil, errors.New("unexpected " + name)
	})
	if !r.WSL() {
		t.Fatal("not WSL")
	}
	if src, _ := r.Source(); !strings.HasPrefix(src, "powershell.exe") {
		t.Fatalf("source %q", src)
	}
	got, err := r.Read(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("image: %v %v", got, err)
	}
	if b, _ := os.ReadFile(got[0]); string(b) != string(png) {
		t.Fatal("content")
	}
	answer = "FILE:C:\\Users\\me\\Pictures\\shot.png\r\nFILE:C:\\Users\\me\\a.docx\r\n"
	if got, err := r.Read(ctx); err != nil || len(got) != 1 || got[0] != "/mnt/c/Users/me/Pictures/shot.png" {
		t.Fatalf("files: %v %v", got, err)
	}
	answer = "NONE\r\n"
	if _, err := r.Read(ctx); !errors.Is(err, ErrNoImage) {
		t.Fatalf("none: %v", err)
	}
	// /proc/version alone tells WSL 1 too.
	r2 := reader(t, "linux", nil, nil, nil)
	r2.ReadFile = func(string) ([]byte, error) { return []byte("Linux version 5.15.153.1-microsoft-standard-WSL2"), nil }
	if !r2.WSL() {
		t.Fatal("WSL from /proc/version")
	}
}

func TestNoTool(t *testing.T) {
	for _, c := range []struct {
		env  map[string]string
		hint string
	}{
		{map[string]string{"DISPLAY": ":0"}, "xclip"},
		{map[string]string{"WAYLAND_DISPLAY": "w"}, "wl-clipboard"},
		{nil, "SSH"},
	} {
		r := reader(t, "linux", c.env, nil, nil)
		_, err := r.Read(context.Background())
		var nt *ErrNoTool
		if !errors.As(err, &nt) || !strings.Contains(nt.Hint, c.hint) {
			t.Errorf("%v: %v", c.env, err)
		}
	}
}

func TestMacAnswer(t *testing.T) {
	dir := t.TempDir()
	r := &Reader{Dir: dir}
	saved := filepath.Join(dir, "x.png")
	_ = os.WriteFile(saved, png, 0o644)
	if got, err := r.answer([]byte("IMAGE\n"), saved); err != nil || got[0] != saved {
		t.Fatalf("image: %v %v", got, err)
	}
	if got, _ := r.answer([]byte("FILE:/a/b.PNG\nFILE:/a/c.pdf\n"), saved); len(got) != 1 || got[0] != "/a/b.PNG" {
		t.Fatalf("files: %v", got)
	}
	if _, err := r.answer([]byte("NONE\n"), saved); !errors.Is(err, ErrNoImage) {
		t.Fatal("none")
	}
}
