package tmux

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/sysx"
)

// Engine queries inside imux (D92, T97.1).

// fakeEngine replaces EngineCommand for one test: it records each line and
// answers with code, stdout and stderr, handling the line when handled.
type fakeEngine struct {
	calls          [][]string
	code           int
	stdout, stderr string
	handled        bool
}

func (f *fakeEngine) install(t *testing.T) {
	t.Helper()
	old := EngineCommand
	EngineCommand = func(_ context.Context, args []string, stdout, stderr io.Writer) (int, bool) {
		f.calls = append(f.calls, append([]string(nil), args...))
		if !f.handled {
			return 0, false
		}
		_, _ = io.WriteString(stdout, f.stdout)
		_, _ = io.WriteString(stderr, f.stderr)
		return f.code, true
	}
	t.Cleanup(func() { EngineCommand = old })
}

// engineFile is an imux-core file in a test folder; it is never run.
func engineFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), EngineName+sysx.ExeSuffix)
	if err := os.WriteFile(p, []byte("not a program"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestInProcessWhen(t *testing.T) {
	(&fakeEngine{handled: true}).install(t)
	bin := engineFile(t)
	other := filepath.Join(filepath.Dir(bin), "tmux")
	if err := os.WriteFile(other, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	list := []string{"-L", "s", "-u", "list-sessions"}
	for _, c := range []struct {
		name string
		bin  string
		args []string
		env  string
		want bool
	}{
		{"the engine", bin, list, "", true},
		{"turned off", bin, list, "0", false},
		{"another name", other, list, "", false},
		{"a missing engine", filepath.Join(t.TempDir(), EngineName+sysx.ExeSuffix), list, "", false},
		{"its version", bin, []string{"-V"}, "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("IMUX_ENGINE_INPROC", c.env)
			if got := inProcess(c.bin, c.args); got != c.want {
				t.Errorf("inProcess = %v, want %v", got, c.want)
			}
		})
	}
	EngineCommand = nil
	if inProcess(bin, list) {
		t.Error("in process without EngineCommand (a test binary)")
	}
}

func TestEngineRunnerInProcess(t *testing.T) {
	f := &fakeEngine{handled: true, stdout: "a\tb\n"}
	f.install(t)
	t.Setenv("IMUX_ENGINE_INPROC", "")
	c := &Client{Bin: engineFile(t), Socket: "s", Runner: EngineRunner{}}
	out, err := c.Run(context.Background(), Cmd{"list-sessions", "-F", "#{session_name}"})
	if err != nil || out != "a\tb\n" {
		t.Fatalf("Run = %q, %v", out, err)
	}
	want := [][]string{{"-L", "s", "-u", "list-sessions", "-F", "#{session_name}"}}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("calls %q, want %q", f.calls, want)
	}
}

func TestEngineRunnerErrors(t *testing.T) {
	f := &fakeEngine{handled: true, code: 1, stderr: "no server running on /tmp/imux-0/s\n"}
	f.install(t)
	t.Setenv("IMUX_ENGINE_INPROC", "")
	c := &Client{Bin: engineFile(t), Socket: "s", Runner: EngineRunner{}}
	_, err := c.Run(context.Background(), Cmd{"has-session"})
	if !errors.Is(err, ErrNoServer) {
		t.Fatalf("err = %v, want ErrNoServer", err)
	}
	var te *Error
	if !errors.As(err, &te) {
		t.Fatalf("err = %#v, want *Error", err)
	}
	if code, ok := te.Err.(*ExitCodeError); !ok || code.Code != 1 {
		t.Errorf("err.Err = %#v, want exit code 1", te.Err)
	}
	f.stderr = "can't find session: x\n"
	if _, err := c.Run(context.Background(), Cmd{"has-session", "-t", "x"}); !errors.Is(err, ErrNoSession) {
		t.Errorf("err = %v, want ErrNoSession", err)
	}
}

func TestEngineRunnerLeavesToBinary(t *testing.T) {
	// A line the engine does not handle (one that starts a server) runs the
	// binary: here a file that is no program, so starting it fails.
	f := &fakeEngine{handled: false}
	f.install(t)
	t.Setenv("IMUX_ENGINE_INPROC", "")
	c := &Client{Bin: engineFile(t), Socket: "s", Runner: EngineRunner{}}
	if _, err := c.Run(context.Background(), Cmd{"new-session", "-d"}); err == nil {
		t.Error("the binary was not run")
	}
	if len(f.calls) != 1 {
		t.Errorf("calls %q, want the line asked once", f.calls)
	}
}

func TestNewRunsInProcess(t *testing.T) {
	if _, ok := New("").Runner.(EngineRunner); !ok {
		t.Errorf("New's runner is %T, want EngineRunner", New("").Runner)
	}
}
