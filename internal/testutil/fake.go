package testutil

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// Fake programs. A test that needs a program on disk for the code under test
// to run (an agent CLI, a slow git, an editor) writes a shell script on Unix.
// Windows runs no shell scripts, so there the program is a batch file,
// path.cmd, that runs the test binary itself with fakeEnv naming a spec file
// (path.fake: which fake it is, and its data). The package's TestMain calls
// FakeMain first, which runs that fake instead of the tests when the process
// was started as one. The test binary is not copied: a freshly written .exe
// is scanned by antivirus software before it may start, which can take
// seconds, and a batch file is also how npm installs agent CLIs on Windows.
//
// cmd.exe passes the batch file's arguments on as it got them (%*), so a
// fake's arguments must not need cmd quoting (&, |, <, >, ^, % or quotes).
// Killing the program kills cmd.exe; code that does not kill the whole
// process tree leaves the fake running until it returns, as a real program's
// children would be left.

// fakeEnv names the spec file of the fake a process runs as.
const fakeEnv = "IMUX_TEST_FAKE"

// FakeFunc is a fake program's main: data is what the test gave WriteFake,
// args are the program's arguments (without the program). It returns the
// exit code.
type FakeFunc func(data string, args []string) int

// fakeSpec is what a fake's spec file holds.
type fakeSpec struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

// builtinFakes are the fakes every package has.
var builtinFakes = map[string]FakeFunc{
	// "print" writes data to stdout and exits 0.
	"print": func(data string, _ []string) int {
		fmt.Print(data)
		return 0
	},
	// "exit" exits with the code in data (0 when empty).
	"exit": func(data string, _ []string) int {
		n, _ := strconv.Atoi(strings.TrimSpace(data))
		return n
	},
}

// FakeMain runs the fake this process was started as, if any, and exits;
// otherwise it returns and TestMain goes on to m.Run. fakes adds the
// package's own fakes to the builtin ones ("print", "exit").
func FakeMain(fakes map[string]FakeFunc) {
	p := os.Getenv(fakeEnv)
	if p == "" {
		return
	}
	os.Exit(runFake(p, fakes, os.Args[1:]))
}

func runFake(specPath string, fakes map[string]FakeFunc, args []string) int {
	b, err := os.ReadFile(specPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake:", err)
		return 127
	}
	var s fakeSpec
	if err := json.Unmarshal(b, &s); err != nil {
		fmt.Fprintln(os.Stderr, "fake:", err)
		return 127
	}
	f := fakes[s.Name]
	if f == nil {
		f = builtinFakes[s.Name]
	}
	if f == nil {
		fmt.Fprintln(os.Stderr, "fake: unknown fake", s.Name)
		return 127
	}
	return f(s.Data, args)
}

// FakeName is the path WriteFake writes for path: path.cmd on Windows,
// path elsewhere.
func FakeName(path string) string {
	if runtime.GOOS == "windows" {
		return path + ".cmd"
	}
	return path
}

// WriteFake writes the fake program name (with data) at FakeName(path) and
// returns that path. Only a test binary whose TestMain calls FakeMain can
// run fakes; WriteFake is for Windows, where Script uses it.
func WriteFake(t testing.TB, path, name, data string) string {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(self, `"%`) {
		t.Fatalf("fake: test binary path %q cannot go in a batch file", self)
	}
	exe := FakeName(path)
	spec, err := json.Marshal(fakeSpec{Name: name, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	specPath := path + ".fake"
	if strings.ContainsAny(specPath, `"%`) {
		t.Fatalf("fake: path %q cannot go in a batch file", specPath)
	}
	WriteFile(t, specPath, spec)
	bat := "@echo off\r\n" +
		`set "` + fakeEnv + "=" + specPath + "\"\r\n" +
		`"` + self + "\" %*\r\n"
	WriteFile(t, exe, []byte(bat))
	return exe
}

// Script writes an executable for the code under test to run and returns
// its path: on Unix the shell script sh (a #!/bin/sh line is added) at path,
// on Windows the fake program name with data at path.cmd (see WriteFake).
func Script(t testing.TB, path, sh, name, data string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return WriteFake(t, path, name, data)
	}
	WriteFile(t, path, []byte("#!/bin/sh\n"+sh+"\n"))
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// Symlink makes newname a symlink to oldname, or skips the test on Windows
// when the account may not create symlinks (that needs Developer Mode or an
// elevated process there).
func Symlink(t testing.TB, oldname, newname string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(newname), 0o755); err != nil {
		t.Fatal(err)
	}
	err := os.Symlink(oldname, newname)
	if err == nil {
		return
	}
	if runtime.GOOS == "windows" {
		t.Skipf("creating a symlink needs Developer Mode or elevation on Windows: %v", err)
	}
	t.Fatal(err)
}
