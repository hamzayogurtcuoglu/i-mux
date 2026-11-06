package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hamzayogurtcuoglu/i-mux/internal/paths"
)

const helperEnv = "IMUX_STATE_TEST_HELPER"

func TestMain(m *testing.M) {
	if id := os.Getenv(helperEnv); id != "" {
		n, _ := strconv.Atoi(os.Getenv("IMUX_STATE_TEST_N"))
		for i := 0; i < n; i++ {
			err := Update(func(s *State) error {
				s.SetNotice(fmt.Sprintf("p%s-%d", id, i))
				return nil
			})
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestConcurrentWritersLoseNothing(t *testing.T) {
	t.Setenv("IMUX_HOME", t.TempDir())
	const procs, n = 2, 60
	var cmds []*exec.Cmd
	for p := 0; p < procs; p++ {
		cmd := exec.Command(os.Args[0], "-test.run=^$")
		cmd.Env = append(os.Environ(), helperEnv+"="+strconv.Itoa(p), "IMUX_STATE_TEST_N="+strconv.Itoa(n))
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, cmd)
	}
	for _, c := range cmds {
		if err := c.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(paths.State())
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(b) {
		t.Fatalf("corrupt JSON: %s", b)
	}
	s, err := Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Notices) != procs*n {
		t.Fatalf("lost updates: %d notices, want %d", len(s.Notices), procs*n)
	}
}

func TestUnknownFieldsSurvive(t *testing.T) {
	t.Setenv("IMUX_HOME", t.TempDir())
	if err := paths.EnsureDir(paths.Home()); err != nil {
		t.Fatal(err)
	}
	raw := `{"version":1,"future_thing":{"a":[1,2]},"limits":{},"zzz":"keep"}`
	if err := os.WriteFile(paths.State(), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Update(func(s *State) error {
		s.SetBin("claude", &Bin{Path: "/x/claude", PathEnv: "/x"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(paths.State())
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	var ft bytes.Buffer
	_ = json.Compact(&ft, m["future_thing"])
	if string(m["zzz"]) != `"keep"` || !strings.Contains(ft.String(), "[1,2]") {
		t.Fatalf("unknown fields lost: %s", b)
	}
	if string(m["version"]) != "1" {
		t.Fatalf("version missing: %s", b)
	}
	s, _ := Read()
	if s.Bins["claude"].Path != "/x/claude" {
		t.Fatal("typed field lost")
	}
}

func TestReadMissingAndCorrupt(t *testing.T) {
	t.Setenv("IMUX_HOME", t.TempDir())
	s, err := Read()
	if err != nil || s.Version != SchemaVersion {
		t.Fatalf("%v %+v", err, s)
	}
	_ = paths.EnsureDir(paths.Home())
	_ = os.WriteFile(paths.State(), []byte("{not json"), 0o600)
	s, err = Read()
	if err != nil || s.Version != SchemaVersion {
		t.Fatalf("corrupt file should yield an empty state: %v %+v", err, s)
	}
}

func TestLimitsExpire(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	s := &State{}
	s.SetLimit("claude", &Limit{At: now, ExpiresAt: now.Add(time.Hour)})
	s.SetLimit("codex", &Limit{At: now, ExpiresAt: now.Add(-time.Minute)})
	if s.ActiveLimit("claude", now) == nil || s.ActiveLimit("codex", now) != nil {
		t.Fatal("ActiveLimit wrong")
	}
	if !s.DropExpired(now) || len(s.Limits) != 1 {
		t.Fatal("DropExpired wrong")
	}
	if s.DropExpired(now) {
		t.Fatal("second DropExpired should change nothing")
	}
}

func TestUpdateErrorWritesNothing(t *testing.T) {
	t.Setenv("IMUX_HOME", t.TempDir())
	err := Update(func(s *State) error { return fmt.Errorf("boom") })
	if err == nil {
		t.Fatal("want error")
	}
	if _, err := os.Stat(paths.State()); !os.IsNotExist(err) {
		t.Fatal("state written despite error")
	}
}
