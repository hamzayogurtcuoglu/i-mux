//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hamzayogurtcuoglu/i-mux/internal/testutil"
)

// TestOnlyTheEngine (D87): i-mux runs on i-mux-core whatever is installed or
// configured. With backend = "tmux" left in the config and a tmux on the PATH,
// imux uses the imux-core next to it (a broken one fails doctor) and never
// runs tmux; IMUX_TMUX_BIN still names the engine.
func TestOnlyTheEngine(t *testing.T) {
	skipOnSystem(t)
	h := newH(t, "[engine]\nbackend = \"tmux\"\n")
	sibling := filepath.Join(filepath.Dir(h.Bin), "imux-core"+exeSuffix)
	// Under make e2e IMUX_TMUX_BIN is set for the whole suite; these calls
	// test the engine's resolution, so they must not inherit it.
	plain := []string{"IMUX_TMUX_BIN="}

	// A tmux on the PATH that leaves a mark when it runs.
	tools := filepath.Join(h.Homes.Root, "tools")
	ran := filepath.Join(h.Homes.Root, "tmux-ran")
	testutil.WriteFile(t, filepath.Join(tools, "tmux"), []byte("#!/bin/sh\ntouch '"+ran+"'\necho 'tmux 3.8'\n"))
	_ = os.Chmod(filepath.Join(tools, "tmux"), 0o755)
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))

	// A broken engine beside imux: doctor fails on it, not on tmux.
	fake := filepath.Join(h.Homes.Root, "fake-core")
	testutil.WriteFile(t, fake, []byte("#!/bin/sh\nexit 1\n"))
	_ = os.Chmod(fake, 0o755)
	if err := os.Symlink(fake, sibling); err != nil {
		t.Fatal(err)
	}
	if out, errOut, err := h.imuxEnv(plain, "doctor", "--ci"); err == nil {
		t.Fatalf("a broken engine still ran: %s%s", out, errOut)
	}

	// The real engine beside imux: doctor passes on it.
	_ = os.Remove(sibling)
	if err := os.Symlink(h.Tmux, sibling); err != nil {
		t.Fatal(err)
	}
	if out, errOut, err := h.imuxEnv(plain, "doctor", "--ci"); err != nil {
		t.Fatalf("doctor on the engine: %v\n%s%s", err, out, errOut)
	}
	_ = os.Remove(sibling)

	// IMUX_TMUX_BIN names the engine: works with nothing beside imux.
	if out, errOut, err := h.imuxEnv([]string{"IMUX_TMUX_BIN=" + h.Tmux}, "doctor", "--ci"); err != nil {
		t.Fatalf("IMUX_TMUX_BIN: %v\n%s%s", err, out, errOut)
	}
	if _, err := os.Stat(ran); err == nil {
		t.Fatal("imux ran tmux")
	}
}
