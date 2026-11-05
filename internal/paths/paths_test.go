package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHomeOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AJAN_HOME", filepath.Join(dir, "x"))
	if got := Home(); got != filepath.Join(dir, "x") {
		t.Fatalf("Home() = %q", got)
	}
	t.Setenv("AJAN_HOME", "")
	t.Setenv("HOME", dir)
	if got := Home(); got != filepath.Join(dir, ".ajan") {
		t.Fatalf("Home() = %q", got)
	}
}

func TestModes(t *testing.T) {
	SetUmask()
	dir := t.TempDir()
	t.Setenv("AJAN_HOME", filepath.Join(dir, "home"))
	if err := Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{Home(), Handoffs(), Launch()} {
		fi, err := os.Stat(d)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != DirMode {
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
	if fi.Mode().Perm() != FileMode {
		t.Errorf("file mode %v, want %v", fi.Mode().Perm(), FileMode)
	}
	// A plain create under the umask is private too.
	p := filepath.Join(Home(), "plain")
	if err := os.WriteFile(p, nil, 0o666); err != nil {
		t.Fatal(err)
	}
	fi, _ = os.Stat(p)
	if fi.Mode().Perm() != 0o600 {
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
	if fi.Mode().Perm() != DirMode {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
}

func TestShorten(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	if got := Shorten("/home/test/proj"); got != "~/proj" {
		t.Fatalf("got %q", got)
	}
	if got := Shorten("/home/testing"); got != "/home/testing" {
		t.Fatalf("got %q", got)
	}
}
