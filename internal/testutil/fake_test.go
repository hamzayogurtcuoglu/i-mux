package testutil

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestMain runs this package's own fakes on Windows (see FakeMain).
func TestMain(m *testing.M) {
	FakeMain(map[string]FakeFunc{
		// "args" prints its arguments, one per line.
		"args": func(_ string, args []string) int {
			os.Stdout.WriteString(strings.Join(args, "\n") + "\n")
			return 0
		},
	})
	os.Exit(m.Run())
}

func TestScriptRuns(t *testing.T) {
	dir := t.TempDir()
	hi := Script(t, filepath.Join(dir, "hi"), "echo hi", "print", "hi\n")
	if hi != FakeName(filepath.Join(dir, "hi")) {
		t.Fatalf("path %q", hi)
	}
	if out, err := exec.Command(hi, "--version").Output(); err != nil || string(out) != "hi\n" {
		t.Fatalf("print: %q %v", out, err)
	}
	fail := Script(t, filepath.Join(dir, "fail"), "exit 3", "exit", "3")
	var ee *exec.ExitError
	if err := exec.Command(fail).Run(); !errors.As(err, &ee) || ee.ExitCode() != 3 {
		t.Fatalf("exit: %v", err)
	}
	args := Script(t, filepath.Join(dir, "args"), `for a in "$@"; do echo "$a"; done`, "args", "")
	if out, err := exec.Command(args, "a", "b c", "--x=1").Output(); err != nil || string(out) != "a\nb c\n--x=1\n" {
		t.Fatalf("args: %q %v", out, err)
	}
}

func TestFakeName(t *testing.T) {
	want := "p"
	if runtime.GOOS == "windows" {
		want = "p.cmd"
	}
	if got := FakeName("p"); got != want {
		t.Fatalf("FakeName %q", got)
	}
}

func TestRunFakeErrors(t *testing.T) {
	dir := t.TempDir()
	if n := runFake(filepath.Join(dir, "none.fake"), nil, nil); n != 127 {
		t.Errorf("missing spec: %d", n)
	}
	p := filepath.Join(dir, "x.fake")
	WriteFile(t, p, []byte(`{"name":"nope"}`))
	if n := runFake(p, nil, nil); n != 127 {
		t.Errorf("unknown fake: %d", n)
	}
	WriteFile(t, p, []byte(`{"name":"exit","data":"4"}`))
	if n := runFake(p, nil, nil); n != 4 {
		t.Errorf("builtin exit: %d", n)
	}
}
