package launch

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

const helperEnv = "IMUX_LAUNCH_TEST_SPEC"

func TestMain(m *testing.M) {
	if p := os.Getenv(helperEnv); p != "" {
		s, err := Read(p, true)
		if err == nil {
			err = Exec(s)
		}
		os.Stderr.WriteString("exec failed: " + err.Error())
		os.Exit(3)
	}
	os.Exit(m.Run())
}

func TestValidate(t *testing.T) {
	ok := &Spec{Bin: "/bin/echo", Args: []string{"a"}, MaxArgBytes: 10}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	big := &Spec{Bin: "/bin/echo", Args: []string{strings.Repeat("x", 11)}, MaxArgBytes: 10}
	if err := big.Validate(); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
	if err := (&Spec{Bin: "echo"}).Validate(); err == nil {
		t.Fatal("relative bin accepted")
	}
	if err := (&Spec{Bin: "/bin/echo", Args: []string{"a\x00b"}}).Validate(); err == nil {
		t.Fatal("NUL accepted")
	}
}

func TestEnviron(t *testing.T) {
	s := &Spec{Agent: "antigravity", Pane: "%3", Env: map[string]string{"PATH": "/nvm/bin:/usr/bin"}}
	env := s.Environ([]string{"HOME=/home/test", "PATH=/usr/bin", "IMUX_PANE=%1"})
	joined := strings.Join(env, "\n")
	for _, want := range []string{"HOME=/home/test", "PATH=/nvm/bin:/usr/bin", "IMUX_PANE=%3", "IMUX_AGENT=antigravity"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %s in %q", want, env)
		}
	}
	if strings.Count(joined, "PATH=") != 1 || strings.Count(joined, "IMUX_PANE=") != 1 {
		t.Fatalf("duplicates: %q", env)
	}
}

func TestWriteReadSweep(t *testing.T) {
	testutil.Isolate(t)
	p, err := Write(&Spec{Agent: "claude", Bin: "/bin/echo", Args: []string{"x"}})
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("%v %v", err, fi.Mode())
	}
	s, err := Read(p, true)
	if err != nil || s.Bin != "/bin/echo" {
		t.Fatalf("%v %+v", err, s)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("spec not deleted after read")
	}
	old, _ := Write(&Spec{Bin: "/bin/true"})
	testutil.Touch(t, old, time.Now().Add(-2*time.Hour))
	fresh, _ := Write(&Spec{Bin: "/bin/true"})
	Sweep(time.Hour)
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("old spec not swept")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("fresh spec swept")
	}
	if !strings.HasPrefix(p, paths.Launch()) {
		t.Fatal("spec outside launch dir")
	}
}

// TestExecByteExactArgv execs a fake agent in a child process and checks
// that argv arrives byte for byte, without any shell involvement.
func TestExecByteExactArgv(t *testing.T) {
	h := testutil.Isolate(t)
	fake := filepath.Join(h.Home, "bin", "fake-agent")
	out := filepath.Join(h.Home, "argv.bin")
	testutil.WriteFile(t, fake, []byte("#!/bin/sh\nfor a in \"$@\"; do printf '%s\\0' \"$a\"; done > \""+out+"\"\nprintf '%s|%s|%s' \"$IMUX_PANE\" \"$IMUX_AGENT\" \"$(pwd)\" > \""+out+".env\"\n"))
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{
		"--session-id", "u-1",
		"say \"hi\" 'there'\nline2 $(touch /tmp/pwned) `id` ; | & * ? ~ ğüşıöç 🎉\t\\",
		"",
		"-i",
	}
	sp, err := Write(&Spec{Agent: "claude", Bin: fake, Args: args, Dir: h.Project, Pane: "%7", Env: map[string]string{"PATH": "/usr/bin:/bin"}})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), helperEnv+"="+sp)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join(args, "\x00") + "\x00"
	if !bytes.Equal(got, []byte(want)) {
		t.Fatalf("argv mismatch\n got %q\nwant %q", got, want)
	}
	env, _ := os.ReadFile(out + ".env")
	if string(env) != "%7|claude|"+h.Project {
		t.Fatalf("env %q", env)
	}
	if _, err := os.Stat(sp); !os.IsNotExist(err) {
		t.Fatal("spec not deleted")
	}
}

func TestExecRejectsTooLarge(t *testing.T) {
	called := false
	orig := execFunc
	execFunc = func(string, []string, []string) error { called = true; return nil }
	defer func() { execFunc = orig }()
	err := Exec(&Spec{Bin: "/bin/echo", Args: []string{strings.Repeat("x", 200)}, MaxArgBytes: 100})
	if !errors.Is(err, ErrTooLarge) || called {
		t.Fatalf("err=%v called=%v", err, called)
	}
}
