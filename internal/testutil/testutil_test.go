package testutil

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestIsolate(t *testing.T) {
	realHome, _ := os.UserHomeDir()
	h := Isolate(t)
	for _, v := range []string{"HOME", "AJAN_HOME", "CLAUDE_CONFIG_DIR", "CODEX_HOME", "COPILOT_HOME"} {
		got := os.Getenv(v)
		if !strings.HasPrefix(got, h.Root) {
			t.Errorf("%s=%q is not under the temp root", v, got)
		}
		if realHome != "" && got == realHome {
			t.Errorf("%s points at the real home", v)
		}
	}
	if _, err := os.Stat(h.Project); err != nil {
		t.Fatal(err)
	}
}

func TestClock(t *testing.T) {
	c := Clock()
	if !c.Now().Equal(FrozenTime) {
		t.Fatal("not frozen")
	}
	c.Advance(time.Minute)
	if !c.Now().Equal(FrozenTime.Add(time.Minute)) {
		t.Fatal("advance failed")
	}
}
