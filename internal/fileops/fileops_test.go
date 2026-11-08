package fileops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCreate(t *testing.T) {
	d := t.TempDir()
	p, err := Create(d, "a.txt", false)
	if err != nil || p != filepath.Join(d, "a.txt") {
		t.Fatalf("%q %v", p, err)
	}
	if _, err := Create(d, "a.txt", false); !errors.Is(err, ErrExists) {
		t.Fatalf("exists: %v", err)
	}
	if p, err := Create(d, "x/y/z.go", false); err != nil || p != filepath.Join(d, "x/y/z.go") {
		t.Fatalf("nested: %q %v", p, err)
	}
	if p, err := Create(d, "dir #1/", true); err != nil {
		t.Fatalf("folder: %v", err)
	} else if fi, _ := os.Stat(p); !fi.IsDir() {
		t.Fatal("not a folder")
	}
	for _, bad := range []string{"", "  ", "/etc/x", "..", "../x", "a/../../x", "."} {
		if _, err := Create(d, bad, false); !errors.Is(err, ErrBadName) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestRename(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "a.txt"), "a")
	write(t, filepath.Join(d, "b.txt"), "b")
	if _, err := Rename(filepath.Join(d, "a.txt"), "b.txt"); !errors.Is(err, ErrExists) {
		t.Fatalf("onto b: %v", err)
	}
	p, err := Rename(filepath.Join(d, "a.txt"), "c d.txt")
	if err != nil || p != filepath.Join(d, "c d.txt") {
		t.Fatalf("%q %v", p, err)
	}
	if b, _ := os.ReadFile(p); string(b) != "a" {
		t.Fatal("content")
	}
	// A case-only rename (the same file on macOS's disk).
	if p, err = Rename(p, "C D.txt"); err != nil {
		t.Fatalf("case: %v", err)
	}
	if _, err := Rename(p, "../out"); !errors.Is(err, ErrBadName) {
		t.Fatalf("out: %v", err)
	}
}

func TestFreeNameAndPaste(t *testing.T) {
	d := t.TempDir()
	write(t, filepath.Join(d, "a.go"), "x")
	write(t, filepath.Join(d, ".env"), "e")
	if n := FreeName(d, "a.go", false); n != "a copy.go" {
		t.Fatal(n)
	}
	if n := FreeName(d, ".env", false); n != ".env copy" {
		t.Fatal(n)
	}
	// Copy twice into the same folder: a copy, then a copy 2.
	p1, err := Paste(filepath.Join(d, "a.go"), d, false)
	if err != nil || filepath.Base(p1) != "a copy.go" {
		t.Fatalf("%q %v", p1, err)
	}
	p2, _ := Paste(filepath.Join(d, "a.go"), d, false)
	if filepath.Base(p2) != "a copy 2.go" {
		t.Fatal(p2)
	}
	// A folder tree, with a link, copies whole; it cannot go into itself.
	write(t, filepath.Join(d, "src/sub/f.txt"), "f")
	if err := os.Symlink("sub/f.txt", filepath.Join(d, "src/link")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(d, "dst"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := Paste(filepath.Join(d, "src"), filepath.Join(d, "dst"), false)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(p, "sub/f.txt")); string(b) != "f" {
		t.Fatal("tree content")
	}
	if l, _ := os.Readlink(filepath.Join(p, "link")); l != "sub/f.txt" {
		t.Fatal("link", l)
	}
	if _, err := Paste(filepath.Join(d, "src"), filepath.Join(d, "src/sub"), false); !errors.Is(err, ErrIntoItself) {
		t.Fatalf("into itself: %v", err)
	}
	if _, err := Paste(filepath.Join(d, "src"), filepath.Join(d, "src"), true); !errors.Is(err, ErrIntoItself) {
		t.Fatalf("onto itself: %v", err)
	}
	// "srcx" is not inside "src".
	if err := os.MkdirAll(filepath.Join(d, "srcx"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Paste(filepath.Join(d, "src"), filepath.Join(d, "srcx"), false); err != nil {
		t.Fatalf("sibling with the same prefix: %v", err)
	}
	// Move: gone from the old place; in its own folder, nothing happens.
	m, err := Paste(filepath.Join(d, "a.go"), filepath.Join(d, "dst"), true)
	if err != nil || m != filepath.Join(d, "dst/a.go") {
		t.Fatalf("%q %v", m, err)
	}
	if _, err := os.Stat(filepath.Join(d, "a.go")); err == nil {
		t.Fatal("moved file still there")
	}
	if same, err := Paste(m, filepath.Join(d, "dst"), true); err != nil || same != m {
		t.Fatalf("cut and pasted in place: %q %v", same, err)
	}
}

func TestTrash(t *testing.T) {
	d := t.TempDir()
	trash := filepath.Join(d, "trash")
	s := &System{Getenv: func(k string) string {
		if k == "IMUX_TRASH_DIR" {
			return trash
		}
		return ""
	}}
	for i := 0; i < 2; i++ {
		write(t, filepath.Join(d, "x.txt"), "x")
		if err := s.Trash(context.Background(), filepath.Join(d, "x.txt")); err != nil {
			t.Fatal(err)
		}
	}
	if ents, _ := os.ReadDir(trash); len(ents) != 2 {
		t.Fatalf("trash: %v", ents)
	}

	// The freedesktop home Trash (Linux without gio).
	data := filepath.Join(d, "data")
	s = &System{
		GOOS:     "linux",
		Getenv:   func(k string) string { return map[string]string{"XDG_DATA_HOME": data}[k] },
		LookPath: func(string) (string, error) { return "", errors.New("no") },
		Now:      func() time.Time { return time.Date(2026, 11, 8, 20, 1, 2, 0, time.Local) },
	}
	write(t, filepath.Join(d, "my file.txt"), "m")
	if err := s.Trash(context.Background(), filepath.Join(d, "my file.txt")); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(data, "Trash/files/my file.txt")); string(b) != "m" {
		t.Fatal("not in Trash/files")
	}
	info, _ := os.ReadFile(filepath.Join(data, "Trash/info/my file.txt.trashinfo"))
	if !strings.Contains(string(info), "Path="+strings.ReplaceAll(filepath.Join(d, "my file.txt"), " ", "%20")+"\n") ||
		!strings.Contains(string(info), "DeletionDate=2026-11-08T20:01:02") {
		t.Fatalf("trashinfo:\n%s", info)
	}
}

func TestCopyTextAndReveal(t *testing.T) {
	var ran []string
	sys := func(goos string, env map[string]string, tools ...string) *System {
		ran = nil
		return &System{
			GOOS:   goos,
			Getenv: func(k string) string { return env[k] },
			LookPath: func(n string) (string, error) {
				for _, x := range tools {
					if x == n {
						return n, nil
					}
				}
				return "", errors.New("no")
			},
			Run: func(_ context.Context, stdin, name string, args ...string) error {
				ran = append(ran, name+" "+strings.Join(args, " ")+" <"+stdin)
				return nil
			},
			Output: func(_ context.Context, name string, args ...string) ([]byte, error) {
				return []byte(`C:\p\a.txt` + "\n"), nil
			},
			ReadFile: func(string) ([]byte, error) { return nil, errors.New("no") },
		}
	}
	ctx := context.Background()
	cases := []struct {
		s    *System
		want string
	}{
		{sys("darwin", nil), "pbcopy  <ç/x"},
		{sys("linux", map[string]string{"WAYLAND_DISPLAY": "w"}, "wl-copy"), "wl-copy  <ç/x"},
		{sys("linux", map[string]string{"DISPLAY": ":0"}, "xclip"), "xclip -selection clipboard -i <ç/x"},
		{sys("linux", map[string]string{"IMUX_COPY_COMMAND": "cat >f"}), "/bin/sh -c cat >f <ç/x"},
	}
	for _, c := range cases {
		ran = nil
		if err := c.s.CopyText(ctx, "ç/x"); err != nil || len(ran) != 1 || ran[0] != c.want {
			t.Errorf("got %q %v, want %q", ran, err, c.want)
		}
	}
	wsl := sys("linux", map[string]string{"WSL_DISTRO_NAME": "U"}, "powershell.exe")
	if err := wsl.CopyText(ctx, "ç"); err != nil || !strings.Contains(ran[0], "FromBase64String('w6c=')") {
		t.Errorf("wsl %q %v", ran, err)
	}
	if err := sys("linux", nil).CopyText(ctx, "x"); err == nil {
		t.Error("no tool, no error")
	}

	if k := sys("linux", nil).Reveal(); k != "" {
		t.Errorf("a server has no file manager: %q", k)
	}
	mac := sys("darwin", nil)
	if mac.Reveal() != "file.reveal_finder" || mac.RevealFile(ctx, "/a b") != nil || ran[0] != "open -R /a b <" {
		t.Errorf("mac %q", ran)
	}
	if wsl := sys("linux", map[string]string{"WSL_DISTRO_NAME": "U"}); wsl.Reveal() != "file.reveal_explorer" ||
		wsl.RevealFile(ctx, "/mnt/c/p/a.txt") != nil || ran[0] != `explorer.exe /select,C:\p\a.txt <` {
		t.Errorf("wsl %q", ran)
	}
	lx := sys("linux", map[string]string{"DISPLAY": ":0"}, "gdbus")
	if lx.Reveal() != "file.reveal_folder" || lx.RevealFile(ctx, "/a b/c") != nil ||
		!strings.Contains(ran[0], "ShowItems ['file:///a%20b/c'] ") {
		t.Errorf("linux %q", ran)
	}
}

func TestOsaError(t *testing.T) {
	out := "execution error: Error: Error: The file “x” doesn’t exist. (-2700)\n"
	if m := osaError(out, errors.New("exit status 1")); m != "The file “x” doesn’t exist." {
		t.Fatalf("%q", m)
	}
	if m := osaError("", errors.New("exit status 1")); m != "exit status 1" {
		t.Fatalf("%q", m)
	}
}
