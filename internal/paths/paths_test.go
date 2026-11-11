package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHomeOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("IMUX_HOME", filepath.Join(dir, "x"))
	if got := Home(); got != filepath.Join(dir, "x") {
		t.Fatalf("Home() = %q", got)
	}
	t.Setenv("IMUX_HOME", "")
	t.Setenv("HOME", dir)
	if got := Home(); got != filepath.Join(dir, ".imux") {
		t.Fatalf("Home() = %q", got)
	}
}

// hasModes reports whether the OS keeps Unix permission bits. Windows does
// not (Go reports 0777/0666 there, and the user's profile ACL keeps imux's
// files private), so only the files' existence is checked on it.
var hasModes = runtime.GOOS != "windows"

func TestModes(t *testing.T) {
	SetUmask()
	dir := t.TempDir()
	t.Setenv("IMUX_HOME", filepath.Join(dir, "home"))
	if err := Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{Home(), Handoffs(), Launch()} {
		fi, err := os.Stat(d)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != DirMode && hasModes {
			t.Errorf("%s mode %v, want %v", d, fi.Mode().Perm(), DirMode)
		}
	}
	if err := WriteFile(State(), []byte("{}")); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(State())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != FileMode && hasModes {
		t.Errorf("file mode %v, want %v", fi.Mode().Perm(), FileMode)
	}
	// A plain create under the umask is private too.
	p := filepath.Join(Home(), "plain")
	if err := os.WriteFile(p, nil, 0o666); err != nil {
		t.Fatal(err)
	}
	fi, _ = os.Stat(p)
	if fi.Mode().Perm() != 0o600 && hasModes {
		t.Errorf("umask not applied: %v", fi.Mode().Perm())
	}
}

func TestEnsureTightensPermissiveDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "loose")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(dir)
	if !fi.IsDir() || fi.Mode().Perm() != DirMode && hasModes {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
}

func TestShorten(t *testing.T) {
	home := filepath.FromSlash("/home/test")
	t.Setenv("HOME", home)
	if got := Shorten(filepath.Join(home, "proj")); got != "~"+string(filepath.Separator)+"proj" {
		t.Fatalf("got %q", got)
	}
	if got := Shorten(home + "ing"); got != home+"ing" {
		t.Fatalf("got %q", got)
	}
}
