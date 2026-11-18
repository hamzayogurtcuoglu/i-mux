//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestEngineSwitch (D83, T88.3): with [engine] backend = "core" the imux-core
// next to the imux binary is the engine (a broken one breaks every engine
// call), IMUX_TMUX_BIN still overrides everything, a running server guards
// the switch, and --yes closes it and switches.
func TestEngineSwitch(t *testing.T) {
	h := newH(t, "[engine]\nbackend = \"core\"\n")
	sibling := filepath.Join(filepath.Dir(h.Bin), "imux-core")
	// Under make e2e-core IMUX_TMUX_BIN is set for the whole suite; these
	// calls test the config's resolution, so they must not inherit it.
	plainBin := []string{"IMUX_TMUX_BIN="}

	// The sibling wins: a broken imux-core there fails doctor.
	fake := filepath.Join(h.Homes.Root, "fake-core")
	testutil.WriteFile(t, fake, []byte("#!/bin/sh\nexit 1\n"))
	_ = os.Chmod(fake, 0o755)
	if err := os.Symlink(fake, sibling); err != nil {
		t.Fatal(err)
	}
	if out, errOut, err := h.imuxEnv(plainBin, "doctor", "--ci"); err == nil {
		t.Fatalf("a broken sibling still ran: %s%s", out, errOut)
	}

	// The real engine as the sibling: doctor passes on it.
	_ = os.Remove(sibling)
	if err := os.Symlink(h.Tmux, sibling); err != nil {
		t.Fatal(err)
	}
	out, _, err := h.imuxEnv(plainBin, "doctor", "--ci")
	if err != nil {
		t.Fatalf("doctor on the sibling engine: %v\n%s", err, out)
	}

	// IMUX_TMUX_BIN overrides the backend too: works with no sibling at all.
	_ = os.Remove(sibling)
	if out, errOut, err := h.imuxEnv([]string{"IMUX_TMUX_BIN=" + h.Tmux}, "doctor", "--ci"); err != nil {
		t.Fatalf("IMUX_TMUX_BIN: %v\n%s%s", err, out, errOut)
	}

	// A running server guards the switch.
	if err := os.Symlink(h.Tmux, sibling); err != nil {
		t.Fatal(err)
	}
	h.mustTmux("new-session", "-d", "-s", "guard", "sh")
	out, errOut, err := h.imuxEnv(plainBin, "__engine", "switch", "tmux")
	if err == nil || !strings.Contains(out+errOut, "sessions are running") {
		t.Fatalf("guard: %v\n%s%s", err, out, errOut)
	}

	// --yes closes the server and switches.
	if out, errOut, err := h.imuxEnv(plainBin, "__engine", "switch", "tmux", "--yes"); err != nil {
		t.Fatalf("switch: %v\n%s%s", err, out, errOut)
	}
	b, err := os.ReadFile(filepath.Join(h.Homes.Imux, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `backend = "tmux"`) {
		t.Fatalf("config after the switch:\n%s", b)
	}
	if out, err := h.tmux("list-sessions"); err == nil || !strings.Contains(out, "no server running") {
		t.Fatalf("the server survived the switch: %v\n%s", err, out)
	}

	// After the switch imux runs on tmux from the PATH again.
	if out, _, err := h.imux("doctor", "--ci"); err != nil {
		t.Fatalf("doctor after the switch: %v\n%s", err, out)
	}
}
